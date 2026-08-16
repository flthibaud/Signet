# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

A self-hosted read-it-later app with native RSS/Atom support, inspired by the discontinued Omnivore. A single Go binary serves both the JSON API and the embedded React Router frontend, backed by PostgreSQL. Key design goals: deploy as one binary + one database, RSS as a first-class citizen, and deduplication (an article saved by N users is stored once).

The app is named **Signet**. The Go module is `github.com/flthibaud/signet` (root package `signet`), hosted at github.com/flthibaud/Signet.

**Project stage — nothing is deployed and nobody uses this yet; local development only.** So there is no backwards compatibility to preserve: a migration may rewrite or drop an existing column instead of being additive, a refactor may break any earlier local install, and a schema or API change needs no deprecation path or compatibility shim. Do not add an abstraction layer for a future that has not arrived — prefer the simplest design that fits the code as it is today over one that keeps an older one working.

## Workflow

`develop` is the integration branch: work branches off it as `feat/*` or `fix/*` and comes back through a PR into `develop` — **not** into `master`, which is the release branch. CI publishes the `dev` image tag from `develop` and `latest` from `master`, so cutting a release is a `develop` → `master` merge. A change a user or an operator can see also updates `README.md`, and `docs/ARCHITECTURE.md` when it changed how the pieces fit together.

## Commands

### Backend (Go 1.25)
```bash
go run ./cmd/api          # Run API server (reads .env; serves API + embedded frontend)
go build -o bin/api ./cmd/api
go test ./...             # All Go tests
go test ./internal/data   # Single package
go vet ./...
```
Every Go command needs `frontend/build/client`, which is gitignored: `frontend.go` `go:embed`s it, so on a fresh clone `go build`, `go vet` and `go test ./...` all fail until `pnpm build` has run once.

**Tests.** `internal/data` runs real queries against `TEST_DATABASE_URL` (falling back to `DATABASE_URL`) and **skips** when neither is set — `testdb_test.go` holds the fixtures, `*_db_test.go` the tests, and the fixtures only touch rows they own. Any new query in that package wants one: a `SELECT` list that has drifted from its `Scan`, or a struct field nothing scans into, is invisible to the compiler and to tests that only assert on SQL strings. `cmd/api` tests handlers without a database — a bare `application`, an `httptest` request, the user and route params on its context — which covers everything that returns before the SQL.

