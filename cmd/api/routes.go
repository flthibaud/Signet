package main

import (
	"io/fs"
	"net/http"

	omnivore "github.com/flthibaud/omnivore-go"
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

	// Users
	router.HandlerFunc(http.MethodGet, "/v1/users/me", app.getCurrentUserHandler)

	// Subscriptions
	router.HandlerFunc(http.MethodGet, "/v1/subscriptions", app.listSubscriptionsHandler)
	router.HandlerFunc(http.MethodPost, "/v1/subscriptions", app.createSubscriptionHandler)
	// router.HandlerFunc(http.MethodDelete, "/v1/subscriptions/:id", app.deleteSubscriptionHandler)

	// Articles d'un feed
	// router.HandlerFunc(http.MethodGet, "/v1/subscriptions/:id/articles", app.listSubscriptionArticlesHandler)

	// Liste des articles (tous les articles de tous les feeds, triés par date de publication)
	router.HandlerFunc(http.MethodGet, "/v1/links", app.listLinksHandler)

	// Serve embedded frontend (Astro build)
	distFS, err := fs.Sub(omnivore.FrontendDist, "frontend/dist")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(distFS))

	// Protected route: requires authentication
	router.HandlerFunc(http.MethodGet, "/app", app.requireAuth(app.serveIndex(distFS, "app/index.html")))

	// Guest-only routes: redirect to /app if already authenticated
	router.HandlerFunc(http.MethodGet, "/auth", app.requireGuest(app.serveIndex(distFS, "auth/index.html")))
	router.HandlerFunc(http.MethodGet, "/", app.requireGuest(app.serveIndex(distFS, "index.html")))

	// Static assets (JS, CSS, images) served without auth checks
	router.NotFound = fileServer

	return app.recoverPanic(app.rateLimit(app.authenticate(router)))
}
