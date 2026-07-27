package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/validator"
	"golang.org/x/time/rate"
)

// contentSecurityPolicy is sent with every response; %s is the per-request
// script nonce.
//
// script-src carries a nonce rather than 'unsafe-inline' because the SPA shell
// Vite emits bootstraps itself from inline scripts — serveIndex stamps the same
// nonce on them. style-src keeps 'unsafe-inline' on purpose: the UI styles
// elements from JavaScript, and locking that down buys little. img-src and
// media-src stay wide because article content comes from arbitrary sites and is
// rendered with its original media.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'nonce-%s'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: https: http:; " +
	"media-src 'self' https: http:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"frame-src 'none'; " +
	"object-src 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'"

// secureHeaders sets the response headers that are the binary's job rather than
// the reverse proxy's: it serves the SPA itself, so nothing upstream can be
// assumed to add them.
func (app *application) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce, err := newNonce()
		if err != nil {
			app.serverErrorResponse(w, r, err)
			return
		}

		h := w.Header()
		h.Set("Content-Security-Policy", fmt.Sprintf(contentSecurityPolicy, nonce))
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		if app.config.hstsMaxAge > 0 && requestIsHTTPS(r) {
			h.Set("Strict-Transport-Security", fmt.Sprintf("max-age=%d", app.config.hstsMaxAge))
		}

		next.ServeHTTP(w, app.contextSetNonce(r, nonce))
	})
}

// newNonce returns a fresh base64 CSP nonce.
func newNonce() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawStdEncoding.EncodeToString(b), nil
}

func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	// Chained proxies append to X-Forwarded-Proto; the client's scheme is first.
	proto, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Proto"), ",")
	return strings.EqualFold(strings.TrimSpace(proto), "https")
}

func (app *application) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create a deferred function (which will always be run in the event of a panic
		// as Go unwinds the stack).
		defer func() {
			// Use the builtin recover function to check if there has been a panic or
			// not.
			if err := recover(); err != nil {
				// If there was a panic, set a "Connection: close" header on the
				// response. This acts as a trigger to make Go's HTTP server
				// automatically close the current connection after a response has been
				// sent.
				w.Header().Set("Connection", "close")
				// The value returned by recover() has the type any, so we use
				// fmt.Errorf() to normalize it into an error and call our
				// serverErrorResponse() helper. In turn, this will log the error using
				// our custom Logger type at the ERROR level and send the client a 500
				// Internal Server Error response.
				app.serverErrorResponse(w, r, fmt.Errorf("%s", err))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

const apiPrefix = "/v1/"

// clientIP resolves the address rate limiting buckets on.
//
// Straight off the connection, that address is whoever opened the socket — which
// behind a reverse proxy is the proxy, putting every user in one bucket. Proxies
// record what they saw in X-Forwarded-For, appending on the right as the request
// travels inward, so the rightmost entries are the ones our own infrastructure
// wrote and the leftmost may have been forged by the client. Reading the header
// naively is worse than not reading it: anyone could vary the leftmost value and
// get a fresh bucket per request.
//
// config.trustedProxyCount says how many hops on the right are ours, so the
// client is the Nth entry counting from that side. A forged value only ever gets
// pushed further left as each proxy appends, and we never look there.
//
// A count rather than a list of trusted addresses because under Docker or a PaaS
// the proxy's address is a container IP that changes on redeploy. Anything that
// doesn't add up — fewer entries than trusted hops, an entry that isn't an
// address — falls back to the peer address, which is the pre-existing behaviour
// and never less safe.
func (app *application) clientIP(r *http.Request) string {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		// No port to strip (unusual, but RemoteAddr is not guaranteed to carry
		// one); the raw value still keys a bucket consistently.
		peer = r.RemoteAddr
	}

	n := app.config.trustedProxyCount
	if n <= 0 {
		return peer
	}

	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded == "" {
		return peer
	}

	entries := strings.Split(forwarded, ",")
	i := len(entries) - n
	if i < 0 {
		// Fewer hops than configured: the leftmost entries are missing, so the
		// one we would read is not the one we trust.
		return peer
	}

	candidate := strings.TrimSpace(entries[i])
	if net.ParseIP(candidate) == nil {
		return peer
	}

	return candidate
}

func (app *application) rateLimit(next http.Handler) http.Handler {
	// Define a client struct to hold the rate limiter and last seen time for each
	// client.
	type client struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}

	var (
		mu sync.Mutex
		// Update the map so the values are pointers to a client struct.
		clients = make(map[string]*client)
	)

	// Launch a background goroutine which removes old entries from the clients map once
	// every minute. Only worth running when the limiter is on — the map stays
	// empty otherwise, and the configuration is fixed by the time the middleware
	// chain is built.
	if app.config.limiter.enabled {
		go func() {
			for {
				time.Sleep(time.Minute)
				// Lock the mutex to prevent any rate limiter checks from happening while
				// the cleanup is taking place.
				mu.Lock()
				// Loop through all clients. If they haven't been seen within the last three
				// minutes, delete the corresponding entry from the map.
				for ip, client := range clients {
					if time.Since(client.lastSeen) > 3*time.Minute {
						delete(clients, ip)
					}
				}
				mu.Unlock()
			}
		}()
	}

	// Importantly, unlock the mutex when the cleanup is complete.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if app.config.limiter.enabled && strings.HasPrefix(r.URL.Path, apiPrefix) {
			ip := app.clientIP(r)
			mu.Lock()
			if _, found := clients[ip]; !found {
				// Create and add a new client struct to the map if it doesn't already exist.
				clients[ip] = &client{
					limiter: rate.NewLimiter(rate.Limit(app.config.limiter.rps), app.config.limiter.burst),
				}
			}
			// Update the last seen time for the client.
			clients[ip].lastSeen = time.Now()

			if !clients[ip].limiter.Allow() {
				mu.Unlock()
				app.rateLimitExceededResponse(w, r)
				return
			}

			mu.Unlock()
		}

		next.ServeHTTP(w, r)
	})
}

