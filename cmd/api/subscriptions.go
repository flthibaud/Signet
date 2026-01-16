package main

import "net/http"

func (app *application) listSubscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	userID := app.contextGetUser(r).ID

	// Appel direct au model
	subs, err := app.models.Subscriptions.GetAllForUser(userID)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	app.writeJSON(w, http.StatusOK, envelope{"subscriptions": subs}, nil)
}
