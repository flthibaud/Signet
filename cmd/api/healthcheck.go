package main

import (
	"context"
	"net/http"
	"time"
)

// readinessTimeout bounds the database probe. It has to be well under the
// orchestrator's own probe timeout (5s in docker-compose.yml), otherwise a
// hung Postgres turns into a hung handler instead of a 503.
const readinessTimeout = 2 * time.Second

// healthcheckHandler is the liveness probe: it answers as long as the process
// is up and serving, and deliberately touches no dependency. A failing database
// must not make an orchestrator kill and restart the process — restarting fixes
// nothing when the problem is downstream.
func (app *application) healthcheckHandler(w http.ResponseWriter, r *http.Request) {
	env := envelope{
		"status": "available",
		"system_info": map[string]string{
			"environment": app.config.env,
			"version":     version,
		},
	}

	err := app.writeJSON(w, http.StatusOK, env, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// readinessHandler is the readiness probe: it reports whether this instance can
// actually serve traffic, which means reaching the database. On failure it
// returns 503 so a load balancer takes the instance out of rotation.
func (app *application) readinessHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), readinessTimeout)
	defer cancel()

	if err := app.db.PingContext(ctx); err != nil {
		app.logError(r, err)
		app.errorResponse(w, r, http.StatusServiceUnavailable, "database unavailable")
		return
	}

	env := envelope{
		"status": "ready",
		"checks": map[string]string{
			"database": "ok",
		},
	}

	err := app.writeJSON(w, http.StatusOK, env, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
