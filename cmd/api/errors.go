package main

import (
	"fmt"
	"net/http"
)

// logError records an error the client is not told the details of, tagged with
// the request it came from so an entry can be traced back to one call.
func (app *application) logError(r *http.Request, err error) {
	app.logger.PrintError(err, map[string]string{
		"request_method": r.Method,
		"request_url":    r.URL.String(),
	})
}

// errorResponse writes the {"error": ...} envelope every handler in this package
// reports failures through. message is an any because the envelope carries two
// shapes: a string for generic errors, and a map[string]string of field errors
// for a 422 (see failedValidationResponse).
//
// If encoding the envelope itself fails the client gets a bare 500, since the
// response cannot be written twice.
func (app *application) errorResponse(w http.ResponseWriter, r *http.Request, status int, message any) {
	env := envelope{"error": message}

	err := app.writeJSON(w, status, env, nil)
	if err != nil {
		app.logError(r, err)
		w.WriteHeader(500)
	}
}

// serverErrorResponse reports an unexpected runtime failure: the details go to
// the log, the client gets a generic message. The two are deliberately
// different — an error string can name a table, a query or an internal host.
func (app *application) serverErrorResponse(w http.ResponseWriter, r *http.Request, err error) {
	app.logError(r, err)
	message := "the server encountered a problem and could not process your request"
	app.errorResponse(w, r, http.StatusInternalServerError, message)
}

// notFoundResponse reports that the resource does not exist. It also backs the
// router's 404 under the API prefix, so an unknown /v1/ path comes back as JSON
// rather than as the file server's plain text (see routes).
func (app *application) notFoundResponse(w http.ResponseWriter, r *http.Request) {
	message := "the requested resource could not be found"
	app.errorResponse(w, r, http.StatusNotFound, message)
}

// methodNotAllowedResponse reports that the path exists but not for this
// method. It is wired to the router's MethodNotAllowed handler.
func (app *application) methodNotAllowedResponse(w http.ResponseWriter, r *http.Request) {
	message := fmt.Sprintf("the %s method is not supported for this resource", r.Method)
	app.errorResponse(w, r, http.StatusMethodNotAllowed, message)
}

// badRequestResponse reports input the server could not even parse — malformed
// JSON, an unusable query parameter. err's text is returned to the client, so
// only pass one whose message is safe to expose.
func (app *application) badRequestResponse(w http.ResponseWriter, r *http.Request, err error) {
	app.errorResponse(w, r, http.StatusBadRequest, err.Error())
}

// failedValidationResponse reports input that parsed but did not pass its
// checks, as a 422 whose envelope maps each field to its message. errors is a
// validator.Validator's Errors map; the frontend feeds it straight back into
// the form (see applyApiError).
func (app *application) failedValidationResponse(w http.ResponseWriter, r *http.Request, errors map[string]string) {
	app.errorResponse(w, r, http.StatusUnprocessableEntity, errors)
}

// rateLimitExceededResponse reports that the client's bucket is empty. Only
// reachable under the API prefix — the SPA's assets are never throttled.
func (app *application) rateLimitExceededResponse(w http.ResponseWriter, r *http.Request) {
	message := "rate limit exceeded"
	app.errorResponse(w, r, http.StatusTooManyRequests, message)
}

// invalidCredentialsResponse reports a failed sign-in. The message says nothing
// about which half was wrong, so it cannot be used to test whether an address
// has an account here (see data.DecoyPasswordCheck for the timing half of it).
func (app *application) invalidCredentialsResponse(w http.ResponseWriter, r *http.Request) {
	message := "invalid authentication credentials"
	app.errorResponse(w, r, http.StatusUnauthorized, message)
}

// invalidAuthenticationTokenResponse reports a missing, malformed or expired
// token on a route that requires one.
func (app *application) invalidAuthenticationTokenResponse(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	message := "invalid or missing authentication token"
	app.errorResponse(w, r, http.StatusUnauthorized, message)
}
