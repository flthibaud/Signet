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
│              │  ClaimDueFeeds() / SyncFeed()  │   │       │
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
- `consecutive_failures` (INTEGER, DEFAULT 0) : Compteur d'échecs consécutifs. Le feed est désactivé (`is_active = FALSE`) après 10 échecs consécutifs. Désactivé ne veut pas dire abandonné : il est retenté une fois par jour (`deadFeedRetryInterval`), et le premier fetch réussi le réactive. Un nouvel abonnement (formulaire ou import OPML) le réactive immédiatement via `MarkDueForSync`. Remis à zéro après chaque fetch réussi.

#### 2. Scheduler (`internal/service/scheduler.go`)

Le scheduler orchestre la synchronisation périodique.

```go
type Scheduler struct {
    feedSvc   *FeedService     // Exécute la synchro d'un feed
    models    data.Models      // Claim des feeds dus + ménage horaire
    logger    *jsonlog.Logger
    interval  time.Duration    // Intervalle entre chaque tick (ex: 15min)
    workers   int              // Nombre de workers concurrents (ex: 5)
    quit      chan struct{}    // Signal d'arrêt
    wg        sync.WaitGroup   // Attente de fin des workers
}
```

**Cycle de vie :** Lancer le Ticker -> Fetcher les feeds -> Distribuer au Worker Pool -> Attendre la complétion ou le signal `quit` (Graceful Shutdown).

**Seul le scheduler synchronise.** Un nouvel abonnement (formulaire ou import OPML) n'importe rien lui-même : `Subscribe` appelle `Feeds.MarkDueForSync` puis `Scheduler.TriggerSync`, et c'est le prochain passage du pool qui ramène les articles. Toute synchro passe ainsi par un claim (`GetFeedsToSync`) ; un import lancé depuis l'abonnement tournait sans claim, en parallèle d'un worker qui pouvait synchroniser le même feed. `NewServices` construit le scheduler avant `SubscriptionService` pour pouvoir le lui passer.

#### 3. Query `GetFeedsToSync` (`internal/data/feeds.go`)

Pour éviter qu'un feed long à traiter ne soit sélectionné deux fois lors du tick suivant (Thundering Herd), nous utilisons une requête `UPDATE ... RETURNING` avec `SKIP LOCKED` natif à PostgreSQL. 

```sql
UPDATE feeds
SET fetching_since = NOW()
WHERE id IN (
    SELECT id FROM feeds f
    WHERE (f.is_active = TRUE OR f.last_fetched_at < NOW() - make_interval(secs => $4::double precision))
      AND (f.fetching_since IS NULL OR f.fetching_since < NOW() - make_interval(secs => $2::double precision))
      AND EXISTS (SELECT 1 FROM subscriptions s WHERE s.feed_id = f.id)
      AND (f.last_fetched_at IS NULL OR f.last_fetched_at < NOW() - make_interval(secs => $3::double precision))
    ORDER BY f.last_fetched_at ASC NULLS FIRST
    LIMIT $1
    FOR UPDATE SKIP LOCKED
)
RETURNING id, url, http_etag, http_last_modified;
```

**Notes :**
- `last_fetched_at` n'est **pas** mis à jour ici, mais à la fin de la tentative, qu'elle réussisse (`ReleaseFeed`) ou échoue (`MarkFeedFailed`) : c'est la date de la dernière tentative. Sans l'écriture en cas d'échec, `syncFeeds`, qui enchaîne les lots, reprendrait aussitôt un feed qui vient d'échouer.
- `ReleaseFeed` et `MarkFeedFailed` n'écrivent que si le claim tient encore (`fetching_since IS NOT NULL`). `MarkDueForSync` l'efface quand quelqu'un s'abonne pendant une synchro : celle-ci a lu la liste des abonnés avant lui, et écrire son ETag par-dessus la remise à zéro ferait du passage suivant un `304`, laissant le nouvel abonné sans articles.
- Les durées sont les règles de la synchro et vivent dans `internal/service/feedsync.go` ; la requête les reçoit en paramètres, `ClaimDueFeeds` est le seul appelant.
- `$2` est la fenêtre de lock (`syncLockWindow`, 10 min) : `fetching_since` remplace le booléen `is_fetching`, donc si un worker crash le lock expire automatiquement et le feed redevient éligible au prochain tick. Elle vaut `feedProcessTimeout` (8 min) + 2 min, donc une synchro lente mais vivante n'est jamais reprise par un autre worker.
- `$4` est `deadFeedRetryInterval` (24 h) : un feed désactivé reste éligible une fois par jour, compté depuis sa dernière tentative (`MarkFeedFailed` écrit `last_fetched_at` à chaque échec).
- `$3` est l'intervalle de rafraîchissement, dérivé de `SCHEDULER_INTERVAL` — c'est le même paramètre qui pilote le ticker, sinon régler `SCHEDULER_INTERVAL` n'aurait aucun effet sur la fréquence réelle. On applique une marge de 10 % (`syncIntervalSlack`) parce que `last_fetched_at` est écrit à la *fin* du sync, quelques secondes après le tick qui l'a lancé : sans cette marge, un feed raterait systématiquement le tick suivant et ne serait rafraîchi qu'un tick sur deux.

#### 4. La synchro d'un feed (`internal/service/feedsync.go`)

