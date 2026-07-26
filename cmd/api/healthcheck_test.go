package main

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flthibaud/signet/internal/jsonlog"
	_ "github.com/lib/pq"
)

// unreachableDB opens a pool pointed at a port nothing listens on. sql.Open is
// lazy, so this only fails when the handler actually pings.
func unreachableDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("postgres", "postgres://signet:signet@127.0.0.1:1/signet?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return db
}

func newProbeApp(db *sql.DB) *application {
	return &application{
		logger: jsonlog.New(io.Discard, jsonlog.LevelInfo),
		db:     db,
	}
}

func TestHealthcheckIgnoresDatabase(t *testing.T) {
	// Liveness must stay green while the database is down: restarting the
	// process does not fix an unreachable Postgres.
	app := newProbeApp(unreachableDB(t))

	rr := httptest.NewRecorder()
	app.healthcheckHandler(rr, httptest.NewRequest(http.MethodGet, "/v1/healthcheck", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestReadinessFailsWhenDatabaseUnreachable(t *testing.T) {
	app := newProbeApp(unreachableDB(t))

	rr := httptest.NewRecorder()
	app.readinessHandler(rr, httptest.NewRequest(http.MethodGet, "/v1/readiness", nil))

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusServiceUnavailable)
	}

	var body struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decoding body: %v", err)
	}
	if body.Error == "" {
		t.Error("body carries no error message, want the standard error envelope")
	}
}
