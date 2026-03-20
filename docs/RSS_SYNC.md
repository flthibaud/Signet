# Synchronisation automatique des flux RSS

## Table des matières

- [Objectif](#objectif)
- [Phase 1 — Scheduler intégré (goroutine + ticker)](#phase-1--scheduler-intégré-goroutine--ticker)
- [Phase 2 — Worker queue PostgreSQL (évolution future)](#phase-2--worker-queue-postgresql-évolution-future)

---

## Objectif

Aujourd'hui, les articles d'un flux RSS ne sont récupérés qu'au moment de la souscription (`POST /v1/subscriptions`), via un `go func()` fire-and-forget. Il n'y a aucun mécanisme de synchronisation périodique.

L'objectif est de mettre en place un système qui :

1. Récupère automatiquement les nouveaux articles de chaque flux RSS actif
2. Distribue les articles à **tous** les abonnés du flux (pas seulement le dernier inscrit)
3. Respecte les ressources (rate limiting, concurrence contrôlée)
4. S'intègre proprement au cycle de vie de l'application (graceful shutdown)

---

## Phase 1 — Scheduler intégré (goroutine + ticker)

### Architecture

```
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

#### 1. Scheduler (`internal/service/scheduler.go`)

Le scheduler est responsable de l'orchestration de la synchronisation périodique.

```go
type Scheduler struct {
    services  *Services
    logger    *jsonlog.Logger
    interval  time.Duration    // Intervalle entre chaque tick (ex: 15min)
    workers   int              // Nombre de workers concurrents (ex: 5)
    quit      chan struct{}     // Signal d'arrêt
    wg        sync.WaitGroup   // Attente de fin des workers
}
```

**Responsabilités :**

- Lancer un `time.Ticker` à l'intervalle configuré
- À chaque tick, récupérer la liste des feeds à synchroniser
- Distribuer les feeds dans un worker pool via un channel
- Respecter le graceful shutdown via le contexte et le channel `quit`

**Cycle de vie :**

```
Start(ctx) ─────> ticker.C reçu ─────> GetFeedsToSync()
                       │                      │
                       │               ┌──────▼──────┐
                       │               │  Envoyer    │
                       │               │  feeds dans │
                       │               │  le channel │
                       │               └──────┬──────┘
                       │                      │
                       │               ┌──────▼──────┐
                       │               │  Workers    │
                       │               │  traitent   │
                       │               │  chaque feed│
                       │               └─────────────┘
                       │
              ctx.Done() ─────> Stop() ─────> wg.Wait()
```

#### 2. Query `GetFeedsToSync` (`internal/data/feeds.go`)

Nouvelle méthode sur `FeedModel` pour récupérer les feeds éligibles à la synchronisation.

```sql
SELECT f.id, f.url, f.original_title, f.site_url, f.image_url,
       f.last_fetched_at, f.is_active, f.created_at
FROM feeds f
WHERE f.is_active = TRUE
  AND EXISTS (
      SELECT 1 FROM subscriptions s WHERE s.feed_id = f.id
  )
  AND (
      f.last_fetched_at IS NULL
      OR f.last_fetched_at < NOW() - INTERVAL '15 minutes'
  )
ORDER BY f.last_fetched_at ASC NULLS FIRST
LIMIT $1;
```

**Critères :**

- Le feed est actif (`is_active = TRUE`)
- Au moins un utilisateur est abonné
- Le dernier fetch date de plus de 15 minutes (ou jamais fetché)
- Les feeds les plus anciens sont traités en priorité
- Limite configurable pour ne pas surcharger un seul tick

#### 3. Distribution aux abonnés (`internal/service/fetcher.go`)

Nouvelle méthode `ImportArticlesForSubscribers()` qui étend `ImportArticles()` :

```
Pour chaque feed à synchroniser :
  1. Fetch le flux RSS
  2. Pour chaque item du flux :
     a. Vérifier si l'article existe (par hash)
     b. Si non, créer l'article (scraping + readability)
     c. Récupérer TOUS les abonnés du feed
     d. Pour chaque abonné, créer un link s'il n'existe pas
  3. Mettre à jour feed.last_fetched_at
```

#### 4. Query `GetSubscribersByFeed` (`internal/data/subscriptions.go`)

```sql
SELECT s.user_id
FROM subscriptions s
WHERE s.feed_id = $1;
```

#### 5. Intégration dans `main.go`

```go
// Démarrer le scheduler
scheduler := service.NewScheduler(app.services, app.logger, cfg.scheduler)
go scheduler.Start(ctx)

// Graceful shutdown
// Le scheduler s'arrête proprement quand le contexte est annulé
```

### Configuration

Nouvelles variables d'environnement :

| Variable | Description | Défaut |
|----------|-------------|--------|
| `SCHEDULER_INTERVAL` | Intervalle entre chaque sync | `15m` |
| `SCHEDULER_WORKERS` | Nombre de workers concurrents | `5` |
| `SCHEDULER_BATCH_SIZE` | Nombre max de feeds par tick | `50` |
| `SCHEDULER_ENABLED` | Activer/désactiver le scheduler | `true` |

### Gestion des erreurs

| Situation | Comportement |
|-----------|-------------|
| Feed HTTP timeout (30s) | Log l'erreur, passer au feed suivant |
| Feed RSS invalide | Log l'erreur, passer au feed suivant |
| Erreur scraping d'un article | Log l'erreur, continuer avec les autres articles |
| Erreur BDD | Log l'erreur, passer au feed suivant |
| Feed en erreur répétée | Désactiver le feed après N échecs consécutifs (à implémenter) |

### Observabilité

Logs structurés JSON à chaque tick :

```json
{
  "level": "INFO",
  "message": "sync completed",
  "properties": {
    "feeds_synced": 12,
    "articles_created": 34,
    "links_created": 78,
    "errors": 1,
    "duration_ms": 4520
  }
}
```

### Limites de cette approche

- **Pas de persistance des jobs** : si l'app redémarre pendant un sync, le travail en cours est perdu (mais la déduplication par hash rend le re-sync safe)
- **Single instance** : si plusieurs instances tournent, chaque instance lancera son propre scheduler (risque de doublons de travail, pas de verrouillage)
- **Pas de retry intelligent** : un feed en erreur sera simplement re-tenté au prochain tick
- **Pas de priorité dynamique** : tous les feeds sont traités de la même manière

---

## Phase 2 — Worker queue PostgreSQL (évolution future)

### Pourquoi évoluer ?

La phase 1 fonctionne bien pour un déploiement single-instance avec un nombre modéré de feeds. Les limites apparaissent quand :

- **Scaling horizontal** : plusieurs instances de l'API tournent → les schedulers se marchent dessus
- **Volume de feeds** : des milliers de feeds avec des fréquences de publication variées
- **Fiabilité** : besoin de retry avec backoff, dead letter queue, suivi des échecs
- **Observabilité** : besoin de savoir quels jobs sont en attente, en cours, échoués

### Architecture cible

```
┌────────────────┐     ┌────────────────┐     ┌────────────────┐
│   API Instance │     │   API Instance │     │   API Instance │
│       #1       │     │       #2       │     │       #3       │
│                │     │                │     │                │
│  ┌──────────┐  │     │  ┌──────────┐  │     │  ┌──────────┐  │
│  │ Worker   │  │     │  │ Worker   │  │     │  │ Worker   │  │
│  │ Client   │  │     │  │ Client   │  │     │  │ Client   │  │
│  └────┬─────┘  │     │  └────┬─────┘  │     │  └────┬─────┘  │
└───────┼────────┘     └───────┼────────┘     └───────┼────────┘
        │                      │                      │
        └──────────────────────┼──────────────────────┘
                               │
                    ┌──────────▼──────────┐
                    │     PostgreSQL      │
                    │                     │
                    │  ┌───────────────┐  │
                    │  │   jobs table  │  │
                    │  │               │  │
                    │  │ - id          │  │
                    │  │ - queue       │  │
                    │  │ - state       │  │
                    │  │ - args (JSON) │  │
                    │  │ - attempts    │  │
                    │  │ - max_retries │  │
                    │  │ - scheduled_at│  │
                    │  │ - locked_by   │  │
                    │  │ - locked_at   │  │
                    │  │ - completed_at│  │
                    │  │ - failed_at   │  │
                    │  │ - last_error  │  │
                    │  └───────────────┘  │
                    │                     │
                    │  SELECT ... FOR     │
                    │  UPDATE SKIP LOCKED │
                    │  (verrouillage)     │
                    └─────────────────────┘
```

### Librairie recommandée : River

[River](https://github.com/riverqueue/river) est une job queue Go native pour PostgreSQL, bien adaptée à ce cas d'usage :

- Utilise `pgx` (driver PostgreSQL performant)
- Verrouillage via `SELECT ... FOR UPDATE SKIP LOCKED` (pas de polling agressif)
- Retry avec backoff exponentiel configurable
- Scheduling périodique intégré (cron-like)
- Observabilité via table `river_job`
- Workers concurrents avec graceful shutdown
- Dead letter queue pour les jobs qui échouent trop

### Schéma de la table jobs

```sql
CREATE TABLE river_job (
    id          BIGSERIAL PRIMARY KEY,
    state       TEXT NOT NULL DEFAULT 'available',  -- available, running, completed, retryable, discarded
    queue       TEXT NOT NULL DEFAULT 'default',
    kind        TEXT NOT NULL,                       -- ex: 'feed_sync'
    args        JSONB NOT NULL,                      -- ex: {"feed_id": 42}
    attempt     INTEGER NOT NULL DEFAULT 0,
    max_attempts INTEGER NOT NULL DEFAULT 5,
    priority    INTEGER NOT NULL DEFAULT 1,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    attempted_at TIMESTAMPTZ,
    finalized_at TIMESTAMPTZ,
    errors      JSONB,                               -- Historique des erreurs
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_river_job_state_queue ON river_job(state, queue, scheduled_at);
```

### Types de jobs

#### 1. `FeedSyncJob` — Synchronisation d'un flux

```go
type FeedSyncArgs struct {
    FeedID int64 `json:"feed_id"`
}
```

**Comportement :**
- Fetch le flux RSS, crée les articles, distribue les links aux abonnés
- Retry : 3 tentatives avec backoff exponentiel (1min, 5min, 30min)
- Après 3 échecs : marquer le job comme `discarded`, log une alerte

#### 2. `FeedSyncSchedulerJob` — Enqueue les syncs (périodique)

Job cron qui s'exécute toutes les 15 minutes :

```go
// Toutes les 15 minutes, enqueue un FeedSyncJob pour chaque feed éligible
river.PeriodicJob{
    Schedule: "*/15 * * * *",
    Constructor: func() (river.JobArgs, *river.InsertOpts) {
        return FeedSyncSchedulerArgs{}, nil
    },
}
```

Ce job :
1. Récupère les feeds éligibles via `GetFeedsToSync()`
2. Enqueue un `FeedSyncJob` pour chacun
3. River se charge de la distribution aux workers disponibles

#### 3. `FeedHealthCheckJob` — Vérification de santé (futur)

Job périodique (toutes les heures) qui :
- Vérifie les feeds en erreur répétée
- Désactive les feeds morts (404, DNS failure)
- Envoie des notifications aux abonnés si un feed est désactivé

### Queues et priorités

| Queue | Priorité | Workers | Usage |
|-------|----------|---------|-------|
| `feed_sync` | 1 (normal) | 10 | Sync périodique des feeds |
| `feed_sync_immediate` | 2 (haute) | 5 | Sync à la souscription (résultat rapide pour l'utilisateur) |
| `maintenance` | 0 (basse) | 2 | Health checks, nettoyage |

### Avantages par rapport à la phase 1

| Aspect | Phase 1 (ticker) | Phase 2 (River) |
|--------|-------------------|-----------------|
| Multi-instance | Non (doublons de travail) | Oui (verrouillage PostgreSQL) |
| Retry | Re-tenté au prochain tick | Backoff exponentiel configurable |
| Observabilité | Logs uniquement | Table `river_job` requêtable |
| Priorisation | Non | Oui (queues + priorités) |
| Persistance | Non (perdu au restart) | Oui (jobs en BDD) |
| Scheduling | `time.Ticker` fixe | Expressions cron flexibles |
| Dead letter | Non | Oui (jobs `discarded`) |
| Complexité | Faible | Moyenne |
| Dépendances | Aucune | `riverqueue/river`, `pgx` |

### Migration phase 1 → phase 2

La migration est incrémentale :

1. **Ajouter River** : `go get github.com/riverqueue/river`
2. **Migrer le driver** : passer de `lib/pq` à `pgx` (River le requiert)
3. **Créer les workers** : encapsuler la logique existante de `FeedService` dans des `river.Worker`
4. **Remplacer le scheduler** : supprimer le ticker, configurer les periodic jobs River
5. **Supprimer l'ancien code** : retirer `scheduler.go` et la goroutine dans `main.go`

La logique métier (`ImportArticlesForSubscribers`, `GetFeedsToSync`) reste identique — seule l'orchestration change.

### Considérations pour la migration

- **Driver PostgreSQL** : River utilise `pgx` au lieu de `lib/pq`. Il faudra migrer le driver pour toute l'application ou maintenir les deux (non recommandé).
- **Migrations** : River fournit ses propres migrations pour créer ses tables internes.
- **Monitoring** : River expose une API Go pour lister/annuler des jobs. On peut l'exposer via un endpoint admin.

---

## Résumé des étapes d'implémentation (Phase 1)

1. Ajouter `GetFeedsToSync()` dans `internal/data/feeds.go`
2. Ajouter `GetSubscribersByFeed()` dans `internal/data/subscriptions.go`
3. Ajouter `ImportArticlesForSubscribers()` dans `internal/service/fetcher.go`
4. Créer `internal/service/scheduler.go` (Scheduler + worker pool)
5. Ajouter la config scheduler dans `main.go`
6. Lancer le scheduler au démarrage avec graceful shutdown
7. Ajouter les variables d'environnement

---

**Dernière mise à jour** : Mars 2026
