# AGENTS.md — Signet

Guidance for AI coding agents working in this repository. Read this file first.

## What this project is

**Signet** is a self-hosted read-it-later app with native RSS/Atom support, inspired by the discontinued Omnivore. A single Go binary serves both the JSON API and the embedded React Router frontend, backed by PostgreSQL. Design goals:

1. deploy as **one binary + one database**;
2. RSS as a first-class citizen, not an add-on;
3. deduplication: an article saved by N users is stored once.

The Go module is `github.com/flthibaud/signet` (root package `signet`), hosted at github.com/flthibaud/Signet. The image is published to `ghcr.io/flthibaud/signet`.

**Project stage: nothing is deployed and nobody uses this yet; local development only.** There is no backwards compatibility to preserve: a migration may rewrite or drop a column instead of being additive, a refactor may break an earlier local install, and a schema or API change needs no deprecation path or shim. Do not add an abstraction for a future that has not arrived — prefer the simplest design that fits the code as it is today.

**Deep-dives live in [`docs/`](./docs/)** (in French) — read the relevant one before changing that area:

- [`docs/ARCHITECTURE.md`](./docs/ARCHITECTURE.md): how the pieces fit — security headers and CSP, SSRF guard, client IP behind a proxy, OPML import/export.
- [`docs/ANTIBOT_FETCHING.md`](./docs/ANTIBOT_FETCHING.md): the article-scraping escalation ladder.
- [`docs/RSS_SYNC.md`](./docs/RSS_SYNC.md): feed polling and the scheduler.
- [`docs/READABILITY_TESTING.md`](./docs/READABILITY_TESTING.md): the readability golden-file tests.
- [`docs/schema.sql`](./docs/schema.sql): the schema, for reference (the migrations are authoritative).

[`CONTRIBUTING.md`](./CONTRIBUTING.md) is the human onboarding path (first clone, golang-migrate CLI, PR expectations). This file keeps only what an agent needs already in context, so **a change to a documented command belongs in both**.

## Layout

Request flow: **React Router SPA → API handlers/middleware → service layer → data layer → PostgreSQL.**

| Path | What |
| --- | --- |
| `cmd/api/` | HTTP layer: handlers, routes, middleware, config, startup migrations |
| `internal/data/` | Data layer — the **only** package that touches SQL |
| `internal/service/` | Business logic: subscriptions, OPML jobs, feed fetching, scheduler, anti-bot scraping |
| `internal/opml/` | OPML read/write, no dependency on the rest |
| `internal/safedial/` | SSRF guard for every outbound HTTP client |
| `internal/readability/` | HTML → markdown extraction |
| `internal/env/` | Environment loader that collects every config error |
| `internal/jsonlog/` | Structured JSON logger |
| `internal/validator/` | Request validation helpers |
| `frontend/` | React 19 + React Router v7 SPA (`ssr: false`), Tailwind v4, TanStack Query, react-hook-form + zod |
| `migrations/` | golang-migrate SQL pairs, embedded by `migrations.go` |
| `frontend.go` | `go:embed`s `frontend/build/client` |

### Backend

