# Signet

[![CI](https://github.com/flthibaud/Signet/actions/workflows/ci.yml/badge.svg)](https://github.com/flthibaud/Signet/actions/workflows/ci.yml)

A self-hosted **read-it-later** app with native **RSS/Atom** support, written in **Go** (API) and **React Router v7** (frontend).

## About

This project is **inspired by [Omnivore](https://github.com/omnivore-app/omnivore)**, an excellent open-source read-it-later reader I used daily, which has been **unmaintained** since it shut down in late 2024 (following the ElevenLabs acquisition).

The goal is a lightweight, self-hostable alternative: a single Go binary (frontend embedded via `go:embed`) and a PostgreSQL database. The focus is on:

- **Deployment simplicity**: one binary, one database, that's it
- **Native RSS support**: RSS/Atom feeds are first-class citizens
- **Deduplication**: an article saved by *N* users is stored only once
- **Reading**: content extraction via readability, sanitization, estimated reading time

![feeds display on homepage](docs/resources/feeds-homepage.png "Feeds")

![feeds details](docs/resources/feed-details.png "Feeds Details")

## Features

- Sign-up / sign-in by email, with a Bearer token **or** an httpOnly cookie
- Subscribe to RSS/Atom feeds, with articles imported as soon as you subscribe
- **Background synchronization**: worker pool, every 15 min by default (`SCHEDULER_INTERVAL`), with ETag / `If-Modified-Since` and per-domain rate limiting
- **Content extraction** via readability then conversion to markdown, falling back to the RSS description
- **Anti-bot fetching**: browser TLS fingerprint by default, optional browser sidecar for JS challenges ([docs/ANTIBOT_FETCHING.md](docs/ANTIBOT_FETCHING.md))
- **Deduplication**: the article is stored once, `links` carries the per-user state
- Per-user reading state: read, favorite, archived, progress, unique slug
- **Full-text search**, multilingual and accent-insensitive, with per-feed and per-date filters
- Per-IP rate limiting on the API (`/v1/*` only, disabled by default)

## Quick start

The shortest path, fully containerized:

```bash
cp .env.example .env
docker compose up -d
```

The application image is pulled from GHCR (`ghcr.io/flthibaud/signet`), published by CI: `latest` tracks `master`, `dev` tracks `develop`. Nothing is compiled locally, so this file works as-is on a NAS or a VPS with no toolchain. For a lasting deployment, pin a version tag in `SIGNET_TAG` rather than following `latest`: pulling a newer image applies its migrations on the next restart, and a migration does not replay backwards.

The app listens on <http://localhost:8000>. The binary embeds its SQL migrations and brings the database up to date itself at startup: there is no migration step to run beforehand, and no dedicated service in the compose file. An empty database is enough.

The application container runs as an unprivileged user with a read-only root filesystem (only `/tmp` is a tmpfs): nothing is written to disk, all state lives in Postgres. Its healthcheck queries `/v1/readiness`, which fails if the database is unreachable.

### Using your own PostgreSQL database

The compose file's `db` service is there to get you going: fixed credentials, no published port, reachable only by the app. To point at an existing database — managed, or already running — just set `DATABASE_URL` in `.env` and remove the `db` service from the compose file; nothing else refers to it:

```bash
DATABASE_URL="postgres://user:password@myhost:5432/signet?sslmode=require"
```

That is the only database setting: there are no separate host, user or password variables to keep in sync with one another.

```bash
docker compose logs -f app
docker compose pull && docker compose up -d   # update
docker compose down          # add -v to also delete the Postgres volume
```

To run the compose file against an image built from the working tree — testing a change before pushing it — tag it under the same name:

```bash
docker build -t ghcr.io/flthibaud/signet:local .
SIGNET_TAG=local docker compose up -d
```

The browser sidecar for anti-bot fetching is **optional** and behind a profile, since it ships a full browser:

```bash
echo 'SOLVER_URL=http://solver:8191/v1' >> .env
docker compose --profile solver up -d
```

## Configuration

Every variable has a sane default: only `DATABASE_URL` is required. The binary refuses to start on an invalid value rather than silently falling back to zero.

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | — | **Required.** PostgreSQL DSN |
| `DATABASE_MAX_OPEN_CONNS` | `25` | Max open connections (`0` = unlimited) |
| `DATABASE_MAX_IDLE_CONNS` | `25` | Idle connections kept around |
| `DATABASE_MAX_IDLE_TIME` | `15m` | Time before an idle connection is closed |
| `AUTO_MIGRATE` | `true` | Applies pending migrations at startup |
| `PORT` | `8000` | HTTP listen port |
| `ENV` | `` (`production` via compose) | Environment name; `production` enables the `Secure` flag on the auth cookie |
| `RATE_LIMITER_ENABLED` | `false` | Per-IP rate limiting on `/v1/*` |
| `RATE_LIMITER_RPS` | `5` | Requests per second per IP |
| `RATE_LIMITER_BURST` | `10` | Bucket size |
| `SCHEDULER_INTERVAL` | `15m` | Feed synchronization period |
| `SCHEDULER_WORKERS` | `5` | Concurrent sync workers |
| `SCHEDULER_BATCH_SIZE` | `50` | Feeds processed per tick |
| `TLS_IMPERSONATE_ENABLED` | `true` | Browser TLS fingerprint for article scraping |
| `SOLVER_URL` | `` | Browser sidecar (FlareSolverr contract); empty = disabled |
| `SOLVER_TIMEOUT` | `60s` | Budget for one browser solve |
| `SOLVER_MAX_PER_FEED` | `5` | Cap on solves per feed run |

The rate limiter is **disabled by default**: a self-hoster serving a handful of trusted users does not need it. It only applies to `/v1/*`.

## Development

### Devcontainer

The project ships a [devcontainer](.devcontainer/devcontainer.json) (VSCode / GitHub Codespaces) with Go, Node.js/pnpm and PostgreSQL preconfigured:

> *VSCode → `Dev Containers: Reopen in Container`*

### Locally

Requirements: Go 1.25+, Node.js 22+ with pnpm, PostgreSQL 14+. The [golang-migrate](https://github.com/golang-migrate/migrate) CLI is only needed for the `make migrate-*` targets below — creating a migration, rolling back, forcing a version. The server itself applies migrations on its own.

```bash
go run ./cmd/api                      # migrates, then serves the API + embedded frontend, port 8000

cd frontend && pnpm install && pnpm dev   # Vite dev server, port 5173, HMR
```

In development, work on <http://localhost:5173>: the Vite dev server proxies `/v1` to the Go API (`VITE_API_TARGET`, default `http://localhost:8000`). The browser therefore sees everything on a single origin, which makes the auth cookies work without any CORS configuration.

The Go binary serves the SPA **from the embedded build**: to see frontend changes on port 8000, you need to re-run `pnpm build` and rebuild the binary.

```bash
make migrate-create name=add_x        # new up/down pair
make migrate-down                     # roll back the last migration
make migrate-version                  # current version
make migrate-force version=N          # recover from a "Dirty database" state
make reset-db                         # full drop + re-run
go test ./...                         # Go tests
cd frontend && pnpm typecheck         # react-router typegen + tsc
```

The binary and the CLI share the same `schema_migrations` table: the two can be mixed with no risk of disagreeing on the version in place. If a migration breaks halfway through, the database is marked *dirty* and every subsequent startup fails with the steps to follow — fix the schema by hand, then `make migrate-force version=N`.

### Production build

```bash
cd frontend && pnpm build && cd ..    # required: feeds the go:embed
go build -o bin/api ./cmd/api
```

## Architecture

- **Go 1.25** + [`httprouter`](https://github.com/julienschmidt/httprouter), PostgreSQL via [`lib/pq`](https://github.com/lib/pq)
- [`gofeed`](https://github.com/mmcdole/gofeed) (RSS/Atom), [`go-readability`](https://codeberg.org/readeck/go-readability) (extraction), [`bluemonday`](https://github.com/microcosm-cc/bluemonday) (sanitization), [`tls-client`](https://github.com/bogdanfinn/tls-client) (browser fingerprint)
- **React 19** + **React Router v7** in **SPA mode** (`ssr: false`), **TailwindCSS v4**, **TanStack Query**, **react-hook-form** + **zod**
- PostgreSQL schema with triggers and `citext`. Full-text search relies on a multilingual, accent-insensitive `tsvector`, maintained by a generated column and weighted across title/description/content, exposed by `GET /v1/search`.

Details in [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Endpoints

| Method | Route | Auth | Description |
|---|---|---|---|
| `GET` | `/v1/healthcheck` | No | Liveness — the process responds (touches no dependency) |
| `GET` | `/v1/readiness` | No | Readiness — `200` if the database is reachable, `503` otherwise |
| `POST` | `/v1/users` | No | Sign-up |
| `POST` | `/v1/tokens/authentication` | No | Sign-in |
| `DELETE` | `/v1/tokens/authentication` | Yes | Sign-out |
| `GET` | `/v1/users/me` | Yes | Current user |
| `GET` | `/v1/subscriptions` | Yes | List subscriptions |
| `POST` | `/v1/subscriptions` | Yes | Subscribe to a feed |
| `DELETE` | `/v1/subscriptions/:id` | Yes | Unsubscribe (keeps the articles) |
| `GET` | `/v1/links` | Yes | Articles, paginated — `is_read`, `is_starred`, `archived`, `feed_id` filters |
| `GET` | `/v1/links/:slug` | Yes | Article detail |
| `PATCH` | `/v1/links/:slug` | Yes | Update the reading state |
| `GET` | `/v1/search` | Yes | Full-text search — see below |

### Full-text search

`GET /v1/search` queries the user's library through the `tsvector` index on
`articles` (title weighted A, description B, content C).

| Parameter | Description |
|---|---|
| `q` | Query (`websearch_to_tsquery` syntax: `"exact phrase"`, `-exclusion`, `or`). The last term is treated as a prefix, for search-as-you-type. Empty = recent articles, sorted by publication date. Between 2 and 200 characters. |
| `lang` | The searcher's locale (`fr-FR`, `en`…), for query stemming. Failing that, the `Accept-Language` header; failing that, the neutral configuration |
| `feed_id`, `is_read`, `is_starred` | Same filters as `/v1/links` |
| `archived` | Tri-state: absent = the whole library (archived included), unlike `/v1/links` |
| `since` | RFC3339 lower bound on the **publication date** — the client computes it in its own timezone |
| `page`, `page_size` | Pagination (default 20, max 100) |

The response does **not** return an exact total, but a `has_more` boolean. Counting
every result would force Postgres to walk all of them to produce a number whose
only useful part is whether there is a next page.

Relevance sorting cannot stop early: knowing the top 20 requires scoring every
result. Ranking therefore only covers the **1000 most recently published
matches** (`searchRankCandidates`), a bound that keeps the cost constant whatever
the size of the library. In exchange, a highly relevant article older than that
threshold can drop out of a very broad search — and pagination does not go beyond
1000 results.

The dates displayed and filtered on are **publication** dates, never import
dates: subscribing to a feed stamps every fetched article with the same second,
which would file a three-week-old article under "today". `links.published_at`
duplicates the column from `articles` so that this sort can use an index
(migration `000010`).

Each result carries a `snippet` produced by `ts_headline`, where the matched terms
are wrapped in `[[hl]]` / `[[/hl]]`. Those are text markers, never HTML: the
frontend splits on them to render a `<mark>` without injecting any tag.

### Multilingual and accents

Each article carries its own search configuration (`articles.language`), derived
from the feed's `<language>` at import time. Its `tsv` is **hybrid**: the text is
indexed twice, once with the article's language, once with the neutral `simple_ua`
configuration. On the query side, both halves are queried and combined with `|`.

That is what avoids having to guess the query's language: the neutral half matches
what the user literally typed, whatever the language, and the stemmed half adds
the morphology of **their** locale. A French speaker searching for
"objets connectes" thus finds an article containing "objet connecté" — without
`lang`, the same query returns nothing.

Every configuration goes through `unaccent` (suffix `_ua`), so "hebergee" finds
"hébergée" and "Zuge" finds "Züge", in both directions.

> **Known limitation — CJK.** Postgres's parser splits on whitespace: in Japanese
> and Chinese, a whole sentence becomes a single lexeme and partial search finds
> nothing. Those languages fall back to `simple_ua`, which does not fix them.
> Fixing them would require application-level bigrams or PGroonga (unavailable on
> stock Postgres). Korean, Arabic, Russian and every language with separators work
> normally.

Errors follow a single envelope: `{"error": ...}`, a string for generic errors, an object `{field: message}` for validation 422s.

## Project structure

```
signet/
├── cmd/api/            # HTTP handlers, middleware, routes
├── internal/
│   ├── data/           # Data access layer (the only one doing SQL)
│   ├── service/        # RSS fetcher, anti-bot scraping, scheduler
│   ├── readability/    # Content extraction → markdown
│   ├── validator/      # Input validation
│   └── jsonlog/        # Structured JSON logging
├── frontend/           # React Router application (SPA)
├── migrations/         # SQL migrations (golang-migrate)
├── docs/               # Technical documentation
├── frontend.go         # go:embed of the frontend build
├── migrations.go       # go:embed of the SQL migrations
├── docker-compose.yml  # app + PostgreSQL + optional sidecar
├── Dockerfile          # multi-stage build
└── Makefile            # migration helpers
```

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — Detailed architecture
- [docs/RSS_SYNC.md](docs/RSS_SYNC.md) — RSS synchronization flow
- [docs/ANTIBOT_FETCHING.md](docs/ANTIBOT_FETCHING.md) — Anti-bot fetching (TLS impersonation, browser sidecar)
- [docs/READABILITY_TESTING.md](docs/READABILITY_TESTING.md) — Readability parser tests
- [docs/schema.sql](docs/schema.sql) — Complete database schema

## License

Signet is distributed under the **[GNU AGPL-3.0-or-later](LICENSE)** license.

Copyright (C) 2026 Florian Thibaud

In practice: you are free to use, modify and redistribute Signet, including for
your own needs or those of your organization. In return, if you distribute a
modified version **or offer it to third parties over a network** (hosting, SaaS),
you must publish its source code under the same license.

Self-hosting for yourself, your family or your team requires nothing: that clause
targets making a service available to third-party users.

The **Signet** name and logo are not covered by the license — see
[NOTICE.md](NOTICE.md).
