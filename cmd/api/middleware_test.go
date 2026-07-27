package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flthibaud/signet/internal/data"
	"github.com/google/uuid"
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

// serveAs runs one request through requireAuthenticatedUser with the given user
// already in the context, mimicking what authenticate does upstream.
func serveAs(user *data.User, method, path string) (served bool, status int) {
	app := &application{}

	handler := app.requireAuthenticatedUser(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		served = true
		w.WriteHeader(http.StatusOK)
	}))

	req := app.contextSetUser(httptest.NewRequest(method, path, nil), user)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	return served, rr.Code
}

func TestRequireAuthenticatedUserRejectsAnonymousAPICalls(t *testing.T) {
	// Anonymous requests used to reach these handlers and get a 200 with an
	// empty body, because the user_id filter matched uuid.Nil and no rows.
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/v1/links"},
		{http.MethodGet, "/v1/links/some-slug"},
		{http.MethodPatch, "/v1/links/some-slug"},
		{http.MethodGet, "/v1/search"},
		{http.MethodGet, "/v1/subscriptions"},
		{http.MethodPost, "/v1/subscriptions"},
		{http.MethodDelete, "/v1/subscriptions/123"},
		{http.MethodGet, "/v1/users/me"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			served, status := serveAs(data.AnonymousUser, route.method, route.path)

			if served {
				t.Error("handler was reached, want the request rejected before it")
			}
			if status != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", status, http.StatusUnauthorized)
			}
		})
	}
}

func TestRequireAuthenticatedUserAllowsPublicAPIRoutes(t *testing.T) {
	for _, route := range []struct{ method, path string }{
		{http.MethodGet, "/v1/healthcheck"},
		{http.MethodGet, "/v1/readiness"},
		{http.MethodPost, "/v1/users"},
		{http.MethodPost, "/v1/tokens/authentication"},
		// Logout must work with a stale cookie, otherwise the browser keeps it.
		{http.MethodDelete, "/v1/tokens/authentication"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			served, status := serveAs(data.AnonymousUser, route.method, route.path)

			if !served {
				t.Errorf("handler not reached, status = %d", status)
			}
		})
	}
}

func TestRequireAuthenticatedUserSkipsSPARoutes(t *testing.T) {
	// The SPA guards itself with requireAuth/requireGuest; /auth and the static
	// assets have to stay reachable anonymously or nobody can log in.
	for _, path := range []string{"/", "/auth", "/app/reader", "/assets/index-abc123.js"} {
		t.Run(path, func(t *testing.T) {
			served, status := serveAs(data.AnonymousUser, http.MethodGet, path)

			if !served {
				t.Errorf("handler not reached, status = %d", status)
			}
		})
	}
}

func TestRequireAuthenticatedUserLetsAuthenticatedCallsThrough(t *testing.T) {
	user := &data.User{ID: uuid.New()}

	served, status := serveAs(user, http.MethodGet, "/v1/links")

	if !served {
		t.Errorf("handler not reached, status = %d", status)
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
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