- **`cmd/api/`** — `main.go` wires the data/service/scheduler layers and an `application` struct that every handler hangs off. `config.go` holds `config` and `loadConfig()`. `migrate.go` runs the embedded migrations first, on a throwaway connection, so the pool opens against the schema the rest of the wiring assumes. `routes.go` is the single source of truth for endpoints and for how the SPA is served.
- **Middleware chain** (`middleware.go`): `secureHeaders → recoverPanic → rateLimit → authenticate → requireAuthenticatedUser → router`. `secureHeaders` is outermost so every response carries the headers; it also puts a per-request CSP nonce on the context, which `serveIndex` stamps onto the SPA shell's inline scripts (hence the shell is served `no-store`). `rateLimit` and `requireAuthenticatedUser` wrap every route but only act on the `/v1/` prefix (`apiPrefix`): throttling a cold page load's ~20 asset requests would return 429s, and the SPA's guest pages must stay reachable anonymously.
- **`internal/data/`** — `models.go` aggregates per-entity models (`Users`, `Feeds`, `Subscriptions`, `Folders`, `Links`, `Articles`, `Tokens`, `OPMLImports`), each a struct wrapping `*sql.DB`. `ErrRecordNotFound` is the "not found" sentinel.
- **`internal/service/`** — `services.go` aggregates `FeedService`, `SubscriptionService`, `OPMLService`.
  - `subscriptions.go` owns the subscribe sequence: look up or create the feed, compensate the orphan feed if the insert fails, start the article import in the background. It declares the narrow store interfaces it needs itself (which `data.FeedModel` / `data.SubscriptionModel` satisfy as-is), so it is tested with fakes.
  - `opml.go` runs subscription-list imports as a background job tracked in `opml_imports`. It subscribes with `SubscribeOptions.DeferImport` and wakes the scheduler once at the end, because one article import per entry would be a stampede.
  - `fetcher.go` fetches/parses RSS (gofeed) and extracts article content (go-readability → markdown via `internal/readability`, sanitized with bluemonday), falling back to the RSS description.
  - `scheduler.go` is a background worker pool (started in `serve()`) that syncs feeds periodically with per-domain rate limiting, and sweeps expired tokens hourly on its own goroutine.
  - Article scraping goes through an anti-bot ladder: `pagefetch.go` (stdlib vs browser-impersonating TLS), `challenge.go` (interstitial detection), `solver.go` (FlareSolverr-compatible sidecar), `scrape.go` (escalation, solve budget, per-host memory). **RSS polling always uses the stdlib client.**
- **`internal/opml/`** — attributes are matched case-insensitively: `encoding/xml` matches them exactly and silently drops a mismatched `xmlURL`, a lost subscription the user cannot notice.

### Frontend

`app/routes.ts` declares every route explicitly, with the four `/app/*` pages nested in the `dashboard.tsx` layout. `app/lib/` holds one module per API domain (`links.ts`, `feeds.ts`, `search.ts`, …) on top of `api.ts`; shapes live in `app/types/`, zod schemas in `app/lib/validations/`, shared UI in `app/components/<Name>/`.

The Go binary serves the SPA from the **embedded build**: `/app/*` requires auth, `/` and `/auth` are guest-only, everything else falls through to the static file server. Embedded assets are only as fresh as the last `pnpm build`.

## Architecture rules (non-negotiable)

