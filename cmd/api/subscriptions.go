package main

import (
	"errors"
	"net/http"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/service"
	"github.com/flthibaud/signet/internal/validator"
)

func (app *application) createSubscriptionHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		URL string `json:"url"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// 1. Validation
	v := validator.New()
	v.Check(input.URL != "", "url", "must be provided")
	v.Check(len(input.URL) <= 2048, "url", "must not be more than 2048 characters long")
	v.Check(validator.IsURL(input.URL), "url", "must be a valid URL")

	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	userID := app.contextGetUser(r).ID

	subscription, err := app.services.SubscriptionService.Subscribe(r.Context(), userID, input.URL, service.SubscribeOptions{})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidFeed):
			v.AddError("url", "the URL does not point to a valid RSS feed")
			app.failedValidationResponse(w, r, v.Errors)
		case errors.Is(err, service.ErrFeedNotFound):
			v.AddError("url", "the feed could not be reached")
			app.failedValidationResponse(w, r, v.Errors)
		case errors.Is(err, service.ErrAlreadySubscribed):
			v.AddError("url", "you are already subscribed to this feed")
			app.failedValidationResponse(w, r, v.Errors)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	// L'import des articles tourne en tâche de fond, la réponse part sans
	// l'attendre.
	err = app.writeJSON(w, http.StatusCreated, envelope{
		"subscription": subscription,
		"message":      "Subscription created. Articles are being imported in the background.",
	}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) deleteSubscriptionHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	userID := app.contextGetUser(r).ID

	err = app.models.Subscriptions.Delete(r.Context(), userID, id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"message": "subscription successfully deleted"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) listSubscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	subscriptions, err := app.models.Subscriptions.GetAllForUser(r.Context(), user.ID)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"subscriptions": subscriptions}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
