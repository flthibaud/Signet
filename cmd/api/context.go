package main

import (
	"context"
	"net/http"

	"github.com/flthibaud/signet/internal/data"
)

// contextKey is unexported so that no other package can produce a key equal to
// one of the constants below, which is what keeps these values out of reach of
// anything but this package.
type contextKey string

// userContextKey holds the *data.User the request authenticated as, set by
// authenticate. It is always present on a request that reached a handler —
// AnonymousUser when no token was supplied.
const userContextKey = contextKey("user")

// nonceContextKey holds the per-request CSP nonce set by secureHeaders.
const nonceContextKey = contextKey("nonce")

// tokenHashContextKey holds the hash of the authentication token the request
// arrived with. It identifies the one session that made this request, which is
// what logout needs in order to leave the user's other devices alone. The hash
// travels rather than the plaintext, so the secret stops at the middleware.
const tokenHashContextKey = contextKey("token_hash")

// contextSetUser returns a copy of r carrying user. Called by authenticate on
// every request, including anonymous ones.
func (app *application) contextSetUser(r *http.Request, user *data.User) *http.Request {
	ctx := context.WithValue(r.Context(), userContextKey, user)
	return r.WithContext(ctx)
}

// contextGetUser returns the user the request authenticated as.
//
// A missing value cannot happen on a served request — authenticate runs ahead
// of the router and always sets one — so it is a wiring bug rather than a
// runtime condition, and panicking surfaces it in development instead of
// letting a handler act on a nil user. recoverPanic turns it into a 500.
func (app *application) contextGetUser(r *http.Request) *data.User {
	user, ok := r.Context().Value(userContextKey).(*data.User)
	if !ok {
		panic("missing user value in request context")
	}
	return user
}

// contextSetTokenHash returns a copy of r carrying the hash of the token it
// authenticated with.
func (app *application) contextSetTokenHash(r *http.Request, hash []byte) *http.Request {
	ctx := context.WithValue(r.Context(), tokenHashContextKey, hash)
	return r.WithContext(ctx)
}

// contextGetTokenHash returns the hash of the request's authentication token,
// or nil for an anonymous request. Unlike the user, this is genuinely absent
// much of the time, so it reports rather than panics.
func (app *application) contextGetTokenHash(r *http.Request) []byte {
	hash, _ := r.Context().Value(tokenHashContextKey).([]byte)
	return hash
}

// contextSetNonce returns a copy of r carrying the CSP nonce generated for this
// response.
func (app *application) contextSetNonce(r *http.Request, nonce string) *http.Request {
	ctx := context.WithValue(r.Context(), nonceContextKey, nonce)
	return r.WithContext(ctx)
}

// contextGetNonce retrieves the CSP nonce advertised to the client. Serving the
// SPA shell without it would ship HTML whose inline scripts the browser refuses
// to run, so a missing value is a wiring bug worth panicking over.
func (app *application) contextGetNonce(r *http.Request) string {
	nonce, ok := r.Context().Value(nonceContextKey).(string)
	if !ok {
		panic("missing nonce value in request context")
	}
	return nonce
}
