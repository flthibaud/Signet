// Package signet is the module root. It holds no logic — only the two
// go:embed declarations that make the binary self-contained: the built frontend
// and the SQL migrations.
//
// They live here rather than beside their consumers because go:embed cannot
// reach outside its own directory, and both trees sit at the repository root.
// The application itself is cmd/api.
package signet

import "embed"

// FrontendDist holds the built React Router SPA, so the one binary serves both
// the JSON API and the interface it is consumed by — no static file host, no
// second deployment artifact to keep in step with this one.
//
// The embed is of frontend/build/client, which means `pnpm build` must have run
// before `go build`: a stale build directory embeds silently, and an absent one
// fails the compile. routes serves it from the fs.Sub of that prefix.
//
//go:embed frontend/build/client/*
var FrontendDist embed.FS