func (app *application) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to get the token from the Authorization header first, then fall back
		// to the auth_token cookie. A malformed or expired cookie is treated as an
		// anonymous request (not a 401): browsers keep sending stale cookies, and
		// guest pages must stay reachable so the user can log in again.
		var token string
		var fromCookie bool

		authorizationHeader := r.Header.Get("Authorization")

		switch {
		case authorizationHeader != "":
			w.Header().Add("Vary", "Authorization")
			headerParts := strings.Split(authorizationHeader, " ")
			if len(headerParts) != 2 || headerParts[0] != "Bearer" {
				app.invalidAuthenticationTokenResponse(w, r)
				return
			}
			token = headerParts[1]

		default:
			cookie, err := r.Cookie("auth_token")
			if err != nil {
				r = app.contextSetUser(r, data.AnonymousUser)
				next.ServeHTTP(w, r)
				return
			}
			token = cookie.Value
			fromCookie = true
		}

		// Validate the token to make sure it is in a sensible format.
		v := validator.New()

		// If the token isn't valid, use the invalidAuthenticationTokenResponse()
		// helper to send a response, rather than the failedValidationResponse() helper
		// that we'd normally use.
		if data.ValidateTokenPlaintext(v, token); !v.Valid() {
			if fromCookie {
				r = app.contextSetUser(r, data.AnonymousUser)
				next.ServeHTTP(w, r)
				return
			}
			app.invalidAuthenticationTokenResponse(w, r)
			return
		}

		// Retrieve the details of the user associated with the authentication token,
		// again calling the invalidAuthenticationTokenResponse() helper if no
		// matching record was found. IMPORTANT: Notice that we are using
		// ScopeAuthentication as the first parameter here.
		user, expiry, err := app.models.Users.GetForToken(data.ScopeAuthentication, token)
		if err != nil {
			switch {
			case errors.Is(err, data.ErrRecordNotFound):
				if fromCookie {
					r = app.contextSetUser(r, data.AnonymousUser)
					next.ServeHTTP(w, r)
					return
				}
				app.invalidAuthenticationTokenResponse(w, r)
			default:
				app.serverErrorResponse(w, r, err)
			}
			return
		}

		// Call the contextSetUser() helper to add the user information to the request
		// context, and carry the token's identity alongside it so logout can end
		// this session alone.
		tokenHash := data.HashTokenPlaintext(token)
		r = app.contextSetUser(r, user)
		r = app.contextSetTokenHash(r, tokenHash)

		// Slide the expiry forward on an active session. Done before the handler
		// runs, since a refreshed cookie has to reach the response header before
		// anything writes a body.
		app.refreshSession(w, r, tokenHash, expiry, fromCookie)

		// Call the next handler in the chain.
		next.ServeHTTP(w, r)
	})
}

// publicAPIRoutes lists the "method path" pairs under apiPrefix that are
// reachable without a token. Everything else under the prefix is rejected by
// requireAuthenticatedUser, so a route added to routes.go is authenticated
// unless it is deliberately listed here.
var publicAPIRoutes = map[string]struct{}{
	http.MethodGet + " /v1/healthcheck":              {},
	http.MethodGet + " /v1/readiness":                {},
	http.MethodPost + " /v1/users":                   {},
	http.MethodPost + " /v1/tokens/authentication":   {},
	http.MethodDelete + " /v1/tokens/authentication": {},
}

// requireAuthenticatedUser rejects anonymous requests to the JSON API.
func (app *application) requireAuthenticatedUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, apiPrefix) {
			if _, public := publicAPIRoutes[r.Method+" "+r.URL.Path]; !public {
				if app.contextGetUser(r).IsAnonymous() {
					app.invalidAuthenticationTokenResponse(w, r)
					return
				}
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (app *application) serveIndex(fsys fs.FS, path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			app.serverErrorResponse(w, r, err)
			return
		}

		body := addScriptNonce(b, app.contextGetNonce(r))

		w.Header().Set("Cache-Control", "no-store")
		http.ServeContent(w, r, "index.html", time.Time{}, bytes.NewReader(body))
	})
}

// addScriptNonce stamps nonce on every <script> tag in the SPA shell. Matching
// on the two forms a tag can open with (rather than the bare prefix) keeps it
// off identifiers like <scripting> — the shell is our own build output, so the
// remaining false-positive would be the literal text "<script" inside a script
// body.
func addScriptNonce(html []byte, nonce string) []byte {
	// Attribute-carrying tags first: stamping the bare form would otherwise
	// produce a `<script nonce="…">` that the second pass matches again.
	html = bytes.ReplaceAll(html, []byte("<script "), []byte(`<script nonce="`+nonce+`" `))
	return bytes.ReplaceAll(html, []byte("<script>"), []byte(`<script nonce="`+nonce+`">`))
}

func (app *application) requireAuth(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := app.contextGetUser(r)
		if user.IsAnonymous() {
			http.Redirect(w, r, "/auth", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (app *application) requireGuest(next http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := app.contextGetUser(r)
		if !user.IsAnonymous() {
			http.Redirect(w, r, "/app", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	}
}
