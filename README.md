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

## Stack technique

### Backend ([cmd/api/](cmd/api/))
- **Go 1.25** + [`httprouter`](https://github.com/julienschmidt/httprouter)
- **PostgreSQL** via [`lib/pq`](https://github.com/lib/pq)
- [`gofeed`](https://github.com/mmcdole/gofeed) pour le parsing RSS/Atom
- [`go-readability`](https://codeberg.org/readeck/go-readability) pour l'extraction de contenu
- [`bluemonday`](https://github.com/microcosm-cc/bluemonday) pour la sanitization HTML
- Auth par token Bearer (bcrypt + SHA256)

### Frontend ([frontend/](frontend/))
- **React 19** + **React Router v7** (mode SSR/SPA)
- **TailwindCSS v4**
- **TanStack Query** pour la gestion des données
- **react-hook-form** pour les formulaires
- Build embarqué dans le binaire Go via `go:embed`

### Base de données ([migrations/](migrations/))
- PostgreSQL avec triggers, index full-text (`tsvector`), CITEXT
- Schéma complet dans [docs/schema.sql](docs/schema.sql)

## Fonctionnalités

- Inscription / connexion par email
- Abonnement à des flux RSS/Atom
- Import automatique des articles au moment de l'abonnement
- Scraping du contenu (readability) avec fallback sur la description RSS
- État de lecture par utilisateur (lu, favori, archivé, progression)
- Slugs uniques par utilisateur
- Rate limiting par IP
- Routes SPA protégées (`/app/*`) et routes invitées (`/auth`, `/`)

## Architecture

```
┌──────────────────────┐
│  React Router SPA    │  (embarqué dans le binaire)
└──────────┬───────────┘
           │ fetch
┌──────────▼───────────┐
│   API Handlers       │  cmd/api/
│   Middleware / Auth  │
└──────────┬───────────┘
           │
┌──────────▼───────────┐
│   Service Layer      │  internal/service/
│   Fetcher / Scheduler│
└──────────┬───────────┘
           │
┌──────────▼───────────┐
│   Data Layer         │  internal/data/
└──────────┬───────────┘
           │
┌──────────▼───────────┐
│   PostgreSQL         │
└──────────────────────┘
```

Documentation détaillée : [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Endpoints principaux

| Méthode | Route | Auth | Description |
|---------|-------|------|-------------|
| `GET`   | `/v1/healthcheck` | Non | Vérification santé |
| `POST`  | `/v1/users` | Non | Inscription |
| `POST`  | `/v1/tokens/authentication` | Non | Connexion (token Bearer) |
| `GET`   | `/v1/users/me` | Oui | Utilisateur courant |
| `GET`   | `/v1/subscriptions` | Oui | Liste des abonnements RSS |
| `POST`  | `/v1/subscriptions` | Oui | S'abonner à un flux |
| `GET`   | `/v1/links` | Oui | Liste paginée des articles |
| `GET`   | `/v1/links/:slug` | Oui | Détail d'un article |

## Démarrage

### Prérequis

- Go 1.25+
- Node.js 22+ et pnpm
- PostgreSQL 14+
- [golang-migrate](https://github.com/golang-migrate/migrate) pour les migrations

### Variables d'environnement

Créer un fichier `.env` à la racine :

```env
ENV=development
PORT=4000
DATABASE_URI=postgres://user:password@localhost:5432/signet?sslmode=disable
RATE_LIMITER_RPS=2
RATE_LIMITER_BURST=4
RATE_LIMITER_ENABLED=true
```

### Migrations

```bash
make migrate-up        # Appliquer toutes les migrations
make migrate-down      # Rollback de la dernière
make migrate-version   # Version actuelle
make reset-db          # Drop + re-run complet
```

### Développement

```bash
# Frontend (port 5173, HMR)
cd frontend
pnpm install
pnpm dev

# Backend (port 4000)
go run ./cmd/api
```

### Build de production

```bash
# Build du frontend (sortie embarquée dans le binaire Go)
cd frontend && pnpm build && cd ..

# Build du binaire
go build -o bin/api ./cmd/api

# Lancement (sert aussi le frontend sur le port 4000)
./bin/api
```

### Devcontainer

Le projet fournit un [devcontainer](.devcontainer/devcontainer.json) (VSCode / GitHub Codespaces) avec Go, Node.js/pnpm et PostgreSQL déjà configurés. Il suffit d'ouvrir le projet dans un conteneur :

> *VSCode → `Dev Containers: Reopen in Container`*

Un [Dockerfile](Dockerfile) multi-stages est également disponible pour le build de production (frontend → binaire Go → image Alpine minimale). Le `docker-compose.yml` pour un déploiement complet (API + PostgreSQL) reste à faire.

## Structure du projet

```
signet/
├── cmd/api/            # Point d'entrée de l'API
├── internal/
│   ├── data/           # Couche d'accès aux données
│   ├── service/        # Logique métier, fetcher RSS
│   ├── readability/    # Extraction de contenu
│   ├── validator/      # Validation des entrées
│   └── jsonlog/        # Logging JSON structuré
├── frontend/           # Application React Router
├── migrations/         # Migrations SQL
├── docs/               # Documentation technique
├── frontend.go         # go:embed du build frontend
├── Dockerfile
└── Makefile
```

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — Architecture détaillée
- [docs/RSS_SYNC.md](docs/RSS_SYNC.md) — Flux de synchronisation RSS
- [docs/LIGHTPANDA_INTEGRATION.md](docs/LIGHTPANDA_INTEGRATION.md) — Intégration Lightpanda
- [docs/READABILITY_TESTING.md](docs/READABILITY_TESTING.md) — Tests du parser readability
- [docs/schema.sql](docs/schema.sql) — Schéma complet de la BDD

## Licence

MIT
