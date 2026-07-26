package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newRateLimitedApp builds an application whose limiter is on and tight enough
// that the second request through it is rejected.
func newRateLimitedApp() *application {
	app := &application{}
	app.config.limiter.enabled = true
	app.config.limiter.rps = 1
	app.config.limiter.burst = 1
	return app
}

// countRequests fires n GETs at path through the rate limiter and reports how
// many reached the wrapped handler and what the last status was.
func countRequests(app *application, path string, n int) (served int, lastStatus int) {
	handler := app.rateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served++
		w.WriteHeader(http.StatusOK)
	}))

	for range n {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "192.0.2.10:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		lastStatus = rr.Code
	}
	return served, lastStatus
}

func TestRateLimitAppliesToAPI(t *testing.T) {
	served, lastStatus := countRequests(newRateLimitedApp(), "/v1/links", 5)

	if served != 1 {
		t.Errorf("%d requests served, want 1 (burst is 1)", served)
	}
	if lastStatus != http.StatusTooManyRequests {
		t.Errorf("last status = %d, want %d", lastStatus, http.StatusTooManyRequests)
	}
}

func TestRateLimitSkipsStaticAssets(t *testing.T) {
	// A cold SPA load pulls ~20 assets in one burst from a single IP. Limiting
	// those returns 429s, which means missing JS chunks and a broken page — the
	// limiter exists to protect the API, not the file server.
	for _, path := range []string{
		"/",
		"/app/reader",
		"/assets/index-abc123.js",
		"/assets/index-abc123.css",
		"/favicon.ico",
	} {
		t.Run(path, func(t *testing.T) {
			served, lastStatus := countRequests(newRateLimitedApp(), path, 25)

			if served != 25 {
				t.Errorf("%d of 25 requests served, want all of them", served)
			}
			if lastStatus != http.StatusOK {
				t.Errorf("last status = %d, want %d", lastStatus, http.StatusOK)
			}
		})
	}
}

func TestRateLimitDisabledLetsAPIThrough(t *testing.T) {
	app := newRateLimitedApp()
	app.config.limiter.enabled = false

	served, lastStatus := countRequests(app, "/v1/links", 25)

	if served != 25 {
		t.Errorf("%d of 25 requests served, want all of them", served)
	}
	if lastStatus != http.StatusOK {
		t.Errorf("last status = %d, want %d", lastStatus, http.StatusOK)
	}
}
