# Signet

Une application **read-it-later** self-hosted avec support natif des flux **RSS/Atom**, écrite en **Go** (API) et **React Router v7** (frontend).

## À propos

Ce projet est **inspiré d'[Omnivore](https://github.com/omnivore-app/omnivore)**, un excellent lecteur read-it-later open-source que j'utilisais au quotidien mais qui n'est **plus maintenu** depuis sa fermeture fin 2024 (suite au rachat par ElevenLabs).

L'objectif est de proposer une alternative légère, self-hostable, avec un seul binaire Go (frontend embarqué via `go:embed`) et une base PostgreSQL. L'accent est mis sur :

- **La simplicité de déploiement** : un binaire, une base de données, c'est tout
- **Le support RSS natif** : les flux RSS/Atom sont des citoyens de première classe
- **La déduplication** : un article sauvé par *N* utilisateurs n'est stocké qu'une seule fois
- **La lecture** : extraction du contenu via readability, sanitization, temps de lecture estimé

![feeds display on homepage](docs/resources/feeds-homepage.png "Feeds")

![feeds details](docs/resources/feed-details.png "Feeds Details")

## Fonctionnalités

- Inscription / connexion par email, avec token Bearer **ou** cookie httpOnly
- Abonnement à des flux RSS/Atom, import des articles dès l'abonnement
- **Synchronisation en tâche de fond** : pool de workers, toutes les 15 min, avec ETag / `If-Modified-Since` et rate limit par domaine
- **Extraction du contenu** via readability puis conversion en markdown, avec repli sur la description RSS
- **Fetch anti-bot** : fingerprint TLS de navigateur par défaut, sidecar navigateur optionnel pour les challenges JS ([docs/ANTIBOT_FETCHING.md](docs/ANTIBOT_FETCHING.md))
- **Déduplication** : l'article est stocké une fois, `links` porte l'état par utilisateur
- État de lecture par utilisateur : lu, favori, archivé, progression, slug unique
- Rate limiting par IP sur l'API (`/v1/*` uniquement, désactivé par défaut)

## Démarrage rapide

Le plus court chemin, tout en conteneurs :

```bash
cp .env.example .env
docker compose up -d --build
```

L'application écoute sur <http://localhost:8000>. Les migrations sont appliquées automatiquement avant le démarrage de l'API.

```bash
docker compose logs -f app
docker compose down          # ajouter -v pour supprimer aussi le volume Postgres
```

Le sidecar navigateur pour l'anti-bot est **optionnel** et derrière un profil, car il embarque un navigateur complet :

```bash
echo 'SOLVER_URL=http://solver:8191/v1' >> .env
docker compose --profile solver up -d
```

## Configuration

Toutes les variables ont un défaut sain : seule `DATABASE_URI` est obligatoire. Le binaire refuse de démarrer sur une valeur invalide plutôt que de retomber silencieusement sur zéro.

| Variable | Défaut | Description |
|---|---|---|
| `DATABASE_URI` | — | **Requis.** DSN PostgreSQL |
| `DATABASE_MAX_OPEN_CONNS` | `25` | Connexions ouvertes max (`0` = illimité) |
| `DATABASE_MAX_IDLE_CONNS` | `25` | Connexions inactives conservées |
| `DATABASE_MAX_IDLE_TIME` | `15m` | Durée avant fermeture d'une connexion inactive |
| `PORT` | `8000` | Port d'écoute HTTP |
| `ENV` | `` | Nom de l'environnement, remonté par le healthcheck |
| `RATE_LIMITER_ENABLED` | `false` | Rate limiting par IP sur `/v1/*` |
| `RATE_LIMITER_RPS` | `5` | Requêtes par seconde et par IP |
| `RATE_LIMITER_BURST` | `10` | Taille du seau |
| `SCHEDULER_INTERVAL` | `15m` | Période de synchronisation des flux |
| `SCHEDULER_WORKERS` | `5` | Workers de synchronisation concurrents |
| `SCHEDULER_BATCH_SIZE` | `50` | Flux traités par tick |
| `TLS_IMPERSONATE_ENABLED` | `true` | Fingerprint TLS de navigateur pour le scraping d'articles |
| `SOLVER_URL` | `` | Sidecar navigateur (contrat FlareSolverr) ; vide = désactivé |
| `SOLVER_TIMEOUT` | `60s` | Budget d'un solve navigateur |
| `SOLVER_MAX_PER_FEED` | `5` | Plafond de solves par run de flux |

Le rate limiter est **désactivé par défaut** : un self-hoster servant quelques utilisateurs de confiance n'en a pas besoin. Il ne s'applique qu'à `/v1/*`, jamais aux assets du SPA — les limiter renverrait des 429 au milieu d'un chargement de page.

## Développement

### Devcontainer

Le projet fournit un [devcontainer](.devcontainer/devcontainer.json) (VSCode / GitHub Codespaces) avec Go, Node.js/pnpm et PostgreSQL préconfigurés :

> *VSCode → `Dev Containers: Reopen in Container`*

### En local

