package main

import (
	"crypto/tls"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/flthibaud/signet"
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

// The proxy chain is what stands between the limiter and either uselessness (one
// bucket for a whole instance) or trivial bypass (a bucket per forged header),
// so every branch of clientIP is pinned here.
func TestClientIP(t *testing.T) {
	const peer = "10.0.0.5" // the reverse proxy, as seen on the socket

	tests := []struct {
		name         string
		trustedCount int
		forwarded    string
		want         string
	}{
		{
			name:         "no proxy configured ignores the header entirely",
			trustedCount: 0,
			forwarded:    "203.0.113.7",
			want:         peer,
		},
		{
			name:         "no proxy and no header",
			trustedCount: 0,
			want:         peer,
		},
		{
			// One proxy in front: it recorded the address it accepted the
			// connection from, which is the client.
			name:         "one trusted proxy",
			trustedCount: 1,
			forwarded:    "203.0.113.7",
			want:         "203.0.113.7",
		},
		{
			// The test that matters. The client sent "1.2.3.4" itself; the proxy
			// appended what it actually saw. Reading from the left would hand out
			// a fresh bucket for every forged value.
			name:         "forged header is pushed left and ignored",
			trustedCount: 1,
			forwarded:    "1.2.3.4, 203.0.113.7",
			want:         "203.0.113.7",
		},
		{
			name:         "forged header with several fake hops",
			trustedCount: 1,
			forwarded:    "1.2.3.4, 5.6.7.8, 9.10.11.12, 203.0.113.7",
			want:         "203.0.113.7",
		},
		{
			// A CDN in front of the local proxy: the CDN recorded the client, the
			// local proxy recorded the CDN.
			name:         "two trusted proxies",
			trustedCount: 2,
			forwarded:    "203.0.113.7, 172.16.0.1",
			want:         "203.0.113.7",
		},
		{
			name:         "two trusted proxies with a forged prefix",
			trustedCount: 2,
			forwarded:    "1.2.3.4, 203.0.113.7, 172.16.0.1",
			want:         "203.0.113.7",
		},
		{
			// Fewer hops than configured — the entry we would read was not
			// written by anything we trust.
			name:         "header shorter than the trusted count",
			trustedCount: 2,
			forwarded:    "203.0.113.7",
			want:         peer,
		},
		{
			name:         "trusted proxy but no header at all",
			trustedCount: 1,
			want:         peer,
		},
		{
			name:         "entry that is not an address",
			trustedCount: 1,
			forwarded:    "not-an-ip",
			want:         peer,
		},
		{
			name:         "surrounding whitespace is trimmed",
			trustedCount: 1,
			forwarded:    "1.2.3.4,   203.0.113.7  ",
			want:         "203.0.113.7",
		},
		{
			name:         "IPv6 client",
			trustedCount: 1,
			forwarded:    "2001:db8::1",
			want:         "2001:db8::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &application{}
			app.config.trustedProxyCount = tt.trustedCount

			req := httptest.NewRequest(http.MethodGet, "/v1/links", nil)
			req.RemoteAddr = peer + ":54321"
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-For", tt.forwarded)
			}

			if got := app.clientIP(req); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

// RemoteAddr is documented as host:port but nothing enforces it; a value without
// a port must still key a bucket rather than fail the request.
func TestClientIPWithPortlessRemoteAddr(t *testing.T) {
	app := &application{}

	req := httptest.NewRequest(http.MethodGet, "/v1/links", nil)
	req.RemoteAddr = "192.0.2.10"

	if got := app.clientIP(req); got != "192.0.2.10" {
		t.Errorf("got %q, want %q", got, "192.0.2.10")
	}
}

// End to end through the middleware: the whole point of the change is that users
// behind one proxy stop sharing a budget.
func TestRateLimitBucketsPerForwardedClient(t *testing.T) {
	t.Run("separate budgets behind a trusted proxy", func(t *testing.T) {
		app := newRateLimitedApp()
		app.config.trustedProxyCount = 1

		// Same middleware instance, so the two share the clients map.
		handler := app.rateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		codes := make([]int, 0, 4)
		for _, forwarded := range []string{"203.0.113.1", "203.0.113.2", "203.0.113.1", "203.0.113.2"} {
			req := httptest.NewRequest(http.MethodGet, "/v1/links", nil)
			req.RemoteAddr = "10.0.0.5:1234"
			req.Header.Set("X-Forwarded-For", forwarded)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			codes = append(codes, rr.Code)
		}

		// Burst of 1 each: both get through once, both are rejected the second time.
		want := []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests, http.StatusTooManyRequests}
		for i, code := range codes {
			if code != want[i] {
				t.Errorf("request %d: got %d, want %d (all codes: %v)", i, code, want[i], codes)
			}
		}
	})

	t.Run("without a trusted proxy the header is ignored", func(t *testing.T) {
		app := newRateLimitedApp()
		app.config.trustedProxyCount = 0

		handler := app.rateLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		var codes []int
		for _, forwarded := range []string{"203.0.113.1", "203.0.113.2"} {
			req := httptest.NewRequest(http.MethodGet, "/v1/links", nil)
			req.RemoteAddr = "10.0.0.5:1234"
			req.Header.Set("X-Forwarded-For", forwarded)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			codes = append(codes, rr.Code)
		}

		if codes[0] != http.StatusOK || codes[1] != http.StatusTooManyRequests {
			t.Errorf("distinct headers must share one bucket at count 0, got %v", codes)
		}
	})
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
		// The SPA reads it to decide whether to offer a sign-up form, which it
		// has to do before anyone can have a token.
		{http.MethodGet, "/v1/config"},
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

// newSecureHeadersApp builds an application with HSTS configured, wrapped
// around a handler that just returns 200.
func newSecureHeadersApp() (*application, http.Handler) {
	app := &application{}
	app.config.hstsMaxAge = 31536000

	return app, app.secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func TestSecureHeadersSetOnEveryResponse(t *testing.T) {
	// The binary serves the SPA itself, so these are its responsibility on the
	// static assets as much as on the API.
	for _, path := range []string{"/", "/app/reader", "/assets/index-abc123.js", "/v1/links"} {
		t.Run(path, func(t *testing.T) {
			_, handler := newSecureHeadersApp()
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))

			for header, want := range map[string]string{
				"X-Content-Type-Options": "nosniff",
				"X-Frame-Options":        "DENY",
				"Referrer-Policy":        "strict-origin-when-cross-origin",
			} {
				if got := rr.Header().Get(header); got != want {
					t.Errorf("%s = %q, want %q", header, got, want)
				}
			}

			csp := rr.Header().Get("Content-Security-Policy")
			for _, directive := range []string{
				"default-src 'self'",
				"frame-ancestors 'none'",
				"object-src 'none'",
				"base-uri 'self'",
			} {
				if !strings.Contains(csp, directive) {
					t.Errorf("CSP %q is missing %q", csp, directive)
				}
			}
		})
	}
}

func TestSecureHeadersNonceIsPerRequest(t *testing.T) {
	_, handler := newSecureHeadersApp()

	seen := make(map[string]bool)
	for range 3 {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

		nonce := cspNonce(t, rr.Header().Get("Content-Security-Policy"))
		if seen[nonce] {
			t.Fatalf("nonce %q reused across requests", nonce)
		}
		seen[nonce] = true
	}
}

func TestHSTSOnlyOverHTTPS(t *testing.T) {
	// Sent over plain HTTP, HSTS would lock a LAN install out of its own
	// hostname for a year. Behind a TLS-terminating proxy our leg is cleartext,
	// so X-Forwarded-Proto is the only signal that the client used HTTPS.
	tests := []struct {
		name        string
		tls         bool
		forwarded   string
		wantEnabled bool
	}{
		{name: "plain http", wantEnabled: false},
		{name: "direct tls", tls: true, wantEnabled: true},
		{name: "proxied https", forwarded: "https", wantEnabled: true},
		{name: "proxied http", forwarded: "http", wantEnabled: false},
		{name: "chained proxies", forwarded: "https, http", wantEnabled: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, handler := newSecureHeadersApp()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			if tt.forwarded != "" {
				req.Header.Set("X-Forwarded-Proto", tt.forwarded)
			}

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			got := rr.Header().Get("Strict-Transport-Security")
			switch {
			case tt.wantEnabled && got != "max-age=31536000":
				t.Errorf("Strict-Transport-Security = %q, want max-age=31536000", got)
			case !tt.wantEnabled && got != "":
				t.Errorf("Strict-Transport-Security = %q, want it unset", got)
			}
		})
	}
}

func TestHSTSDisabledByZeroMaxAge(t *testing.T) {
	app, _ := newSecureHeadersApp()
	app.config.hstsMaxAge = 0
	handler := app.secureHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.TLS = &tls.ConnectionState{}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("Strict-Transport-Security = %q, want it unset", got)
	}
}