`FeedService` expose deux méthodes au scheduler, et rien d'autre : `ClaimDueFeeds` (réserver les feeds dus selon les règles de la synchro) et `SyncFeed` (synchroniser un feed réservé). Toutes les règles — seuil de désactivation, timeout, fenêtre de lock, marge, relance des feeds morts — sont des constantes de `feedsync.go`, passées en paramètres aux requêtes : aucune n'est écrite deux fois.

`SyncFeed` rend un `SyncResult` (`Status` : `not_modified | synced | failed | deactivated | cancelled`, plus `NewArticles`, `FailedItems`, `UnstorableItems`, `Failures`, `Err`, `StoreErr`) et rend **toujours** le claim, quelle que soit l'issue :

```text
SyncFeed(feed réservé) :
  1. Contexte déjà annulé (arrêt) → Unclaim, cancelled.
  2. Fetch avec If-None-Match / If-Modified-Since, sous feedProcessTimeout (8 min).
  3. 304 → ReleaseFeed avec les en-têtes stockés, not_modified.
  4. Erreur (transport, non-200, parse) → MarkFeedFailed(seuil) : failed, ou
     deactivated au seuil. Si c'est l'arrêt qui a coupé : Unclaim, cancelled,
     aucun échec compté.
  5. 200 → pour chaque item : dédup par hash, création de l'article si nouveau
     (readability, repli sur la description du flux), BULK INSERT des links pour
     tous les abonnés. Un item en échec est logué ("feed item failed", avec
     permanent=true|false) et compté dans FailedItems, ou dans UnstorableItems
     si la base l'a refusé pour son contenu.
  6. ReleaseFeed : nouveaux en-têtes de cache (un en-tête absent garde l'ancien),
     sauf si un item est dans FailedItems — on garde alors les anciens pour que
     la synchro suivante relise tout et retente l'item.
```

**Échec transitoire ou permanent.** Garder l'ETag fait relire tout le flux, et rescraper chaque item sans article, à chaque synchro. C'est le bon choix pour un timeout ou une base injoignable un instant, mais pas pour un item que la base refusera à chaque fois : l'ETag n'avancerait plus jamais. `internal/data` enveloppe donc dans `data.ErrUnstorable` les refus PostgreSQL des classes SQLSTATE 22 (donnée invalide) et 23 (contrainte violée), sur `Articles.Insert` et `Links.BulkInsertForArticle`, et `SyncFeed` les compte à part, sans retenir l'ETag. Exception : la clé étrangère (`23503`) reste transitoire, car elle vient d'un utilisateur ou d'un feed supprimé pendant la synchro, que la suivante ne verra plus. L'item refusé est retenté seulement quand le flux change.

**Construction de l'article.** `buildArticle` (`fetcher.go`) est la seule règle qui passe d'un item à un article, sans réseau ni base : le titre et le corps de la page extraite par readability, chacun remplacé par celui de l'item s'il est vide (`Content`, puis `Description`), et un corps que la conversion Markdown rejette gardé en texte brut. Chaque chaîne passe par `cleanArticleString` (octets NUL et UTF-8 invalide, que PostgreSQL refuse).

**Collision de slug.** Le slug d'un link est `hash[:12]`, unique par utilisateur. `BulkInsertForArticle` utilise un `ON CONFLICT DO NOTHING` sans cible, qui couvre `(user_id, article_id)` et `(user_id, slug)` : l'utilisateur qui détient déjà un autre article au même préfixe n'a pas celui-ci, au lieu que l'insert échoue pour tous les abonnés.

Une panique (le parseur readability sur du HTML hostile) est rattrapée par `SyncFeed` et enregistrée comme un échec. Le scheduler écrit une ligne `feed sync finished` par résultat, en INFO ; seul `StoreErr` — une écriture de notre côté qui n'a pas eu lieu — sort en ERROR.

**Arrêt de l'application.** Un fetch coupé par l'arrêt ne compte pas comme un échec du feed, et `Unclaim` rend le claim sans toucher `last_fetched_at` ni le compteur : au redémarrage, le feed est repris tout de suite au lieu d'attendre l'expiration du lock. Les feeds d'un lot réservé passent tous par `SyncFeed`, même après l'arrêt, puisque c'est lui qui rend le claim.

### Distribution des feeds aux workers

Les feeds récupérés par `GetFeedsToSync` (jusqu'à `SCHEDULER_BATCH_SIZE`) sont envoyés dans un channel buffered. Les `SCHEDULER_WORKERS` goroutines consomment ce channel en parallèle. Chaque worker traite un feed à la fois jusqu'à ce que le channel soit vide.

Un tick ne s'arrête pas au premier lot : `syncFeeds` réclame un nouveau lot tant que le précédent était plein, si bien que tous les feeds dus passent à chaque intervalle quel que soit leur nombre. Le lot ne sert plus qu'à borner le nombre de feeds verrouillés en même temps, pour que leur lock n'expire pas avant qu'un worker les atteigne. La boucle termine parce qu'un feed traité sort des feeds dus : succès comme échec écrivent `last_fetched_at`. Seules exceptions : un feed remis dû par un nouvel abonnement pendant sa synchro, qu'on veut justement reprendre, et l'arrêt, qui stoppe la boucle de toute façon.

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
| `SCHEDULER_BATCH_SIZE` | Nombre max de feeds réclamés à la fois (un tick enchaîne les lots jusqu'à épuisement des feeds dus) | `50` |

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