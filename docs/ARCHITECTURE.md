# Architecture Signet

## Table des matières

- [Vue d'ensemble](#vue-densemble)
- [Structure du projet](#structure-du-projet)
- [Architecture en couches](#architecture-en-couches)
- [API REST](#api-rest)
- [Authentification](#authentification)
- [Modèle de données](#modèle-de-données)
- [Flux d'import RSS](#flux-dimport-rss)
- [Déduplication](#déduplication)
- [Requêtes SQL courantes](#requêtes-sql-courantes)
- [Configuration](#configuration)

---

## Vue d'ensemble

Signet est un lecteur "read-it-later" avec support natif des flux RSS/Atom, écrit en Go. L'architecture repose sur une séparation claire entre :

- **Le contenu partagé** : Articles stockés une seule fois
- **Les données personnelles** : État de lecture et organisation par utilisateur
- **Les sources** : Flux RSS et abonnements

### Principes fondamentaux

1. **Un article = une entrée** : Le même article sauvé par 1000 utilisateurs n'est stocké qu'une fois
2. **Déduplication par hash** : Chaque URL unique a un hash SHA256 pour éviter les doublons
3. **Séparation contenu/métadonnées** : Le HTML/texte est séparé de l'état de lecture
4. **RSS first-class** : Les flux RSS sont une fonctionnalité centrale

---

## Structure du projet

```
signet/
├── cmd/
│   └── api/                    # Point d'entrée de l'API
│       ├── main.go             # Bootstrap, config, démarrage
│       ├── server.go           # Serveur HTTP avec graceful shutdown
│       ├── routes.go           # Définition des routes
│       ├── middleware.go       # Auth, rate limiting, panic recovery
│       ├── context.go          # Gestion du contexte (user)
│       ├── helpers.go          # Utilitaires JSON, parsing params
│       ├── errors.go           # Réponses d'erreur standardisées
│       ├── healthcheck.go      # Handler healthcheck
│       ├── users.go            # Handler inscription
│       ├── tokens.go           # Handler authentification
│       ├── subscriptions.go    # Handlers abonnements RSS
│       └── articles.go         # Handlers articles (en cours)
│
├── internal/
│   ├── data/                   # Couche d'accès aux données
│   │   ├── models.go           # Registre des modèles
│   │   ├── users.go            # CRUD utilisateurs
│   │   ├── articles.go         # CRUD articles
│   │   ├── feeds.go            # CRUD flux RSS
│   │   ├── subscriptions.go    # CRUD abonnements
│   │   ├── links.go            # CRUD liens (articles sauvés)
│   │   └── tokens.go           # Gestion des tokens
│   │
│   ├── service/                # Logique métier
│   │   ├── services.go         # Conteneur de services
│   │   └── fetcher.go          # Fetching RSS & scraping articles
│   │
│   ├── validator/              # Validation des entrées
│   │   └── validator.go        # Helpers de validation
│   │
│   └── jsonlog/                # Logging structuré
│       └── jsonlog.go          # Logger JSON
│
├── migrations/                 # Scripts de migration SQL
├── docs/
│   ├── schema.sql              # Schéma complet de la BDD
│   └── ARCHITECTURE.md         # Ce document
│
├── go.mod                      # Dépendances Go
└── Makefile                    # Commandes de build/run
```

---

## Architecture en couches

L'application suit une architecture en 3 couches :

```
┌─────────────────────────────────────────┐
│           API Handlers (cmd/api/)       │
│   Routes, Middleware, Validation HTTP   │
└────────────────────┬────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────┐
│      Service Layer (internal/service/)  │
│   Logique métier, Fetching, Scraping    │
└────────────────────┬────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────┐
│       Data Layer (internal/data/)       │
│    Modèles, Requêtes SQL, Validation    │
└────────────────────┬────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────┐
│              PostgreSQL                 │
└─────────────────────────────────────────┘
```

### Responsabilités

| Couche | Responsabilité |
|--------|----------------|
| **API** | Routing HTTP, validation des requêtes, sérialisation JSON, middleware |
| **Service** | Orchestration, logique métier complexe, appels externes (RSS, scraping) |
| **Data** | CRUD base de données, requêtes SQL, mapping struct/table |

---

## API REST

### Endpoints implémentés

| Méthode | Route | Auth | Description |
|---------|-------|------|-------------|
| `GET` | `/v1/healthcheck` | Non | Liveness : le process sert des requêtes, sans vérifier ses dépendances |
| `GET` | `/v1/readiness` | Non | Readiness : `PingContext` sur la base (timeout 2s), `503` si injoignable |
| `POST` | `/v1/users` | Non | Inscription d'un utilisateur |
| `POST` | `/v1/tokens/authentication` | Non | Connexion (obtenir un token) |
| `GET` | `/v1/subscriptions` | Oui | Liste des abonnements RSS |
| `POST` | `/v1/subscriptions` | Oui | S'abonner à un flux RSS |

### Endpoints prévus (non implémentés)

| Méthode | Route | Description |
|---------|-------|-------------|
| `DELETE` | `/v1/subscriptions/:id` | Se désabonner d'un flux |
| `GET` | `/v1/subscriptions/:id/articles` | Articles d'un flux |
| `GET` | `/v1/articles` | Liste des articles sauvés |
| `GET` | `/v1/articles/:slug` | Détail d'un article |
| `PATCH` | `/v1/articles/:slug` | Modifier état (lu, archivé, etc.) |

### Middleware

Les requêtes passent par 5 middleware dans cet ordre :

```go
secureHeaders → recoverPanic → rateLimit → authenticate → requireAuthenticatedUser → handler
```

1. **secureHeaders** : Pose les en-têtes de sécurité sur *toutes* les réponses (voir ci-dessous). En premier pour qu'un 429 ou une panic récupérée les portent aussi.
2. **recoverPanic** : Capture les panics et retourne une erreur 500
3. **rateLimit** : Limite par IP (configurable RPS + burst)
4. **authenticate** : Valide le token Bearer ou le cookie `auth_token`, attache l'user au contexte — un token absent ou périmé donne `data.AnonymousUser`, pas un 401 (les pages invité du SPA doivent rester accessibles)
5. **requireAuthenticatedUser** : Renvoie 401 sur tout `/v1/` pour un user anonyme. Les exceptions sont listées dans `publicAPIRoutes` : `GET /v1/healthcheck`, `GET /v1/readiness`, `POST /v1/users`, `POST` et `DELETE /v1/tokens/authentication` (le logout doit pouvoir expirer un cookie périmé). Une nouvelle route est donc protégée par défaut.

### En-têtes de sécurité

Le binaire sert lui-même le SPA : ces en-têtes sont sa responsabilité, pas celle d'un reverse proxy dont on ne peut rien supposer. `secureHeaders` les pose sur l'API comme sur les assets statiques.

| En-tête | Valeur |
|---------|--------|
| `Content-Security-Policy` | voir ci-dessous |
| `X-Content-Type-Options` | `nosniff` |
| `X-Frame-Options` | `DENY` (redondant avec `frame-ancestors`, gardé pour les vieux navigateurs) |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Strict-Transport-Security` | `max-age=31536000`, **uniquement en HTTPS** |

**HSTS** n'est envoyé que si la requête est arrivée en HTTPS (`r.TLS` non nil, ou `X-Forwarded-Proto: https` posé par le proxy qui termine TLS). Sans ce garde-fou, une installation en HTTP sur un hostname de LAN se verrouillerait elle-même pour un an. `X-Forwarded-Proto` est falsifiable, mais mentir dessus n'active HSTS que dans le navigateur du menteur. `HSTS_MAX_AGE=0` désactive l'en-tête ; `includeSubDomains` n'est pas posé par défaut, une instance à l'apex d'un domaine casserait les autres sous-domaines en HTTP.

**CSP** :

```
default-src 'self'; script-src 'self' 'nonce-<aléatoire>'; style-src 'self' 'unsafe-inline';
img-src 'self' data: https: http:; media-src 'self' https: http:; font-src 'self';
connect-src 'self'; frame-src 'none'; object-src 'none'; base-uri 'self';
form-action 'self'; frame-ancestors 'none'
```

- **`script-src` avec nonce** : le shell HTML produit par Vite s'amorce depuis des `<script>` inline, donc `'self'` seul ne suffit pas. Plutôt que `'unsafe-inline'` (qui annule à peu près l'intérêt de la CSP), `secureHeaders` génère un nonce par requête et `serveIndex` l'estampille sur chaque balise `<script>` du shell. Conséquence : le shell est servi en `Cache-Control: no-store` — une copie en cache rejouerait un nonce périmé contre un en-tête frais et la page resterait blanche. Il fait ~2 Ko, et les assets qu'il tire restent cachables (leur nom porte un hash).
- **`style-src 'unsafe-inline'`** : l'UI pose des styles depuis JavaScript ; serrer cette directive casserait des composants pour un gain marginal (l'injection de CSS est bien moins exploitable que celle de JS).
- **`img-src` / `media-src` larges** : le contenu des articles vient de sites arbitraires et est rendu avec ses images d'origine. `http:` reste autorisé pour les installations servies en clair ; en HTTPS le navigateur bloque de toute façon le contenu mixte.
- **`connect-src 'self'`** : le frontend n'appelle que l'API en same-origin (chemins relatifs `/v1/...`).

`cmd/api/middleware_test.go` vérifie le nonce contre le shell réellement embarqué : un build frontend qui émettrait ses `<script>` sous une forme que `addScriptNonce` ne reconnaît pas échoue en test, pas dans le navigateur.

### Sorties HTTP et SSRF

Le serveur fait des requêtes HTTP vers des URL fournies par les utilisateurs : l'URL du flux à l'abonnement, puis le `<link>` de chaque item du flux au moment du scraping. Sans garde-fou, ces deux chemins transforment l'application en proxy vers son propre réseau — et le second est le pire des deux, puisque le contenu récupéré est stocké dans `articles` et relu par l'attaquant via `GET /v1/links/:slug`. Sur un hébergeur cloud, `http://169.254.169.254/` rend des credentials IAM à qui les demande.

`internal/safedial` pose le contrôle dans `net.Dialer.Control` plutôt que dans un validateur d'URL, parce qu'un validateur ne voit que ce que l'utilisateur a soumis et se fait contourner de deux façons :

- **redirection** — une URL publique répondant `302` vers une adresse interne est suivie par le client HTTP, qui ne repasse pas par la validation ;
- **DNS rebinding** — un nom que l'attaquant contrôle résout vers une IP publique à la validation et vers `127.0.0.1` à la connexion.

`Control` s'exécute après la résolution DNS, sur l'adresse réellement composée, et à chaque connexion ouverte par la chaîne de redirections. Le hook est installé sur les trois clients Go : le client stdlib (polling RSS, `CreateFromURL`, favicon), le client TLS imperso (scraping) et son fallback.

Deux catégories de plages :

| | Bloqué par défaut | Rouvert par `FETCH_ALLOW_PRIVATE_NETWORKS=true` |
|---|---|---|
| RFC1918, CGNAT, ULA, loopback | oui | **oui** |
| Link-local (`169.254/16`, `fe80::/10`), multicast, réservé, 6to4/NAT64/Teredo | oui | **non** |

La deuxième ligne ne se rouvre jamais : un auto-hébergeur qui autorise son LAN pour s'abonner à un flux interne ne doit pas rouvrir au passage l'endpoint de métadonnées de son hébergeur. Les plages de transition IPv6 y figurent parce qu'elles encapsulent une adresse IPv4 que la pile déballe à l'émission — `2002:a9fe:a9fe::` et `64:ff9b::a9fe:a9fe` atteignent les métadonnées tout en ressemblant à de l'unicast global.

**Le sidecar navigateur est l'angle mort.** C'est un autre processus, qui fait sa propre résolution DNS ; aucun dialer de notre côté ne s'y applique. L'URL cible est vérifiée avant l'envoi (`Guard.CheckURL`), mais ce contrôle est intrinsèquement perdant face au rebinding. La vraie mesure est l'isolation réseau du conteneur — voir `docs/ANTIBOT_FETCHING.md`. Il reste désactivé par défaut.

### Format des réponses

**Succès** :
```json
{
  "subscription": {
    "id": 1,
    "feed": { ... },
    "unread_count": 42
  }
}
```

**Erreur** :
```json
{
  "error": "message d'erreur"
}
```

**Erreur de validation** :
```json
{
  "error": {
    "email": "must be a valid email address",
    "password": "must be at least 8 characters"
  }
}
```

---

## Authentification

### Flux de connexion

```
┌──────┐          ┌─────┐          ┌────┐
│Client│          │ API │          │ DB │
└──┬───┘          └──┬──┘          └─┬──┘
   │                 │               │
   │ POST /v1/users  │               │
   │ {email,password}│               │
   │────────────────>│               │
   │                 │ INSERT user   │
   │                 │ (bcrypt hash) │
   │                 │──────────────>│
   │                 │               │
   │   201 Created   │               │
   │<────────────────│               │
   │                 │               │
   │ POST /v1/tokens/authentication  │
   │ {email,password}│               │
   │────────────────>│               │
   │                 │ Verify hash   │
   │                 │──────────────>│
   │                 │               │
   │                 │ INSERT token  │
   │                 │ (24h expiry)  │
   │                 │──────────────>│
   │                 │               │
   │ {token: "xxx"}  │               │
   │<────────────────│               │
   │                 │               │
   │ GET /v1/subscriptions           │
   │ Authorization: Bearer xxx       │
   │────────────────>│               │
   │                 │ Validate token│
   │                 │──────────────>│
   │                 │               │
   │   200 OK        │               │
   │<────────────────│               │
└──┴───┘          └──┴──┘          └─┴──┘
```

### Détails techniques

- **Hachage mot de passe** : bcrypt (cost factor par défaut)
- **Token** : 26 caractères base32, stocké en SHA256 dans la BDD
- **Expiration** : 24 heures
- **Header** : `Authorization: Bearer <token>`

---

## Modèle de données

### Schéma relationnel

```
┌─────────┐       ┌──────────────┐       ┌───────┐
│  users  │───────│ subscriptions│───────│ feeds │
└────┬────┘       └──────────────┘       └───┬───┘
     │                                       │
     │            ┌───────┐                  │
     └────────────│ links │──────────────────┘
                  └───┬───┘
                      │
     ┌────────────────┼────────────────┐
     │                │                │
┌────▼────┐    ┌──────▼──────┐   ┌─────▼─────┐
│ articles│    │ link_labels │   │ highlights│
└─────────┘    └──────┬──────┘   └───────────┘
                      │
                ┌─────▼─────┐
                │  labels   │
                └───────────┘
```

### Tables principales

#### `users`

```sql
id            UUID PRIMARY KEY (auto-generated)
username      TEXT UNIQUE
email         CITEXT UNIQUE
password_hash BYTEA
created_at    TIMESTAMP
updated_at    TIMESTAMP
```

#### `articles`

Contenu partagé entre tous les utilisateurs.

```sql
id                    BIGINT PRIMARY KEY
url                   TEXT NOT NULL
hash                  TEXT UNIQUE        -- SHA256 de l'URL
title                 TEXT NOT NULL
description           TEXT
author                TEXT
image_url             TEXT
page_type             TEXT               -- 'article', 'video', 'pdf'
reading_time_minutes  REAL               -- Calculé automatiquement
original_html         TEXT               -- HTML brut (debug/fallback)
content               TEXT               -- HTML nettoyé
text_content          TEXT NOT NULL      -- Texte pour recherche
tsv                   TSVECTOR           -- Index full-text
published_at          TIMESTAMP
created_at            TIMESTAMP
updated_at            TIMESTAMP
```

#### `feeds`

Flux RSS/Atom disponibles.

```sql
id              BIGINT PRIMARY KEY
url             TEXT UNIQUE
original_title  TEXT
site_url        TEXT
image_url       TEXT
last_fetched_at TIMESTAMP
is_active       BOOLEAN DEFAULT TRUE
created_at      TIMESTAMP
```

#### `subscriptions`

Abonnements utilisateur ↔ flux.

```sql
id           BIGINT PRIMARY KEY
user_id      UUID REFERENCES users
feed_id      BIGINT REFERENCES feeds
custom_title TEXT               -- Override du titre
custom_icon  TEXT               -- Emoji ou URL
category     TEXT               -- "Tech", "News", etc.
created_at   TIMESTAMP

UNIQUE(user_id, feed_id)
```

#### `links`

Articles sauvés par utilisateur (état personnel).

```sql
id                             BIGINT PRIMARY KEY
user_id                        UUID REFERENCES users
article_id                     BIGINT REFERENCES articles
feed_id                        BIGINT REFERENCES feeds  -- NULL si sauvé manuellement
slug                           TEXT UNIQUE(user_id, slug)
is_read                        BOOLEAN DEFAULT FALSE
is_starred                     BOOLEAN DEFAULT FALSE
reading_progress               REAL    -- 0.0 à 1.0
reading_progress_anchor_index  INTEGER -- Index du paragraphe
saved_at                       TIMESTAMP
archived_at                    TIMESTAMP
created_at                     TIMESTAMP
updated_at                     TIMESTAMP

UNIQUE(user_id, article_id)
```

#### `labels`

Tags personnalisés.

```sql
id          BIGINT PRIMARY KEY
user_id     UUID REFERENCES users
name        TEXT
color       TEXT DEFAULT '#808080'
description TEXT
position    INTEGER            -- Ordre d'affichage
created_at  TIMESTAMP

UNIQUE(user_id, name)
```

#### `highlights`

Annotations sur les articles.

```sql
id             BIGINT PRIMARY KEY
user_id        UUID REFERENCES users
link_id        BIGINT REFERENCES links
quote          TEXT NOT NULL      -- Texte surligné
annotation     TEXT               -- Note personnelle
color          TEXT DEFAULT '#FFEB3B'
position_start INTEGER
position_end   INTEGER
created_at     TIMESTAMP
updated_at     TIMESTAMP
```

---

## Flux d'import RSS

### 1. Création d'un abonnement

```
User                API                    FeedService              DB
 │                   │                          │                    │
 │ POST /v1/subscriptions                       │                    │
 │ {url: "https://..."}                         │                    │
 │──────────────────>│                          │                    │
 │                   │                          │                    │
 │                   │ Valide URL               │                    │
 │                   │                          │                    │
 │                   │ Feed existe?             │                    │
 │                   │─────────────────────────────────────────────>│
 │                   │                          │                    │
 │                   │ Non → CreateFromURL()    │                    │
 │                   │─────────────────────────>│                    │
 │                   │                          │ HTTP GET RSS       │
 │                   │                          │───────────>        │
 │                   │                          │ Parse gofeed       │
 │                   │                          │ INSERT feed        │
 │                   │                          │───────────────────>│
 │                   │                          │                    │
 │                   │ INSERT subscription      │                    │
 │                   │─────────────────────────────────────────────>│
 │                   │                          │                    │
 │                   │ go ImportArticles()      │  (async)           │
 │                   │─────────────────────────>│                    │
 │                   │                          │                    │
 │  202 Accepted     │                          │                    │
 │<──────────────────│                          │                    │
```

### 2. Import des articles (async)

Pour chaque `<item>` du flux RSS :

```go
// 1. Calculer le hash de l'URL
hash := sha256(item.Link)

// 2. Vérifier si l'article existe
article, err := models.Articles.GetByHash(ctx, hash)

// 3. Si nouveau, scraper le contenu
if article == nil {
    // Tente readability, fallback sur RSS description
    parsed := fetchWithReadability(item.Link)
    article = createArticle(parsed, hash)
}

// 4. Créer le link pour l'utilisateur
if !linkExists(userID, article.ID) {
    slug := generateUniqueSlug(userID, article.Title)
    createLink(userID, article.ID, feedID, slug)
}
```

### Librairies utilisées

| Librairie | Usage |
|-----------|-------|
| `github.com/mmcdole/gofeed` | Parsing RSS/Atom |
| `codeberg.org/readeck/go-readability/v2` | Extraction du contenu (readability) |
| `github.com/microcosm-cc/bluemonday` | Sanitization HTML (XSS) |

---

## Déduplication

### Stratégie multi-niveaux

| Niveau | Table | Contrainte | Effet |
|--------|-------|------------|-------|
| 1 | `feeds` | `UNIQUE(url)` | 1 flux RSS par URL |
| 2 | `articles` | `UNIQUE(hash)` | 1 article par URL (hash SHA256) |
| 3 | `links` | `UNIQUE(user_id, article_id)` | 1 sauvegarde par user+article |
| 4 | `subscriptions` | `UNIQUE(user_id, feed_id)` | 1 abonnement par user+feed |

### Exemple

```
Alice sauve https://blog.com/article-x
  → articles.id = 1 (nouveau, hash = sha256(url))
  → links.id = 100 (Alice ↔ article 1)

Bob sauve https://blog.com/article-x
  → articles.id = 1 (existant via hash)
  → links.id = 101 (Bob ↔ article 1)

Économie : 1 article en BDD au lieu de 2
```

---

## Requêtes SQL courantes

### Articles non-lus d'un utilisateur

```sql
SELECT
    a.id, a.title, a.author, a.image_url, a.reading_time_minutes,
    l.saved_at, l.reading_progress,
    f.original_title AS feed_title
FROM links l
JOIN articles a ON l.article_id = a.id
LEFT JOIN feeds f ON l.feed_id = f.id
WHERE l.user_id = $1
  AND l.is_read = false
  AND l.archived_at IS NULL
ORDER BY l.saved_at DESC
LIMIT 50;
```

### Abonnements avec compteur non-lus

```sql
SELECT
    s.id, s.custom_title, s.category, s.created_at,
    f.id AS feed_id, f.url, f.original_title, f.site_url, f.image_url,
    COUNT(l.id) FILTER (WHERE l.is_read = FALSE) AS unread_count
FROM subscriptions s
JOIN feeds f ON s.feed_id = f.id
LEFT JOIN links l ON l.feed_id = f.id AND l.user_id = s.user_id
WHERE s.user_id = $1
GROUP BY s.id, f.id
ORDER BY s.created_at DESC;
```

### Recherche full-text

```sql
SELECT
    a.id, a.title, a.description,
    ts_rank(a.tsv, query) AS rank
FROM articles a,
     to_tsquery('french', $1) AS query
WHERE a.tsv @@ query
  AND EXISTS (
      SELECT 1 FROM links
      WHERE article_id = a.id AND user_id = $2
  )
ORDER BY rank DESC
LIMIT 20;
```

---

## Configuration

### Variables d'environnement

| Variable | Description | Exemple |
|----------|-------------|---------|
| `ENV` | Environnement | `development`, `production` |
| `PORT` | Port du serveur | `4000` |
| `DATABASE_URL` | Connection PostgreSQL | `postgres://user:pass@host/db` |
| `RATE_LIMITER_RPS` | Requêtes/seconde par IP | `2` |
| `RATE_LIMITER_BURST` | Capacité burst | `4` |
| `RATE_LIMITER_ENABLED` | Activer le rate limiting | `true` |
| `HSTS_MAX_AGE` | Durée HSTS en secondes, `0` pour désactiver | `31536000` |
| `FETCH_ALLOW_PRIVATE_NETWORKS` | Autorise les fetches vers le réseau privé | `false` |

### Structure application

```go
type application struct {
    config   config           // Configuration
    logger   *jsonlog.Logger  // Logging structuré JSON
    models   data.Models      // Accès BDD
    services service.Services // Logique métier
}

type config struct {
    port    int
    env     string
    db      struct { dsn string }
    limiter struct {
        rps     float64
        burst   int
        enabled bool
    }
}
```

---

## Dépendances principales

| Package | Version | Usage |
|---------|---------|-------|
| `github.com/julienschmidt/httprouter` | - | Routing HTTP |
| `github.com/lib/pq` | - | Driver PostgreSQL |
| `github.com/google/uuid` | - | Génération UUID |
| `golang.org/x/crypto/bcrypt` | - | Hachage mots de passe |
| `golang.org/x/time/rate` | - | Rate limiting |
| `github.com/mmcdole/gofeed` | - | Parsing RSS/Atom |
| `codeberg.org/readeck/go-readability/v2` | - | Extraction contenu web |
| `github.com/microcosm-cc/bluemonday` | - | Sanitization HTML |
| `github.com/joho/godotenv` | - | Chargement .env |

---

## Évolutions futures

### En cours

- [ ] Endpoints articles (listing, détail, modification état)
- [ ] Suppression d'abonnements
- [ ] Labels et highlights dans l'API

### Planifiées

- [ ] Cron job de synchronisation des feeds
- [ ] Sauvegarde manuelle d'articles (hors RSS)
- [ ] Recherche full-text via API
- [ ] Notifications nouveaux articles
- [ ] Export OPML

### À considérer

- [ ] Support newsletters (forward email)
- [ ] Apps mobiles natives
- [ ] Résumés IA des articles
- [ ] Partage public de collections

---

**Dernière mise à jour** : Janvier 2026
**Version** : 2.0.0