// cspNonce pulls the nonce out of a script-src directive.
func cspNonce(t *testing.T, csp string) string {
	t.Helper()

	_, rest, found := strings.Cut(csp, "'nonce-")
	if !found {
		t.Fatalf("no nonce in CSP %q", csp)
	}
	nonce, _, _ := strings.Cut(rest, "'")
	if nonce == "" {
		t.Fatalf("empty nonce in CSP %q", csp)
	}
	return nonce
}

// TestServeIndexNonceMatchesCSP is the check that keeps the SPA loading: the
// shell's inline bootstrap scripts only run if they carry the same nonce the
// header advertises.
func TestServeIndexNonceMatchesCSP(t *testing.T) {
	shell := `<!DOCTYPE html><html><head><link rel="stylesheet" href="/assets/root.css"/></head>` +
		`<body><script>console.log("hi")</script>` +
		`<script type="module" async="">import "/assets/manifest.js";</script></body></html>`
	fsys := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte(shell)}}

	app := &application{}
	handler := app.secureHeaders(app.serveIndex(fsys, "index.html"))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	nonce := cspNonce(t, rr.Header().Get("Content-Security-Policy"))
	body := rr.Body.String()

	if want := 2; strings.Count(body, `nonce="`+nonce+`"`) != want {
		t.Errorf("body stamps the nonce %d times, want %d:\n%s", strings.Count(body, `nonce="`+nonce+`"`), want, body)
	}
	if strings.Contains(body, "<script>") || strings.Contains(body, "<script type") {
		t.Errorf("a script tag was left without a nonce:\n%s", body)
	}
	// A cached shell would be replayed against a fresh header, and its stale
	// nonce would get every inline script refused.
	if got := rr.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
}

// TestServeIndexNonceOnRealShell runs the check against the shell that actually
// ships, so a frontend build that emits inline scripts in a shape addScriptNonce
// does not recognise fails here rather than in a browser.
func TestServeIndexNonceOnRealShell(t *testing.T) {
	distFS, err := fs.Sub(signet.FrontendDist, "frontend/build/client")
	if err != nil {
		t.Fatalf("frontend dist: %v", err)
	}
	shell, err := fs.ReadFile(distFS, "index.html")
	if err != nil {
		t.Skipf("no built frontend to check (run pnpm build): %v", err)
	}

	app := &application{}
	handler := app.secureHeaders(app.serveIndex(distFS, "index.html"))

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))

	nonce := cspNonce(t, rr.Header().Get("Content-Security-Policy"))
	opening := strings.Count(string(shell), "<script")
	if opening == 0 {
		t.Fatal("built index.html has no script tags, which cannot be right")
	}
	if got := strings.Count(rr.Body.String(), `nonce="`+nonce+`"`); got != opening {
		t.Errorf("%d of %d script tags carry the nonce", got, opening)
	}
}