CI (`.github/workflows/ci.yml`) builds the frontend first, then adds **staticcheck**, `govulncheck`, `go test -race` and a `go mod tidy` diff check on top of the commands above. It has no database, so the `internal/data` tests skip there.

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
make migrate-goto version=N   # Migrate up or down to a specific version
make migrate-force version=N  # Recover from a "Dirty database" error
make reset-db                 # Drop everything + re-run
```
The Makefile loads `.env`, uses `DATABASE_URL` and needs the golang-migrate CLI (the server itself does not). These targets are for local schema work; **deployments do not need them**. `migrations.go` embeds `migrations/*.sql` into the binary and `cmd/api/migrate.go` applies anything pending at startup (`AUTO_MIGRATE`, on by default), which is why `docker-compose.yml` has no migration service. Both paths share the standard `schema_migrations` table, so they can be mixed. `./api --migrate-only` migrates and exits, for a PaaS release command or a job ahead of a multi-replica rollout.

A new migration must be a complete `NNNNNN_name.{up,down}.sql` pair — the embed glob takes `*.sql` and golang-migrate silently skips names it cannot parse, so a typo means the migration never runs. `cmd/api/migrate_test.go` checks the embedded set against the directory.

## Architecture

Request flow: **React Router SPA → API handlers/middleware → service layer → data layer → PostgreSQL.**

- **`cmd/api/`** — HTTP layer. `main.go` wires the data/service/scheduler layers and an `application` struct that all handlers hang off; `config.go` holds the `config` struct and `loadConfig()`, which reads every environment variable through `internal/env`; `migrate.go` runs the embedded migrations first, on a throwaway connection, so the pool is opened against the schema the rest of the wiring assumes. `routes.go` is the single source of truth for endpoints and for how the embedded SPA is served. `middleware.go` holds the middleware chain `secureHeaders → recoverPanic → rateLimit → authenticate → requireAuthenticatedUser → router`. `secureHeaders` is outermost so every response carries the security headers; it also puts a per-request CSP nonce on the context, which `serveIndex` stamps onto the SPA shell's inline scripts (hence the shell is served `no-store`) — see the En-têtes de sécurité section of `docs/ARCHITECTURE.md`. `rateLimit` and `requireAuthenticatedUser` wrap every route but only act on the `/v1/` prefix (`apiPrefix`), because the same binary serves the embedded SPA: throttling a cold page load's ~20 asset requests would return 429s and break it, and the SPA's guest pages must stay reachable anonymously. `requireAuthenticatedUser` is deny-by-default — a new `/v1/` route is authenticated unless its `"METHOD /path"` is added to `publicAPIRoutes`.
- **`internal/data/`** — Data layer. `models.go` aggregates per-entity models (`Users`, `Feeds`, `Subscriptions`, `Folders`, `Links`, `Articles`, `Tokens`, `OPMLImports`), each a struct wrapping `*sql.DB`. `ErrRecordNotFound` is the sentinel for "not found". This is the only layer that touches SQL.
- **`internal/opml/`** — OPML reading and writing (`encoding/xml`), with no dependency on the rest. Attributes are matched case-insensitively because `encoding/xml` matches them exactly and drops a mismatched `xmlURL` silently — a lost subscription the user cannot notice.
- **`internal/service/`** — Business logic. `services.go` aggregates services (`FeedService`, `SubscriptionService`, `OPMLService`); `subscriptions.go` owns the subscribe sequence — look up or create the feed, compensate the orphan feed if the insert fails, start the article import in the background — and declares the narrow store interfaces it needs itself, which `data.FeedModel` and `data.SubscriptionModel` satisfy as they are, so it can be tested with fakes. `opml.go` runs subscription-list imports as a background job tracked in `opml_imports`; it subscribes with `SubscribeOptions.DeferImport` and wakes the scheduler once at the end, because one article import per entry would be a stampede — see the *Import / export OPML* section of `docs/ARCHITECTURE.md`. `fetcher.go` fetches/parses RSS (gofeed), extracts article content (go-readability → markdown via `internal/readability`, sanitized with bluemonday) with fallback to the RSS description; `scheduler.go` is a background worker pool (started in `serve()`) that periodically syncs feeds with per-domain rate limiting. Article scraping goes through an anti-bot ladder — `pagefetch.go` (stdlib vs browser-impersonating TLS transports), `challenge.go` (interstitial detection), `solver.go` (FlareSolverr-compatible browser sidecar), `scrape.go` (escalation, solve budget, per-host memory); see `docs/ANTIBOT_FETCHING.md`. RSS polling always uses the stdlib client.
- **`frontend/`** — React 19 + React Router v7 in **SPA mode** (`ssr: false`), TailwindCSS v4, TanStack Query for server state, react-hook-form + zod for forms. `app/routes.ts` declares every route explicitly, with the four `/app/*` pages nested in a `dashboard.tsx` layout. `app/lib/` holds one module per API domain (`links.ts`, `feeds.ts`, `search.ts`, …) on top of `api.ts`, the matching shapes live in `app/types/`, the zod schemas in `app/lib/validations/`, and shared UI in `app/components/<Name>/`.

### Key cross-cutting conventions

- **Auth**: Bearer token *or* an httpOnly `auth_token` cookie (see `authenticate`). Tokens are validated and resolved to a user; unauthenticated requests get `data.AnonymousUser` rather than a 401, so `requireAuthenticatedUser` is what turns that into a 401 for the API. `requireAuth`/`requireGuest` guard the SPA HTML routes (redirects, not 401s).
- **Sessions**: `SESSION_TTL` is an *idle* timeout. `authenticate` reads the token's expiry alongside the user and `refreshSession` (`cmd/api/tokens.go`) slides it back out to a full TTL once 10% of the lifetime has elapsed — so an active session never expires and costs one `UPDATE` every few days. Refreshing is restricted to `/v1/` requests: `authenticate` also fronts the SPA's static assets, and a `Set-Cookie` on a cacheable response leaks tokens across users. Logout deletes only the token the request carried (`authenticate` puts its hash in the context; `Tokens.DeleteByHash`), leaving the user's other devices signed in — `Tokens.DeleteAllForUser` is still there for a "sign out everywhere". The scheduler sweeps expired tokens hourly on its own goroutine; nothing else prunes that table.
- **Registration**: closed unless `REGISTRATION_ENABLED=true`, with one exception — while the `users` table is empty the sign-up goes through anyway (`Users.HasAny`), because there is no CLI to create the first account with. The check is the first thing `registerUserHandler` does, before the body is even read, for the same reason the password is validated before it is hashed: a rejected caller must not cost a bcrypt round. The SPA reads the flag from the public `GET /v1/config` to decide whether to show the "Create account" tab; that endpoint reports the raw flag and ignores the empty-table exception, which is deliberate.
- **Embedded frontend**: `frontend.go` `go:embed`s `frontend/build/client/*`. The Go binary serves the SPA: `/app/*` requires auth, `/` and `/auth` are guest-only, everything else falls through to the static file server. The embedded assets are only as fresh as the last `pnpm build`.
- **Same-origin API**: the frontend calls the API with relative paths (`/v1/...`) and `credentials: "include"`. In production the Go binary serves both from one origin. In development `frontend/vite.config.ts` proxies `/v1` to `VITE_API_TARGET` (default `http://localhost:8000`), so port 5173 is also same-origin from the browser's point of view and no CORS configuration is needed anywhere.
- **Error envelope**: handlers return errors as `{"error": ...}` — a string for generic errors, or a `{field: message}` map for 422 validation failures. The frontend's `apiFetch` (`frontend/app/lib/api.ts`) normalizes this into an `ApiError`; `applyApiError` maps field errors back onto react-hook-form.
- **Feeds must be forced to answer 200 for a new subscriber**: `ImportArticlesForSubscribers` returns on a `304` before it creates any `links`, so subscribing to a feed the instance already knows would leave that user with an empty library. `Feeds.MarkDueForSync` clears `http_etag`/`http_last_modified` and backdates `last_fetched_at`; both `Subscribe` (existing feed) and the OPML import call it. It writes the epoch rather than NULL — `Feeds.Get`/`GetByURL` scan that column into a plain `time.Time`.
- **Deduplication**: `articles` are stored once, deduped on the SHA256 of the *normalized* URL (`internal/service/urlhash.go` — scheme/host/slash/tracking-param canonicalization; hashing the raw URL split the same page across feeds, and an item with no link hashed the empty string, merging every linkless item into one article); `links` is the per-user join carrying read/favorite/archive state, progress, and a per-user unique slug. Unsubscribing deletes only the subscription, not the user's links.
- **Outbound fetches are guarded**: feed and article URLs come from users, so every Go HTTP client that fetches one carries `internal/safedial`'s `net.Dialer.Control` hook, which rejects private/link-local addresses after DNS resolution and on every redirect hop. A new outbound client must be built from `Guard.Transport`/`Guard.Dialer` — a URL validator is not a substitute (redirects and DNS rebinding walk straight past one). The browser sidecar is out of reach of this and relies on network isolation. See the *Sorties HTTP et SSRF* section of `docs/ARCHITECTURE.md`.
- **Full-text search**: `GET /v1/search` → `internal/data/search.go`. Each article carries two tsvectors — one in its own language's configuration, one in the neutral `simple_ua` — all of them created by migration 000009; `language.go` maps a BCP-47 subtag to a configuration and falls back to `simple_ua` for anything PostgreSQL cannot stem (ja, zh, th). Snippets mark matches with `[[hl]]`/`[[/hl]]` and never with HTML, so a match inside an article cannot inject a tag into the results list. `searchRankCandidates` (1000) bounds how many rows get scored, and therefore how deep pagination can go.
- **Logging**: structured JSON via `internal/jsonlog` (`logger.PrintInfo/PrintError/PrintFatal`), not the standard logger.
- **Comments explain the *why***: the code already shows what it does, so a comment earns its place by naming a constraint, a trade-off, or what the obvious alternative would have broken — which is why the existing ones run long. Code and comments are in **English**; the deep-dives in `docs/` are in **French**.

## Deployment

A three-stage `Dockerfile` (pnpm build → `go build` with those assets copied in for the `go:embed` → alpine) produces the image published to `ghcr.io/flthibaud/signet`. `docker-compose.yml` runs it as an unprivileged user on a read-only root filesystem — the app writes nothing to disk, keep it that way — and its healthcheck polls `/v1/readiness` (dependencies reachable), which is deliberately separate from `/v1/healthcheck` (process is up). The FlareSolverr sidecar is opt-in behind a compose profile: `docker compose --profile solver up -d`.

## Configuration

Config is read from environment (`.env` auto-loaded) by `loadConfig()` in `cmd/api/config.go`. `internal/env`'s `Loader` collects every parse and range error instead of stopping at the first, so a misconfigured deployment sees all of its problems on one start rather than one per restart. Adding a variable means one `l.X(key, default)` line there, plus an `l.Check` if it has a valid range. See `.env.example`. Required: `DATABASE_URL`. Others: `PORT`, `ENV`, `AUTO_MIGRATE` (default true), `RATE_LIMITER_{RPS,BURST,ENABLED}`, `TRUSTED_PROXY_COUNT` (default 0; how many reverse proxies are in front — the limiter buckets on the Nth `X-Forwarded-For` entry from the right, so at 0 behind a proxy every user shares one bucket, and reading the header from the left instead would make it trivially bypassable; see the *Identification du client derrière un proxy* section of `docs/ARCHITECTURE.md`), `SCHEDULER_{INTERVAL,WORKERS,BATCH_SIZE}` (defaults 15m / 5 / 50), `SESSION_TTL` (default 720h, idle timeout), `HSTS_MAX_AGE` (default 31536000, 0 disables), `REGISTRATION_ENABLED` (default **false**, see below), `FETCH_ALLOW_PRIVATE_NETWORKS` (default false), `DATABASE_MAX_{OPEN_CONNS,IDLE_CONNS,IDLE_TIME}`, and for the anti-bot ladder `TLS_IMPERSONATE_ENABLED` (default true) and `SOLVER_{URL,TIMEOUT,MAX_PER_FEED}` (an empty `SOLVER_URL` disables browser escalation).

A devcontainer (`.devcontainer/`) provides Go, Node/pnpm, and PostgreSQL. Deep-dives live in `docs/` (`ARCHITECTURE.md`, `ANTIBOT_FETCHING.md`, `RSS_SYNC.md`, `READABILITY_TESTING.md`, `schema.sql`). `CONTRIBUTING.md` is the human onboarding path — first-clone setup, the golang-migrate CLI, PR expectations; this file keeps only what a session needs already in context, so a change to a documented command belongs in both.
