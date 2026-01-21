package main

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

func (app *application) routes() http.Handler {
	router := httprouter.New()

	router.NotFound = http.HandlerFunc(app.notFoundResponse)
	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)

	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthcheckHandler)

	// Auth
	router.HandlerFunc(http.MethodPost, "/v1/users", app.registerUserHandler)
	router.HandlerFunc(http.MethodPost, "/v1/tokens/authentication", app.createAuthenticationTokenHandler)

	// Subscriptions
	router.HandlerFunc(http.MethodGet, "/v1/subscriptions", app.listSubscriptionsHandler)
	router.HandlerFunc(http.MethodPost, "/v1/subscriptions", app.createSubscriptionHandler)
	// router.HandlerFunc(http.MethodDelete, "/v1/subscriptions/:id", app.deleteSubscriptionHandler)

	// Articles d'un feed
	// router.HandlerFunc(http.MethodGet, "/v1/subscriptions/:id/articles", app.listSubscriptionArticlesHandler)

	return app.recoverPanic(app.rateLimit(app.authenticate(router)))
}
