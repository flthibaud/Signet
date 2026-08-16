package main

import "net/http"

// clientConfigHandler exposes the instance settings the SPA needs before anyone
// has signed in — currently only whether the sign-up form is worth showing.
//
// It is deliberately its own endpoint rather than a field on the healthcheck,
// which is a liveness probe and should keep answering the one question it
// answers. Being public, it must only ever carry values that are already
// observable from the outside: whether registration is open is something a
// single POST would reveal anyway.
//
// The bootstrap exception (see registerUserHandler) is not reflected here. On an
// empty database this reports false while the API would still accept an
// account, which is the intent — the exception is a safety net for whoever
// installs the instance, not an invitation shown to visitors.
func (app *application) clientConfigHandler(w http.ResponseWriter, r *http.Request) {
	env := envelope{
		"config": map[string]bool{
			"registration_enabled": app.config.registrationEnabled,
		},
	}

	err := app.writeJSON(w, http.StatusOK, env, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