Prérequis : Go 1.25+, Node.js 22+ avec pnpm, PostgreSQL 14+, et [golang-migrate](https://github.com/golang-migrate/migrate).

```bash
make migrate-up                       # appliquer les migrations
go run ./cmd/api                      # API + frontend embarqué, port 8000

cd frontend && pnpm install && pnpm dev   # dev server Vite, port 5173, HMR
```

En développement, travailler sur <http://localhost:5173> : le dev server Vite proxifie `/v1` vers l'API Go (`VITE_API_TARGET`, défaut `http://localhost:8000`). Le navigateur voit donc tout sur une même origine, ce qui fait fonctionner les cookies d'auth sans configuration CORS.

Le binaire Go sert le SPA **depuis le build embarqué** : pour voir des changements frontend sur le port 8000, il faut relancer `pnpm build` puis rebuilder le binaire.

```bash
make migrate-down                     # rollback de la dernière migration
make migrate-version                  # version actuelle
make reset-db                         # drop + re-run complet
go test ./...                         # tests Go
cd frontend && pnpm typecheck         # typegen react-router + tsc
```

### Build de production

```bash
cd frontend && pnpm build && cd ..    # obligatoire : alimente le go:embed
go build -o bin/api ./cmd/api
```

## Architecture

```
React Router SPA (embarqué)  →  cmd/api/        handlers, middleware, auth
                             →  internal/service/  fetcher RSS, scraping, scheduler
                             →  internal/data/     accès SQL
                             →  PostgreSQL
```

- **Go 1.25** + [`httprouter`](https://github.com/julienschmidt/httprouter), PostgreSQL via [`lib/pq`](https://github.com/lib/pq)
- [`gofeed`](https://github.com/mmcdole/gofeed) (RSS/Atom), [`go-readability`](https://codeberg.org/readeck/go-readability) (extraction), [`bluemonday`](https://github.com/microcosm-cc/bluemonday) (sanitization), [`tls-client`](https://github.com/bogdanfinn/tls-client) (fingerprint navigateur)
- **React 19** + **React Router v7** en **mode SPA** (`ssr: false`), **TailwindCSS v4**, **TanStack Query**, **react-hook-form** + **zod**
- Schéma PostgreSQL avec triggers et `citext`. Un index full-text `tsvector` (français, pondéré titre/description/contenu) est maintenu par trigger mais n'est **pas encore exposé** par un endpoint de recherche.

Détails dans [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Endpoints

| Méthode | Route | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/healthcheck` | Non | Vérification santé |
| `POST` | `/v1/users` | Non | Inscription |
| `POST` | `/v1/tokens/authentication` | Non | Connexion |
| `DELETE` | `/v1/tokens/authentication` | Oui | Déconnexion |
| `GET` | `/v1/users/me` | Oui | Utilisateur courant |
| `GET` | `/v1/subscriptions` | Oui | Liste des abonnements |
| `POST` | `/v1/subscriptions` | Oui | S'abonner à un flux |
| `DELETE` | `/v1/subscriptions/:id` | Oui | Se désabonner (conserve les articles) |
| `GET` | `/v1/links` | Oui | Articles, paginés — filtres `is_read`, `is_starred`, `archived`, `feed_id` |
| `GET` | `/v1/links/:slug` | Oui | Détail d'un article |
| `PATCH` | `/v1/links/:slug` | Oui | Mettre à jour l'état de lecture |

Les erreurs suivent une enveloppe unique : `{"error": ...}`, une chaîne pour les erreurs génériques, un objet `{champ: message}` pour les 422 de validation.

## Structure du projet

```
signet/
├── cmd/api/            # Handlers HTTP, middleware, routes
├── internal/
│   ├── data/           # Couche d'accès aux données (seule à faire du SQL)
│   ├── service/        # Fetcher RSS, scraping anti-bot, scheduler
│   ├── readability/    # Extraction de contenu → markdown
│   ├── validator/      # Validation des entrées
│   └── jsonlog/        # Logging JSON structuré
├── frontend/           # Application React Router (SPA)
├── migrations/         # Migrations SQL (golang-migrate)
├── docs/               # Documentation technique
├── frontend.go         # go:embed du build frontend
├── docker-compose.yml  # app + PostgreSQL + sidecar optionnel
├── Dockerfile          # build multi-stages
└── Makefile            # helpers de migration
```

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — Architecture détaillée
- [docs/RSS_SYNC.md](docs/RSS_SYNC.md) — Flux de synchronisation RSS
- [docs/ANTIBOT_FETCHING.md](docs/ANTIBOT_FETCHING.md) — Fetch anti-bot (TLS imperso, sidecar navigateur)
- [docs/READABILITY_TESTING.md](docs/READABILITY_TESTING.md) — Tests du parser readability
- [docs/schema.sql](docs/schema.sql) — Schéma complet de la BDD

## Licence

Signet est distribué sous licence **[GNU AGPL-3.0-or-later](LICENSE)**.

Copyright (C) 2026 Florian Thibaud

Concrètement : vous êtes libre d'utiliser, modifier et redistribuer Signet, y
compris pour vos propres besoins ou ceux de votre organisation. En contrepartie,
si vous distribuez une version modifiée **ou si vous la proposez à des tiers via
un réseau** (hébergement, SaaS), vous devez en publier le code source sous la
même licence.

L'auto-hébergement pour soi, sa famille ou son équipe n'impose rien : cette
clause vise la mise à disposition d'un service à des utilisateurs tiers.

Le nom et le logo **Signet** ne sont pas couverts par la licence — voir
[NOTICE.md](NOTICE.md).
