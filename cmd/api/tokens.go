package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/validator"
)

const (
	ScopeActivation     = "activation"
	ScopeAuthentication = "authentication" // Include a new authentication scope.
)

// authCookieName is the httpOnly cookie the SPA authenticates with; API clients
// send the same token as a bearer instead.
const authCookieName = "auth_token"

// sessionRefreshRatio decides when an authenticated request slides its token's
// expiry back out to a full sessionTTL: once less than this share of the
// lifetime is left. Refreshing on every request would mean a write per request
// for no added security, so an active session costs one UPDATE per tenth of the
// TTL — a little over three days at the default of thirty.
//
// The window is idle-based and uncapped: staying away longer than sessionTTL
// signs you out, staying active never does.
const sessionRefreshRatio = 0.9

// sessionDueForRefresh reports whether a token whose expiry is the given time
// has aged past sessionRefreshRatio of its lifetime. A token issued with a
// longer TTL than the one now configured simply isn't due yet.
func sessionDueForRefresh(expiry time.Time, ttl time.Duration) bool {
	return time.Until(expiry) <= time.Duration(float64(ttl)*sessionRefreshRatio)
}

// setAuthCookie writes the session cookie for a token that expires at expiry.
// SameSite=Lax is enough because the API is same-origin with the SPA and every
// state-changing call is a fetch from it.
func (app *application) setAuthCookie(w http.ResponseWriter, plaintext string, expiry time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    plaintext,
		Path:     "/",
		HttpOnly: true,
		Secure:   app.config.env == "production",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(time.Until(expiry).Seconds()),
	})
}

// clearAuthCookie expires the session cookie.
func (app *application) clearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   app.config.env == "production",
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// refreshSession extends the life of a token that is being actively used, and
// re-issues the cookie carrying it so the browser keeps the two in step.
//
// A failure here is logged and swallowed: the request itself authenticated
// fine, and the session simply keeps its old expiry until the next attempt.
func (app *application) refreshSession(w http.ResponseWriter, r *http.Request, tokenHash []byte, expiry time.Time, fromCookie bool) {
	// Only the JSON API refreshes. authenticate also runs in front of the
	// embedded SPA's static assets, and hanging a Set-Cookie off a response an
	// intermediary may cache is how one user's token ends up in another's
	// browser. The SPA calls /v1/users/me on every load, so a session in use
	// still gets its refresh.
	if !strings.HasPrefix(r.URL.Path, apiPrefix) {
		return
	}

	ttl := app.config.sessionTTL

	if !sessionDueForRefresh(expiry, ttl) {
		return
	}

	newExpiry, err := app.models.Tokens.Refresh(data.ScopeAuthentication, tokenHash, ttl)
	if err != nil {
		// ErrRecordNotFound means the token lapsed or was deleted in the moment
		// between authenticating and refreshing — nothing to report.
		if !errors.Is(err, data.ErrRecordNotFound) {
			app.logError(r, err)
		}
		return
	}

	if fromCookie {
		cookie, err := r.Cookie(authCookieName)
		if err != nil {
			return
		}
		app.setAuthCookie(w, cookie.Value, newExpiry)
	}
}

func (app *application) createAuthenticationTokenHandler(w http.ResponseWriter, r *http.Request) {
	// Parse the email and password from the request body.
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Validate the email and password provided by the client.
	v := validator.New()

	data.ValidateEmail(v, input.Email)
	data.ValidatePasswordPlaintext(v, input.Password)

	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	// Lookup the user record based on the email address. If no matching user was
	// found, then we call the app.invalidCredentialsResponse() helper to send a 401
	// Unauthorized response to the client (we will create this helper in a moment).
	user, err := app.models.Users.GetByEmail(input.Email)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.invalidCredentialsResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	// Check if the provided password matches the actual password for the user.
	match, err := user.Password.Matches(input.Password)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	// If the passwords don't match, then we call the app.invalidCredentialsResponse()
	// helper again and return.
	if !match {
		app.invalidCredentialsResponse(w, r)
		return
	}

	// Otherwise, if the password is correct, we mint an authentication token. Its
	// expiry is slid forward by refreshSession for as long as the session stays
	// in use, so this is an idle timeout rather than a hard deadline.
	token, err := app.models.Tokens.New(user.ID, app.config.sessionTTL, data.ScopeAuthentication)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	app.setAuthCookie(w, token.Plaintext, token.Expiry)

	// Encode the token to JSON and send it in the response along with a 201 Created
	// status code.
	err = app.writeJSON(w, http.StatusCreated, envelope{"authentication_token": token}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// deleteAuthenticationTokenHandler logs the user out of the device that asked:
// it deletes the token this request carried and clears the cookie. The user's
// other sessions are deliberately left alone — signing out on a phone should
// not sign out the laptop.
func (app *application) deleteAuthenticationTokenHandler(w http.ResponseWriter, r *http.Request) {
	if tokenHash := app.contextGetTokenHash(r); tokenHash != nil {
		err := app.models.Tokens.DeleteByHash(data.ScopeAuthentication, tokenHash)
		if err != nil {
			app.serverErrorResponse(w, r, err)
			return
		}
	}

	// Expire the cookie regardless of authentication state, so a stale or
	// invalid cookie is cleaned up too.
	app.clearAuthCookie(w)

	err := app.writeJSON(w, http.StatusOK, envelope{"message": "successfully logged out"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
