# Contributing to Signet

Thanks for taking an interest. This document should get you from a clone to a running instance in under ten minutes.

## Getting set up

The fastest path is the [devcontainer](.devcontainer/devcontainer.json), which brings Go, Node/pnpm and PostgreSQL preconfigured:

> *VSCode → `Dev Containers: Reopen in Container`*

Without it you need **Go 1.25+**, **Node.js 22+ with pnpm**, and **PostgreSQL 14+**.

```bash
git clone https://github.com/flthibaud/Signet.git && cd Signet
cp .env.example .env          # only DATABASE_URL is actually required
cd frontend && pnpm install && pnpm build && cd ..
go run ./cmd/api
```

`pnpm build` is not optional, even if you never touch the frontend: [frontend.go](frontend.go) `go:embed`s `frontend/build/client`, so **the Go packages do not compile until that directory exists**. This is also why CI builds the frontend first and hands its output to every Go job.

The server applies its own migrations at startup, so an empty database is enough. It listens on <http://localhost:8000>.

## Day-to-day

```bash
go run ./cmd/api                          # API + embedded frontend, port 8000
cd frontend && pnpm dev                   # Vite dev server, port 5173, HMR
```

Work on <http://localhost:5173> when you are changing the frontend: Vite proxies `/v1` to the Go API (`VITE_API_TARGET`, default `http://localhost:8000`), so the browser sees a single origin and the auth cookies work with no CORS setup. Port 8000 serves the SPA **from the embedded build**, which means changes only show up there after another `pnpm build`.

```bash
go test ./...                             # Go tests
go vet ./...                              # must be clean
cd frontend && pnpm typecheck             # react-router typegen + tsc
```

CI additionally runs `staticcheck`, `govulncheck`, `go test -race`, and checks that `go mod tidy` leaves no diff. Running `go vet` and the tests locally catches almost everything.

### Database-backed tests

`internal/data` has tests that run real queries, because a query naming a column that no longer exists — or a `SELECT` list that has drifted from its `Scan` — is invisible to both the compiler and to tests that only assert on SQL strings. They read `TEST_DATABASE_URL`, fall back to `DATABASE_URL`, and **skip** when neither is set, so `go test ./...` still passes with no database.

Point them at a throwaway database:

```bash
TEST_DATABASE_URL="postgres://signet:signet@localhost:5432/signet_test?sslmode=disable" go test ./internal/data
```

The fixtures only insert rows they own and delete them on cleanup — nothing truncates a table — so they are safe against a development database, if not against one you care about.

**Any new query in that package wants a test there.**

### Migrations

```bash
make migrate-create name=add_x   # new up/down pair
make migrate-down                # roll back the last one
make migrate-version
make migrate-force version=N     # recover from a "Dirty database" error
make reset-db                    # drop everything and re-run
```

These need the [golang-migrate](https://github.com/golang-migrate/migrate) CLI; the server itself does not. A migration must be a complete `NNNNNN_name.{up,down}.sql` **pair**: the embed glob takes `*.sql` and golang-migrate silently skips names it cannot parse, so a typo means the migration never runs. `cmd/api/migrate_test.go` checks the embedded set against the directory and will catch it.

## Conventions

- **The data layer is the only place that writes SQL.** Handlers call services, services call models.
- **Every outbound HTTP client must dial through [`internal/safedial`](internal/safedial/safedial.go).** Feed and article URLs come from users, and a URL validator is not a substitute — redirects and DNS rebinding walk straight past one.
- **A new `/v1/` route is authenticated by default.** `requireAuthenticatedUser` is deny-by-default; a public route has to be added to `publicAPIRoutes` in [middleware.go](cmd/api/middleware.go).
- **Log through `internal/jsonlog`**, not the standard logger.
- **Comment the *why*.** The code already shows what it does. A comment earns its place by explaining a constraint, a trade-off, or what the obvious alternative would have broken.
- Code and code comments are in **English**. The deep-dives in `docs/` are in French.

## Pull requests

Branch off `develop`, keep the change focused, and make sure `go vet ./...`, `go test ./...` and `pnpm typecheck` pass. If you changed behaviour a user or an operator can see, update the README — and `docs/ARCHITECTURE.md` if you changed how a piece fits together.

Opening an issue first is welcome for anything large; it is cheaper to disagree about an approach than about a finished branch.

## License

Contributions are made under the project's [AGPL-3.0-or-later](LICENSE) license.
