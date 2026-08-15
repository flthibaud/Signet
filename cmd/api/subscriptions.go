package main

import (
	"encoding/json"
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

	// The article import runs in the background; the response does not wait on it.
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

func (app *application) updateSubscriptionHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	// Decoded raw so that an absent key and an explicit null stay distinct:
	// null unfiles the subscription, absent is a malformed request.
	var input struct {
		FolderID json.RawMessage `json:"folder_id"`
	}

	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	v := validator.New()
	v.Check(len(input.FolderID) > 0, "folder_id", "must be provided")
	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	var folderID *int64
	if err := json.Unmarshal(input.FolderID, &folderID); err != nil {
		app.badRequestResponse(w, r, errors.New("body contains incorrect JSON type for field \"folder_id\""))
		return
	}

	userID := app.contextGetUser(r).ID

	if folderID != nil {
		if _, err := app.models.Folders.Get(r.Context(), userID, *folderID); err != nil {
			switch {
			case errors.Is(err, data.ErrRecordNotFound):
				v.AddError("folder_id", "no such folder")
				app.failedValidationResponse(w, r, v.Errors)
			default:
				app.serverErrorResponse(w, r, err)
			}
			return
		}
	}

	err = app.models.Subscriptions.SetFolder(r.Context(), userID, id, folderID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"message": "subscription successfully updated"}, nil)
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
