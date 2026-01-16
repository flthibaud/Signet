package main

import (
	"net/http"

	"github.com/flthibaud/omnivore-go/internal/validator"
)

func (app *application) createSubscriptionHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Url string `json:"url"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// 1. Validation
	v := validator.New()
	v.Check(input.Url != "", "url", "must be provided")
	v.Check(len(input.Url) <= 2048, "url", "must not be more than 2048 characters long") // URL peut être longue

	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	currentUserID := app.contextGetUser(r).ID

	// 2. Appel au Model : On s'assure que le Feed et la Subscription existent en base
	// Cette fonction renvoie l'objet Subscription complet (avec ID)
	sub, err := app.models.Feeds.SubscribeUser(currentUserID, input.Url)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	// 2. Lancer l'import des articles en arrière-plan (Async)
	// app.background(func() {
	// 	app.services.FeedImporter.ImportRecent(sub.FeedID, currentUserID)
	// })
	err = app.services.FeedImporter.ImportRecent(sub.FeedID, currentUserID)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	// 3. Réponse immédiate
	app.writeJSON(w, http.StatusCreated, envelope{"subscription": sub}, nil)
}
