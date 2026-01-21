package main

import (
	"context"
	"errors"
	"net/http"

	"github.com/flthibaud/omnivore-go/internal/data"
	"github.com/flthibaud/omnivore-go/internal/service"
	"github.com/flthibaud/omnivore-go/internal/validator"
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

	// 2. Vérifie si le feed existe déjà en base
	feed, err := app.models.Feeds.GetByURL(r.Context(), input.URL)
	if err != nil && err != data.ErrRecordNotFound {
		app.serverErrorResponse(w, r, err)
		return
	}

	// 3. Si feed n'existe pas, le créer
	if feed == nil {
		feed, err = app.services.FeedService.CreateFromURL(r.Context(), input.URL)
		if err != nil {
			// Gestion d'erreur spécifique
			switch {
			case errors.Is(err, service.ErrInvalidFeed):
				app.badRequestResponse(w, r, err)
			case errors.Is(err, service.ErrFeedNotFound):
				app.notFoundResponse(w, r)
			default:
				app.serverErrorResponse(w, r, err)
			}
			return
		}
	}

	// 4. Vérifie si subscription existe déjà
	exists, err := app.models.Subscriptions.Exists(r.Context(), userID, feed.ID)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	if exists {
		app.alreadyExistsResponse(w, r)
		return
	}

	// 5. Crée la subscription
	subscription := &data.Subscription{
		UserID: userID,
		FeedID: feed.ID,
	}

	err = app.models.Subscriptions.Insert(r.Context(), subscription)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	// 6. Import des articles EN BACKGROUND (non-bloquant)
	go func() {
		ctx := context.Background()
		err := app.services.FeedService.ImportArticles(ctx, feed.ID, userID)
		if err != nil {
			app.logger.PrintError(err, nil)
		}
	}()

	// 7. Retourne immédiatement (sans attendre l'import)
	app.writeJSON(w, http.StatusCreated, envelope{
		"subscription": subscription,
		"message":      "Subscription created. Articles are being imported in the background.",
	}, nil)
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
