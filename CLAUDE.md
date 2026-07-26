# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

A self-hosted read-it-later app with native RSS/Atom support, inspired by the discontinued Omnivore. A single Go binary serves both the JSON API and the embedded React Router frontend, backed by PostgreSQL. Key design goals: deploy as one binary + one database, RSS as a first-class citizen, and deduplication (an article saved by N users is stored once).

The app is named **Signet**. The Go module is `github.com/flthibaud/signet` (root package `signet`), hosted at github.com/flthibaud/Signet.

## Commands

### Backend (Go 1.25)
```bash
go run ./cmd/api          # Run API server (reads .env; serves API + embedded frontend)
go build -o bin/api ./cmd/api
go test ./...             # All Go tests
go test ./internal/data   # Single package
go vet ./...
```

### Frontend (in `frontend/`, pnpm)
```bash
pnpm dev          # Vite dev server with HMR (port 5173)
pnpm build        # Production build -> frontend/build/client (embedded into Go binary)
pnpm typecheck    # react-router typegen && tsc
```
Note (per project memory): avoid `pnpm add` in the devcontainer; run typechecks via the direct binaries.

### Database migrations (golang-migrate, via Makefile)
```bash
make migrate-up               # Apply all
make migrate-down             # Roll back last
make migrate-create name=foo  # New sequential migration pair
make migrate-version
make migrate-force version=N  # Recover from a "Dirty database" error
make reset-db                 # Drop everything + re-run
```
The Makefile loads `.env` and uses `DATABASE_URI`.

## Architecture

Request flow: **React Router SPA → API handlers/middleware → service layer → data layer → PostgreSQL.**

- **`cmd/api/`** — HTTP layer. `main.go` wires config (from env), the data/service/scheduler layers, and an `application` struct that all handlers hang off. `routes.go` is the single source of truth for endpoints and for how the embedded SPA is served. `middleware.go` holds the middleware chain `recoverPanic → rateLimit → authenticate → router`.
- **`internal/data/`** — Data layer. `models.go` aggregates per-entity models (`Users`, `Feeds`, `Subscriptions`, `Links`, `Articles`, `Tokens`), each a struct wrapping `*sql.DB`. `ErrRecordNotFound` is the sentinel for "not found". This is the only layer that touches SQL.
- **`internal/service/`** — Business logic. `services.go` aggregates services (currently `FeedService`); `fetcher.go` fetches/parses RSS (gofeed), extracts article content (go-readability → markdown via `internal/readability`, sanitized with bluemonday) with fallback to the RSS description; `scheduler.go` is a background worker pool (started in `serve()`) that periodically syncs feeds with per-domain rate limiting. Article scraping goes through an anti-bot ladder — `pagefetch.go` (stdlib vs browser-impersonating TLS transports), `challenge.go` (interstitial detection), `solver.go` (FlareSolverr-compatible browser sidecar), `scrape.go` (escalation, solve budget, per-host memory); see `docs/ANTIBOT_FETCHING.md`. RSS polling always uses the stdlib client.
- **`frontend/`** — React 19 + React Router v7 in **SPA mode** (`ssr: false`), TailwindCSS v4, TanStack Query for server state, react-hook-form + zod for forms.

### Key cross-cutting conventions

- **Auth**: Bearer token *or* an httpOnly `auth_token` cookie (see `authenticate`). Tokens are validated and resolved to a user; unauthenticated requests get `data.AnonymousUser`. `requireAuth`/`requireGuest` guard the SPA HTML routes.
- **Embedded frontend**: `frontend.go` `go:embed`s `frontend/build/client/*`. The Go binary serves the SPA: `/app/*` requires auth, `/` and `/auth` are guest-only, everything else falls through to the static file server. **You must run `pnpm build` before the embed has up-to-date assets** (and before `go build`).
- **Same-origin API**: the frontend calls the API with relative paths (`/v1/...`) and `credentials: "include"`. There is no Vite proxy configured, so API calls work when the frontend is served same-origin by the Go binary; pure `pnpm dev` against a separate backend port would need a proxy or CORS added.
- **Error envelope**: handlers return errors as `{"error": ...}` — a string for generic errors, or a `{field: message}` map for 422 validation failures. The frontend's `apiFetch` (`frontend/app/lib/api.ts`) normalizes this into an `ApiError`; `applyApiError` maps field errors back onto react-hook-form.
- **Deduplication**: `articles` are stored once (deduped by content hash); `links` is the per-user join carrying read/favorite/archive state, progress, and a per-user unique slug. Unsubscribing deletes only the subscription, not the user's links.
- **Logging**: structured JSON via `internal/jsonlog` (`logger.PrintInfo/PrintError/PrintFatal`), not the standard logger.

## Configuration

Config is read from environment (`.env` auto-loaded). See `.env.example`. Required: `DATABASE_URI`. Others: `PORT`, `ENV`, `RATE_LIMITER_{RPS,BURST,ENABLED}`, `SCHEDULER_{INTERVAL,WORKERS,BATCH_SIZE}` (defaults 15m / 5 / 50).

A devcontainer (`.devcontainer/`) provides Go, Node/pnpm, and PostgreSQL. Further docs live in `docs/` (`ARCHITECTURE.md`, `RSS_SYNC.md`, `READABILITY_TESTING.md`, `schema.sql`).