- **SQL lives in `internal/data` only.** Handlers call services, services call models.
- **Every outbound fetch of a user-supplied URL is guarded.** A new HTTP client must be built from `internal/safedial`'s `Guard.Transport` / `Guard.Dialer`, whose `Control` hook rejects private/link-local addresses after DNS resolution and on every redirect hop. A URL validator is not a substitute — redirects and DNS rebinding walk straight past one. The browser sidecar is out of reach of this and relies on network isolation.
- **`/v1/` is authenticated by default.** `requireAuthenticatedUser` is deny-by-default: a new route is protected unless its `"METHOD /path"` is added to `publicAPIRoutes`.
- **The app writes nothing to disk.** It runs as an unprivileged user on a read-only root filesystem; keep it that way.
- **Never `Set-Cookie` outside `/v1/`.** `authenticate` also fronts the static assets, and a cookie on a cacheable response leaks tokens across users — which is why session refresh is restricted to `/v1/` requests.
- **Rejected callers must not cost a bcrypt round.** Validate the password before hashing it; check registration before reading the body.
- **Errors use one envelope**: `{"error": ...}` — a string for generic errors, a `{field: message}` map for 422 validation failures. The frontend's `apiFetch` normalizes it into an `ApiError`; `applyApiError` maps field errors back onto react-hook-form.
- **Search snippets never contain HTML.** Matches are marked with `[[hl]]` / `[[/hl]]`, so an article cannot inject a tag into the results list.
- **Log through `internal/jsonlog`** (`logger.PrintInfo/PrintError/PrintFatal`), never the standard logger.
- **Config goes through `internal/env`** — see [Configuration](#configuration).

## Domain behaviour that is easy to break

- **Deduplication**: `articles` are stored once, keyed on the SHA256 of the *normalized* URL (`internal/service/urlhash.go`: scheme/host/slash/tracking-param canonicalization). Hashing the raw URL split one page across feeds, and a linkless item hashed the empty string, merging every linkless item into one article. `links` is the per-user join carrying read/favorite/archive state, progress and a per-user unique slug.
- **Unsubscribing deletes only the subscription**, never the user's links.
- **A new subscriber must force the feed to answer 200.** `ImportArticlesForSubscribers` returns on a `304` before creating any `links`, so subscribing to a feed the instance already knows would leave that user with an empty library. `Feeds.MarkDueForSync` clears `http_etag` / `http_last_modified` and backdates `last_fetched_at`; both `Subscribe` (existing feed) and the OPML import call it. It writes the epoch rather than NULL because `Feeds.GetByURL` scans that column into a plain `time.Time`.
- **Auth**: Bearer token *or* an httpOnly `auth_token` cookie. Unauthenticated requests get `data.AnonymousUser`, not a 401; `requireAuthenticatedUser` turns that into a 401 for the API, and `requireAuth` / `requireGuest` guard the SPA HTML routes with redirects.
- **Sessions**: `SESSION_TTL` is an *idle* timeout. `authenticate` reads the token's expiry alongside the user, and `refreshSession` (`cmd/api/tokens.go`) slides it back to a full TTL once 10 % of the lifetime has elapsed — an active session never expires and costs one `UPDATE` every few days. Logout deletes only the token the request carried (`Tokens.DeleteByHash`), leaving the user's other devices signed in. Nothing but the scheduler's hourly sweep prunes the tokens table.
- **Registration** is closed unless `REGISTRATION_ENABLED=true`, except while the `users` table is empty (`Users.HasAny`), because there is no CLI to create the first account. The check is the first thing `registerUserHandler` does. The public `GET /v1/config` reports the raw flag and deliberately ignores the empty-table exception; the SPA uses it to show or hide the "Create account" tab.
- **Same-origin API**: the frontend calls relative paths (`/v1/...`) with `credentials: "include"`. In production one binary serves both; in development `frontend/vite.config.ts` proxies `/v1` to `VITE_API_TARGET` (default `http://localhost:8000`). No CORS configuration exists anywhere, and none should be added.
- **Full-text search**: `GET /v1/search` → `internal/data/search.go`. Each article carries two tsvectors — one in its language's configuration, one in the neutral `simple_ua` — all created by migration 000009. `language.go` maps a BCP-47 subtag to a configuration and falls back to `simple_ua` for languages PostgreSQL cannot stem (ja, zh, th). `searchRankCandidates` (1000) bounds how many rows are scored, and therefore how deep pagination goes.

## Commands

**Every Go command needs `frontend/build/client`**, which is gitignored: `frontend.go` `go:embed`s it, so on a fresh clone `go build`, `go vet` and `go test ./...` all fail until `pnpm build` has run once.

```bash
# Backend (Go 1.26)
go run ./cmd/api                 # API + embedded frontend on :8000 (reads .env)
go build -o bin/api ./cmd/api
go test ./...                    # all tests; go test ./internal/data for one package
go vet ./...

# Frontend (in frontend/, pnpm)
pnpm dev                         # Vite dev server with HMR on :5173, proxies /v1
pnpm build                       # -> frontend/build/client, embedded into the Go binary
pnpm typecheck                   # react-router typegen && tsc

# Migrations (golang-migrate CLI, loads .env, uses DATABASE_URL)
make migrate-up | migrate-down | migrate-version
make migrate-create name=foo     # new sequential up/down pair
make migrate-goto version=N
make migrate-force version=N     # recover from "Dirty database"
make reset-db                    # drop everything and re-run
```

In the devcontainer, **do not run `pnpm add` / `pnpm install`** — the maintainer installs dependencies; edit `package.json` and say what needs installing. `pnpm typecheck` can abort with `ERR_PNPM_ABORTED_REMOVE_MODULES_DIR_NO_TTY` there; run the binaries directly from `frontend/` instead: `node_modules/.bin/react-router typegen && node_modules/.bin/tsc`.

The `make migrate-*` targets are for local schema work only; deployments do not need them. `migrations.go` embeds `migrations/*.sql` and `cmd/api/migrate.go` applies anything pending at startup (`AUTO_MIGRATE`, on by default), which is why `docker-compose.yml` has no migration service. Both paths share the standard `schema_migrations` table, so they can be mixed. `./api --migrate-only` migrates and exits, for a PaaS release command or a job ahead of a multi-replica rollout.

### Before every push

Run what CI runs (`.github/workflows/ci.yml`). CI runs the `internal/data` tests against a Postgres service; locally they **skip** without a reachable database, so point `DATABASE_URL` at one and run `go run ./cmd/api --migrate-only` first (the tests do not migrate).

```bash
(cd frontend && pnpm typecheck && pnpm build)
test -z "$(gofmt -l .)"
go mod tidy && git diff --exit-code -- go.mod go.sum
go build ./... && go vet ./...
go run honnef.co/go/tools/cmd/staticcheck@latest ./...
go run golang.org/x/vuln/cmd/govulncheck@latest ./...
go test -race ./...
```

## Tests

- **`internal/data`** runs real queries against `TEST_DATABASE_URL` (falling back to `DATABASE_URL`) and **skips** when neither is set. `testdb_test.go` holds the fixtures, `*_db_test.go` the tests; fixtures only touch rows they own. **Every new query in that package gets a DB test**: a `SELECT` list that has drifted from its `Scan`, or a struct field nothing scans into, is invisible to the compiler and to tests that only assert on SQL strings.
- **`cmd/api`** tests handlers without a database — a bare `application`, an `httptest` request, the user and route params on its context — which covers everything that returns before the SQL.
- **Migrations**: a new one must be a complete `NNNNNN_name.{up,down}.sql` pair. The embed glob takes `*.sql` and golang-migrate silently skips names it cannot parse, so a typo means the migration never runs. `cmd/api/migrate_test.go` checks the embedded set against the directory.

## Configuration

Config is read from the environment (`.env` auto-loaded) by `loadConfig()` in `cmd/api/config.go`. `internal/env`'s `Loader` collects every parse and range error instead of stopping at the first, so a misconfigured deployment sees all its problems on one start. **Adding a variable means one `l.X(key, default)` line there, plus an `l.Check` if it has a valid range, plus an entry in `.env.example`.**

| Variable | Default | Notes |
| --- | --- | --- |
| `DATABASE_URL` | — | **required** |
| `PORT`, `ENV` | | |
| `AUTO_MIGRATE` | `true` | apply pending migrations at startup |
| `RATE_LIMITER_{RPS,BURST,ENABLED}` | | |
| `TRUSTED_PROXY_COUNT` | `0` | reverse proxies in front; the limiter buckets on the Nth `X-Forwarded-For` entry **from the right**. At 0 behind a proxy every user shares one bucket; reading from the left would make it trivially bypassable |
| `SCHEDULER_{INTERVAL,WORKERS,BATCH_SIZE}` | `15m` / `5` / `50` | |
| `SESSION_TTL` | `720h` | idle timeout |
| `HSTS_MAX_AGE` | `31536000` | `0` disables |
| `REGISTRATION_ENABLED` | **`false`** | see Registration above |
| `FETCH_ALLOW_PRIVATE_NETWORKS` | `false` | lifts the SSRF guard |
| `DATABASE_MAX_{OPEN_CONNS,IDLE_CONNS,IDLE_TIME}` | | |
| `TLS_IMPERSONATE_ENABLED` | `true` | anti-bot ladder |
| `SOLVER_{URL,TIMEOUT,MAX_PER_FEED}` | | empty `SOLVER_URL` disables browser escalation |

## Deployment

A three-stage `Dockerfile` (pnpm build → `go build` with those assets copied in for the `go:embed` → alpine). `docker-compose.yml` runs it unprivileged on a read-only root filesystem; its healthcheck polls `/v1/readiness` (dependencies reachable), deliberately separate from `/v1/healthcheck` (process is up, reports the version). The FlareSolverr sidecar is opt-in: `docker compose --profile solver up -d`.

## Conventions

- Code, identifiers and comments are in **English**; the deep-dives in `docs/` are in **French**.
- **Comments explain the *why***: the code already shows what it does, so a comment earns its place by naming a constraint, a trade-off, or what the obvious alternative would have broken — which is why the existing ones run long.
- A change a user or an operator can see also updates `README.md`, and `docs/ARCHITECTURE.md` when it changes how the pieces fit together.

## Git workflow

- `develop` is the integration branch; `master` is the release branch. CI publishes the `dev` image from `develop` and `latest` from `master`, so cutting a release is a `develop` → `master` merge followed by a tag.
- Branch off an up-to-date `develop` as `feat/<slug>`, `fix/<slug>`, `chore/<slug>`, `test/<slug>` or `docs/<slug>`. PRs target **`develop`, never `master`**. Run the [pre-push checks](#before-every-push) first.
- Conventional commits in English, imperative, no scope: `fix: stop refetching the sidebar on every reading progress save`.
