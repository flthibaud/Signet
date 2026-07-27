package signet

import "embed"

// Migrations holds the SQL migration files, so the binary can bring an empty or
// out-of-date database up to the schema it expects (see cmd/api/migrate.go).
//
// They are embedded rather than shipped as a directory next to the binary for
// the same reason the frontend is: whatever is not inside the binary does not
// exist wherever it runs. It costs ~11 KB.
//
//go:embed migrations/*.sql
var Migrations embed.FS
