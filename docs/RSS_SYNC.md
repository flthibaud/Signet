# Synchronisation automatique des flux RSS

## Table des matières

- [Objectif](#objectif)
- [Phase 1 — Scheduler intégré (goroutine + ticker)](#phase-1--scheduler-intégré-goroutine--ticker)
- [Phase 2 — Worker queue PostgreSQL (River)](#phase-2--worker-queue-postgresql-évolution-future)

---

## Objectif

Aujourd'hui, les articles d'un flux RSS ne sont récupérés qu'au moment de la souscription (`POST /v1/subscriptions`), via un `go func()` fire-and-forget. Il n'y a aucun mécanisme de synchronisation périodique.

L'objectif est de mettre en place un système qui :

1. Récupère automatiquement les nouveaux articles de chaque flux RSS actif.
2. Économise la bande passante et respecte les serveurs cibles (Courtoisie HTTP : ETag / Last-Modified).
3. Distribue les articles à **tous** les abonnés du flux de manière performante.
4. Respecte les ressources locales (rate limiting, concurrence contrôlée, prévention des requêtes en double).
5. S'intègre proprement au cycle de vie de l'application (graceful shutdown).

---

## Phase 1 — Scheduler intégré (goroutine + ticker)

### Architecture

```text
┌──────────────────────────────────────────────────────────┐
│                      main.go                             │
│                                                          │
│   ┌──────────┐      ┌────────────────────────────┐       │
│   │  HTTP    │      │        Scheduler            │       │
│   │  Server  │      │                             │       │
│   │          │      │  ┌───────┐   ┌───────────┐  │       │
│   │          │      │  │Ticker │──>│ syncFeeds │  │       │
│   │          │      │  │(15min)│   └─────┬─────┘  │       │
│   │          │      │  └───────┘         │        │       │
│   │          │      │              ┌─────▼─────┐  │       │
│   │          │      │              │Worker Pool│  │       │
│   │          │      │              │  (N=5)    │  │       │
│   │          │      │              └─────┬─────┘  │       │
│   └──────────┘      └──────────────┬─────┘        │       │
│                                    │              │       │
│              ┌─────────────────────▼──────────┐   │       │
│              │         FeedService            │   │       │
│              │  ImportArticlesForSubscribers() │   │       │
│              └────────────────┬───────────────┘   │       │
│                               │                   │       │
│                    ┌──────────▼──────────┐        │       │
│                    │     PostgreSQL      │        │       │
│                    └────────────────────┘         │       │
└──────────────────────────────────────────────────────────┘
```

### Composants

#### 1. Nouveaux champs dans la base de données
Pour supporter la courtoisie HTTP et éviter les téléchargements inutiles, la table `feeds` doit inclure :
- `http_etag` (VARCHAR) : Pour stocker l'identifiant de version du flux.
- `http_last_modified` (VARCHAR) : Pour stocker la date de dernière modification donnée par le serveur cible.
- `fetching_since` (TIMESTAMPTZ, NULL) : Timestamp du début du fetch en cours. Remplace un simple booléen pour permettre l'auto-expiration des locks orphelins (ex: crash du worker). Un feed est considéré verrouillé si `fetching_since IS NOT NULL AND fetching_since > NOW() - INTERVAL '10 minutes'`.
- `consecutive_failures` (INTEGER, DEFAULT 0) : Compteur d'échecs consécutifs. Le feed est automatiquement désactivé (`is_active = FALSE`) après 10 échecs consécutifs. Remis à zéro après chaque fetch réussi.

#### 2. Scheduler (`internal/service/scheduler.go`)

Le scheduler orchestre la synchronisation périodique.

```go
type Scheduler struct {
    services  *Services
    logger    *jsonlog.Logger
    interval  time.Duration    // Intervalle entre chaque tick (ex: 15min)
    workers   int              // Nombre de workers concurrents (ex: 5)
    quit      chan struct{}    // Signal d'arrêt
    wg        sync.WaitGroup   // Attente de fin des workers
}
```

**Cycle de vie :** Lancer le Ticker -> Fetcher les feeds -> Distribuer au Worker Pool -> Attendre la complétion ou le signal `quit` (Graceful Shutdown).

#### 3. Query `GetFeedsToSync` (`internal/data/feeds.go`)

Pour éviter qu'un feed long à traiter ne soit sélectionné deux fois lors du tick suivant (Thundering Herd), nous utilisons une requête `UPDATE ... RETURNING` avec `SKIP LOCKED` natif à PostgreSQL. 

```sql
UPDATE feeds
SET fetching_since = NOW()
WHERE id IN (
    SELECT id FROM feeds f
    WHERE f.is_active = TRUE
      AND (f.fetching_since IS NULL OR f.fetching_since < NOW() - INTERVAL '10 minutes')
      AND EXISTS (SELECT 1 FROM subscriptions s WHERE s.feed_id = f.id)
      AND (f.last_fetched_at IS NULL OR f.last_fetched_at < NOW() - INTERVAL '15 minutes')
    ORDER BY f.last_fetched_at ASC NULLS FIRST
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
RETURNING id, url, http_etag, http_last_modified;
```

**Notes :**
- `last_fetched_at` n'est **pas** mis à jour ici. Il sera mis à jour uniquement après un fetch réussi, ce qui évite de devoir restaurer sa valeur précédente en cas d'échec.
- `fetching_since` avec un seuil de 10 minutes remplace le booléen `is_fetching` : si un worker crash, le lock expire automatiquement et le feed redevient éligible au prochain tick.

#### 4. Distribution aux abonnés (`internal/service/fetcher.go`)

Nouvelle méthode `ImportArticlesForSubscribers()` optimisée :

La logique d'extraction d'articles (readability, hash, métadonnées) doit être factorisée dans une méthode privée commune avec `ImportArticles()` existante, pour éviter la duplication de code.

```text
Pour chaque feed à synchroniser :
  1. Fetch le flux RSS en envoyant les headers conditionnels :
     - If-None-Match: [http_etag]
     - If-Modified-Since: [http_last_modified]
  2. Si le serveur répond HTTP 304 (Not Modified) :
     a. Mettre fetching_since = NULL, last_fetched_at = NOW(),
        consecutive_failures = 0, loguer le succès, et passer au feed suivant.
  3. Si erreur (timeout, 4xx, 5xx) :
     a. Incrémenter consecutive_failures.
     b. Si consecutive_failures >= 10, mettre is_active = FALSE et loguer un warning.
     c. Mettre fetching_since = NULL (libérer le lock).
  4. Si HTTP 200 (nouveau contenu) :
     a. Parser le flux, sauvegarder les nouveaux etag/last-modified.
     b. Pour chaque item du flux, vérifier si l'article existe (par hash). Si non, créer l'article.
        (Réutiliser la logique privée commune : readability, hash, métadonnées.)
     c. Récupérer la liste des IDs de TOUS les abonnés du feed.
     d. Effectuer un BULK INSERT pour créer les liens abonnés-articles en une seule requête SQL
        (INSERT INTO links ... ON CONFLICT DO NOTHING) plutôt qu'une requête par abonné.
  5. Mettre à jour : fetching_since = NULL, last_fetched_at = NOW(),
     consecutive_failures = 0 en BDD.
```

### Distribution des feeds aux workers

Les feeds récupérés par `GetFeedsToSync` (jusqu'à `SCHEDULER_BATCH_SIZE`) sont envoyés dans un channel buffered. Les `SCHEDULER_WORKERS` goroutines consomment ce channel en parallèle. Chaque worker traite un feed à la fois jusqu'à ce que le channel soit vide.

```go
feedsChan := make(chan Feed, len(feeds))
for _, f := range feeds {
    feedsChan <- f
}
close(feedsChan)

for i := 0; i < s.workers; i++ {
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        for feed := range feedsChan {
            s.processFeed(ctx, feed)
        }
    }()
}
```

### Rate limiting par domaine

Pour éviter de bombarder un même serveur avec plusieurs requêtes simultanées, un rate limiter par domaine est appliqué (`golang.org/x/time/rate`). Chaque domaine est limité à 1 requête par seconde. Le rate limiter est partagé entre tous les workers via une `sync.Map`.

### Configuration

| Variable | Description | Défaut |
|----------|-------------|--------|
| `SCHEDULER_INTERVAL` | Intervalle entre chaque sync | `15m` |
| `SCHEDULER_WORKERS` | Nombre de workers concurrents | `5` |
| `SCHEDULER_BATCH_SIZE` | Nombre max de feeds par tick | `50` |

---

## Phase 2 — Worker queue PostgreSQL (évolution future)

### Pourquoi évoluer ?

La phase 1 atteint ses limites en cas de **Scaling horizontal** (multiples instances API), de fort volume de flux, et de besoin de **Retry avec backoff**.

### Architecture cible avec River

[River](https://github.com/riverqueue/river) est une job queue Go native pour PostgreSQL. Elle gère le verrouillage via `SELECT ... FOR UPDATE SKIP LOCKED`, le retry exponentiel, et les workers concurrents.

### Optimisation : Le Polling Adaptatif

Au lieu de synchroniser tous les flux bêtement toutes les 15 minutes via un cron global, la Phase 2 introduira un système auto-régulé :
* À la fin d'un `FeedSyncJob` réussi, le worker analyse la fréquence de publication du flux.
* Il planifie **lui-même** le prochain run pour ce flux en insérant un nouveau job avec `scheduled_at = NOW() + dynamic_interval`.
* Un flux très actif sera planifié dans 10 minutes, un flux inactif dans 24 heures.

### Schéma de la table jobs (géré par River)

```sql
CREATE TABLE river_job (
    id          BIGSERIAL PRIMARY KEY,
    state       TEXT NOT NULL DEFAULT 'available',
    queue       TEXT NOT NULL DEFAULT 'default',
    kind        TEXT NOT NULL,
    args        JSONB NOT NULL,
    attempt     INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    priority    INTEGER NOT NULL DEFAULT 1,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    -- ... autres champs internes à River
);
```

### Types de jobs

#### 1. `FeedSyncJob` — Synchronisation d'un flux
* **Rôle :** Fetch le flux (avec courtoisie HTTP), Bulk Insert des articles/links, calcule le prochain intervalle de vérification, et enqueue le prochain `FeedSyncJob`.
* **Retry :** 3 tentatives avec backoff exponentiel (1min, 5min, 30min). Marqué comme `discarded` après 3 échecs (Dead Letter Queue).

#### 2. `FeedHealthCheckJob` — Vérification et Nettoyage (Maintenance)
* **Rôle 1 :** Job périodique qui désactive les feeds morts (ex: 404 consécutifs) et notifie les abonnés.
* **Rôle 2 (Pruning) :** Configuration du *Pruning* agressif de River pour supprimer régulièrement les `river_job` avec `state = 'completed'`. Cela évite que la table PostgreSQL ne grossisse indéfiniment.

### Considérations techniques pour la migration

1. **Pool de connexions (`pgxpool`) :** River requiert le driver `pgx`. Il est crucial de configurer correctement la taille du pool (`max_conns`) pour absorber les requêtes de l'API HTTP **et** la concurrence des workers River.
2. **Migrations automatiques :** River fournit ses propres outils de migration SQL pour maintenir la table `river_job`.
3. **API Admin :** Exposer les métriques de River (jobs en attente, erreurs) sur un endpoint protégé pour faciliter l'observabilité.

---

**Dernière mise à jour** : Mars 2026