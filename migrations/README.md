The SQL migration files for the database, managed with
[golang-migrate](https://github.com/golang-migrate/migrate).

They are embedded into the binary by `migrations.go` at the repository root, and
applied on startup unless `AUTO_MIGRATE=false` (see `cmd/api/migrate.go`). A
deployment therefore needs nothing but the binary and a `DATABASE_URL`; there is
no separate migration step to run first.

Create a pair with `make migrate-create name=add_something`. **Both files must
exist, and both must be named `NNNNNN_name.{up,down}.sql`** — the embed glob only
picks up `*.sql`, and golang-migrate silently skips any name it cannot parse, so
a typo means the migration quietly never runs. `TestEmbeddedMigrationsMatchDirectory`
in `cmd/api/migrate_test.go` guards against exactly that.

Applied state lives in the `schema_migrations` table, shared by the binary and
the `make migrate-*` targets, so the two can be mixed freely.

If a migration fails part-way it leaves the database marked dirty, and every
later start refuses to proceed. Finish or undo it by hand, then clear the flag
with `make migrate-force version=N`.
