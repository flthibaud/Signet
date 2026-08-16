# Architecture Signet

## Table des matières

- [Vue d'ensemble](#vue-densemble)
- [Structure du projet](#structure-du-projet)
- [Architecture en couches](#architecture-en-couches)
- [API REST](#api-rest)
- [Authentification](#authentification)
- [Modèle de données](#modèle-de-données)
- [Flux d'import RSS](#flux-dimport-rss)
- [Import / export OPML](#import--export-opml)
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
| `GET` | `/v1/config` | Non | Réglages de l'instance dont le SPA a besoin avant connexion (`registration_enabled`) |
| `POST` | `/v1/users` | Non | Inscription d'un utilisateur ; `403` si les inscriptions sont fermées |
| `POST` | `/v1/tokens/authentication` | Non | Connexion (obtenir un token) |
| `GET` | `/v1/subscriptions` | Oui | Liste des abonnements RSS, avec leur dossier |
| `POST` | `/v1/subscriptions` | Oui | S'abonner à un flux RSS |
| `PATCH` | `/v1/subscriptions/:id` | Oui | Ranger un flux dans un dossier (`folder_id`, `null` pour déclasser) |
| `DELETE` | `/v1/subscriptions/:id` | Oui | Se désabonner d'un flux |
| `GET` | `/v1/folders` | Oui | Liste des dossiers, par ordre alphabétique |
| `POST` | `/v1/folders` | Oui | Créer un dossier |
| `PATCH` | `/v1/folders/:id` | Oui | Renommer un dossier |
| `DELETE` | `/v1/folders/:id` | Oui | Supprimer un dossier ; ses flux repassent non classés |
| `POST` | `/v1/opml/import` | Oui | Importer une liste d'abonnements (corps = le fichier, 2 Mo max) → `202` + le job |
| `GET` | `/v1/opml/imports/latest` | Oui | Progression et bilan du dernier import |
| `GET` | `/v1/opml/export` | Oui | Exporter ses abonnements au format OPML |

### Endpoints prévus (non implémentés)

| Méthode | Route | Description |
|---------|-------|-------------|
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
3. **rateLimit** : Limite par IP client (configurable RPS + burst, voir ci-dessous)
4. **authenticate** : Valide le token Bearer ou le cookie `auth_token`, attache l'user au contexte — un token absent ou périmé donne `data.AnonymousUser`, pas un 401 (les pages invité du SPA doivent rester accessibles)
5. **requireAuthenticatedUser** : Renvoie 401 sur tout `/v1/` pour un user anonyme. Les exceptions sont listées dans `publicAPIRoutes` : `GET /v1/healthcheck`, `GET /v1/readiness`, `POST /v1/users`, `POST` et `DELETE /v1/tokens/authentication` (le logout doit pouvoir expirer un cookie périmé). Une nouvelle route est donc protégée par défaut.

### Identification du client derrière un proxy

`rateLimit` a besoin de savoir *qui* parle. L'adresse de la connexion TCP ne le dit pas dès qu'un reverse proxy est devant : elle vaut alors celle du proxy, identique pour tout le monde, et `RATE_LIMITER_RPS` devient un plafond partagé par l'instance entière — un seul abuseur throttle tous les utilisateurs.

Les proxys consignent ce qu'ils ont vu dans `X-Forwarded-For`, **en ajoutant à droite** au fur et à mesure que la requête progresse vers l'intérieur. Les entrées les plus à droite sont donc celles écrites par notre propre infrastructure ; celles de gauche peuvent avoir été forgées par le client. Lire l'en-tête naïvement est pire que ne pas le lire : il suffirait de faire varier la valeur de gauche pour obtenir un bucket neuf à chaque requête, et le limiteur ne limiterait plus rien.

`TRUSTED_PROXY_COUNT` déclare combien de sauts, en partant de la droite, sont les nôtres. `clientIP` lit la Nième entrée depuis ce bord : une valeur forgée n'est que poussée vers la gauche par chaque ajout, là où on ne regarde jamais.

```
TRUSTED_PROXY_COUNT=1
  XFF: "203.0.113.7"                → 203.0.113.7   (le proxy a consigné le client)
  XFF: "1.2.3.4, 203.0.113.7"       → 203.0.113.7   (le client a forgé "1.2.3.4")

TRUSTED_PROXY_COUNT=2               (un CDN devant le proxy local)
  XFF: "203.0.113.7, 172.16.0.1"    → 203.0.113.7
```

Un compteur plutôt qu'une liste d'adresses de confiance à la `set_real_ip_from` : sous Docker ou sur un PaaS, l'adresse du proxy est une IP de conteneur qui change au redéploiement. La valeur à poser est le nombre de proxys qui ajoutent un `X-Forwarded-For` entre Internet et le binaire — `1` pour un Traefik ou un Coolify seul, `2` avec un Cloudflare devant. Le défaut `0` ignore l'en-tête et conserve le comportement d'une instance directement exposée.

Se tromper n'est pas symétrique : **trop haut** est sans danger (la chaîne est plus courte que N, on retombe sur l'adresse de la connexion), **trop bas** fait lire une entrée écrite par un maillon non fiable, donc falsifiable. En cas de doute, surestimer.

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
   │                 │ (SESSION_TTL) │
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
- **Token** : 16 octets d'entropie encodés en base32 (26 caractères), stocké en SHA256 dans la BDD. La longueur attendue par `ValidateTokenPlaintext` est dérivée de l'encodage, pas écrite en dur.
- **Header** : `Authorization: Bearer <token>` ou cookie httpOnly `auth_token`

### Ouverture des inscriptions

`POST /v1/users` est publique — elle doit l'être, personne n'a de token avant d'avoir un compte — donc n'importe qui trouvant l'URL d'une instance pourrait s'y inscrire. `REGISTRATION_ENABLED` ferme la porte, et elle est **fermée par défaut** : une instance auto-hébergée n'a pas à accepter des comptes que son propriétaire n'a pas voulus.

Reste le problème d'amorçage : il n'existe aucune CLI de création d'utilisateur, donc une install neuve avec le réglage par défaut serait inaccessible à son propre installateur. D'où l'exception — tant que la table `users` est vide, l'inscription passe quand même (`Users.HasAny`). Le parcours par défaut est donc : on installe, on crée son compte, l'instance se ferme d'elle-même juste après. Deux inscriptions simultanées sur une base vide peuvent toutes deux aboutir ; ce sont deux comptes créés par la même personne sur sa propre instance, la course est assumée.

Le contrôle est la toute première chose que fait `registerUserHandler`, avant même de lire le corps de la requête, pour la même raison que le mot de passe est validé avant d'être haché : un appelant refusé ne doit pas coûter un tour de bcrypt.

Le SPA lit `GET /v1/config` pour savoir s'il propose l'onglet « Create account ». Cet endpoint ignore l'exception d'amorçage et renvoie `false` sur une base vide, alors que l'API accepterait un compte : l'exception est un filet de sécurité pour qui installe, pas une invitation affichée aux visiteurs. Le `403` reste de toute façon le vrai garde-fou — masquer un onglet n'empêche pas un `curl`.

### Durée de vie des sessions

`SESSION_TTL` (30 jours par défaut) est un **délai d'inactivité**, pas une échéance ferme. `authenticate` lit l'expiration du token en même temps que l'user — la requête ne coûte pas d'aller-retour supplémentaire — et `refreshSession` la repousse à `now + SESSION_TTL` dès qu'il reste moins de 90 % de la durée de vie. Une session active coûte donc un `UPDATE` tous les trois jours environ, et n'expire jamais tant qu'elle sert ; une session laissée de côté plus longtemps que le TTL est perdue. Quand le token vient du cookie, celui-ci est réémis avec le nouveau `Max-Age` pour que navigateur et BDD restent d'accord.

Le refresh se fait avant que le handler ne s'exécute : un `Set-Cookie` doit partir avant que quoi que ce soit n'écrive un corps de réponse. Un échec est journalisé puis ignoré — la requête, elle, s'est bien authentifiée. Il ne s'applique qu'aux requêtes sous `/v1/` : `authenticate` couvre aussi les assets statiques du SPA, et accrocher un `Set-Cookie` à une réponse qu'un intermédiaire peut mettre en cache est le meilleur moyen de servir le token d'un user à un autre.

### Déconnexion

`DELETE /v1/tokens/authentication` ne supprime que le token présenté par la requête : `authenticate` dépose son hash dans le contexte (le hash, pas le plaintext, pour que le secret s'arrête au middleware) et le handler fait un `DeleteByHash`. Se déconnecter du téléphone laisse donc le laptop connecté. `Tokens.DeleteAllForUser` reste disponible pour un futur « se déconnecter partout ».

### Nettoyage

Rien d'autre ne purge la table `tokens` : les rangées périmées n'authentifient plus personne mais s'accumuleraient indéfiniment. Le scheduler lance donc, sur sa propre goroutine, un `DELETE ... WHERE expiry < now()` toutes les heures (index `idx_tokens_expiry`), indépendamment du tick de synchro des feeds.

---

## Modèle de données

### Schéma relationnel

```
┌─────────┐       ┌──────────────┐       ┌───────┐
│  users  │───────│ subscriptions│───────│ feeds │
└────┬────┘       └───────┬──────┘       └───┬───┘
     │                    │                  │
     │              ┌─────▼─────┐            │
     │              │  folders  │            │
     │              └───────────┘            │
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
folder_id    BIGINT REFERENCES folders ON DELETE SET NULL  -- NULL = non classé
created_at   TIMESTAMP

UNIQUE(user_id, feed_id)
```

#### `folders`

Dossiers d'abonnements, à plat comme chez Feedly. Ils naissent d'un import OPML
ou de la sidebar, qui les crée, les renomme et les supprime via `/v1/folders`.

Ils n'ont pas de couleur : ce sont les flux qui portent une identité visuelle,
avec le favicon de `feeds.image_url`.

`folder_id NULL` **est** la catégorie « Uncategorized » : c'est un regroupement
que l'UI applique, pas une ligne. Une ligne sentinelle obligerait à la créer
pour chaque inscrit, à interdire sa suppression, et à l'exclure à l'écriture de
l'OPML — les lecteurs sortant les flux non classés à la racine. C'est aussi
pourquoi « Uncategorized » n'est pas cliquable dans la sidebar, là où un dossier
mène à `/app?folder_id=` : il n'y a pas d'identifiant sur lequel filtrer.

Supprimer un dossier ne désabonne jamais (`ON DELETE SET NULL`) : ses flux
repassent simplement non classés. `Folders.Get` sert de garde d'appartenance
avant `Subscriptions.SetFolder` — plier cette vérification dans une sous-requête
de l'`UPDATE` écrirait `NULL` quand le dossier appartient à quelqu'un d'autre,
déclassant l'abonnement au lieu de refuser l'écriture.

```sql
id         BIGINT PRIMARY KEY
user_id    UUID REFERENCES users ON DELETE CASCADE
name       TEXT
created_at TIMESTAMP

UNIQUE(user_id, name)   -- rend FolderModel.GetOrCreate idempotent
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
// 1. Résoudre le lien de l'item (fallback <guid> permalien) puis le hasher
//    sous sa forme normalisée
link := itemURL(item.Link, item.GUID)
if link == "" {
    continue // rien à dédupliquer ni à scraper
}
hash := sha256(normalizeURL(link))

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

## Import / export OPML

L'OPML est le format d'échange de tous les lecteurs RSS : sans lui, personne ne
migre depuis Feedly ou Inoreader sans ressaisir ses URLs, et s'abonner dans
Signet serait un aller sans retour. `internal/opml` lit et écrit le format, sans
rien connaître des modèles.

### Pourquoi l'import est un job et pas une requête

Un OPML de 200 flux, c'est 200 fetchs HTTP (`FeedService.CreateFromURL`) : aucun
client n'attend ça. `POST /v1/opml/import` crée donc une ligne `opml_imports`,
lance le travail en tâche de fond et rend `202`. Le front suit via
`GET /v1/opml/imports/latest`, qu'il interroge toutes les deux secondes tant que
le job tourne.

La ligne **survit à la fin du job** : c'est elle qui porte le bilan
(`38 importés · 2 déjà abonnés · 2 en échec`, avec le détail des échecs), et un
rechargement de page doit pouvoir le retrouver. Pour autant la table ne grossit
pas : `Insert` supprime les imports précédents du même utilisateur, ce qui la
borne au nombre de comptes sans aucun travail de fond.

```
POST /v1/opml/import
  ├── opml.Parse            outlines imbriqués aplatis, doublons retirés
  ├── OPMLImports.Insert    remplace l'import précédent → 202
  └── goroutine détachée
        ├── Folders.GetOrCreate   une fois par nom distinct, séquentiellement
        ├── 4 workers → SubscriptionService.Subscribe(..., DeferImport: true)
        │     └── AppendResult    compteurs + ligne de rapport, un seul UPDATE
        ├── Feeds.MarkDueForSync  sur les flux souscrits
        ├── Scheduler.TriggerSync réveille le pool de sync
        └── MarkFinished          completed | interrupted
```

### Les deux pièges de l'import en masse

**La ruée des imports d'articles.** `Subscribe` lance normalement l'import des
articles du flux en tâche de fond. Sur 200 lignes d'OPML, ce sont 200 goroutines
qui fetchent et scrapent des flux entiers en même temps. D'où
`SubscribeOptions.DeferImport` : l'import OPML marque les flux comme dus puis
réveille le scheduler **une fois**, et c'est son pool de workers — avec son rate
limiting par domaine — qui fait le travail. Un tick ne traitant que `batchSize`
flux, un gros import se remplit sur plusieurs passes.

**Le 304 qui vide la bibliothèque.** S'abonner à un flux que l'instance connaît
déjà envoie un `If-None-Match` avec l'ETag stocké. Le serveur répond `304`,
`ImportArticlesForSubscribers` retourne avant la boucle qui crée les `links`, et
le nouvel abonné n'a **aucun article** — jusqu'à ce que l'éditeur publie. D'où
`Feeds.MarkDueForSync`, qui remet `last_fetched_at` à l'époque et vide
`http_etag` / `http_last_modified` pour forcer un `200`. Elle est appelée aussi
bien par l'import OPML que par `Subscribe` sur un flux préexistant, ce qui
corrige le même trou sur le chemin unitaire.

### Reprise après crash

Un job dont le processus meurt resterait `running` pour toujours. Le ménage
horaire du scheduler (`housekeeping`) appelle `OPMLImports.FailStale` : tout job
dont `updated_at` n'a pas bougé depuis 30 minutes passe `interrupted`.
`updated_at` est un vrai battement de cœur — le trigger
`update_opml_imports_modtime` le rafraîchit à chaque `AppendResult`, donc à
chaque flux traité. Un `SIGTERM` propre n'attend pas ce balayage : `Shutdown`
annule le contexte et le job s'enregistre lui-même comme `interrupted`, via un
contexte détaché de l'annulation.

### Dossiers et profondeur

OPML 2.0 autorise une imbrication arbitraire. La quasi-totalité des lecteurs
exportent un seul niveau, mais NewsBlur et Thunderbird imbriquent. Les dossiers
de Signet étant plats, `opml.Parse` joint le chemin des ancêtres :
`Tech > Langages > Go` devient le dossier `"Tech / Langages / Go"` (5 niveaux
maximum). `opml.Write` réécrit un seul niveau, nom tel quel : ce qui est affiché
est ce qui est exporté.

### Limites volontaires

| Garde | Valeur | Raison |
|-------|--------|--------|
| Taille du corps | 2 Mo | Mille flux tiennent bien en dessous |
| Nombre d'entrées | 1000 | Chaque entrée coûte un fetch : c'est autant un rate limit qu'une limite de taille |
| Workers d'import | 4 | Borne ce qu'un seul utilisateur fait marteler au réseau |
| Schémas d'URL | `http(s)` uniquement | Le reste de la défense SSRF est assuré par `internal/safedial` |

---

## Déduplication

### Stratégie multi-niveaux

| Niveau | Table | Contrainte | Effet |
|--------|-------|------------|-------|
| 1 | `feeds` | `UNIQUE(url)` | 1 flux RSS par URL |
| 2 | `articles` | `UNIQUE(hash)` | 1 article par URL (hash SHA256 de l'URL normalisée) |
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

### Normalisation de l'URL

Le hash porte sur une forme canonique de l'URL, pas sur la chaîne brute (`normalizeURL`, `internal/service/urlhash.go`). Deux flux qui pointent la même page l'épellent rarement pareil, et sans normalisation `http://` vs `https://`, un `www.`, un slash final, un `#ancre`, un `?utm_source=…` ou un simple ordre de paramètres différent suffisent à créer deux articles — le stockage unique promis ne tient alors plus entre flux.

Sont donc ramenés à une forme unique : le schéma (`http` est replié sur `https`, les deux servant en pratique le même document), la casse du host, le `www.`, les ports par défaut, le fragment, les identifiants d'URL, le slash final, l'ordre des paramètres, et les paramètres de campagne (`utm_*`, `pk_*`, `fbclid`, `gclid`…). Le reste de la query est **conservé** : beaucoup de sites adressent encore leur contenu par paramètre (`?p=42`), et fusionner deux articles distincts est bien pire que rater une déduplication. Une URL relative, malformée ou non-HTTP est laissée telle quelle : aucune forme canonique n'est fiable dans ce cas.

Le résultat sert uniquement de clé : c'est le lien d'origine qui est stocké dans `articles.url` et scrapé.

Enfin, un item sans lien exploitable est ignoré. Un `<link>` vide hashait la chaîne vide, donc *tous* les items sans lien, tous flux confondus, fusionnaient en un seul article. `itemURL` retombe d'abord sur le `<guid>` quand celui-ci est une URL HTTP absolue (le cas `isPermaLink="true"`), et l'item est sauté sinon — sans marquer le flux en échec, puisqu'un retry échouerait à l'identique et bloquerait l'ETag.

> **Migration** : le hash étant calculé différemment, les articles déjà en base ne sont plus retrouvés. Les items encore dans la fenêtre du flux au moment de la mise à jour sont ré-importés une fois sous un nouveau hash (doublon ponctuel pour les abonnés) ; l'ancienne ligne reste et le régime permanent est correct ensuite.

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
| `REGISTRATION_ENABLED` | Ouverture des inscriptions (fermé par défaut) | `true` |
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
