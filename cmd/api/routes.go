package main

import (
	"io/fs"
	"net/http"

	"github.com/flthibaud/origami"
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

	// Articles d'un feed
	// router.HandlerFunc(http.MethodGet, "/v1/subscriptions/:id/articles", app.listSubscriptionArticlesHandler)

	// Liste des articles (tous les articles de tous les feeds, triés par date de publication)
	router.HandlerFunc(http.MethodGet, "/v1/links", app.listLinksHandler)

	// Recuperer le contenu d'un article
	router.HandlerFunc(http.MethodGet, "/v1/links/:slug", app.getLinkHandler)

	// Serve embedded frontend (React Router SPA build)
	distFS, err := fs.Sub(origami.FrontendDist, "frontend/build/client")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(distFS))

	// SPA routes: all serve the same index.html, client-side router handles the rest
	spaIndex := app.serveIndex(distFS, "index.html")

	// Protected routes: require authentication
	router.HandlerFunc(http.MethodGet, "/app", app.requireAuth(spaIndex))
	router.HandlerFunc(http.MethodGet, "/app/*path", app.requireAuth(spaIndex))

	// Guest-only routes: redirect to /app if already authenticated
	router.HandlerFunc(http.MethodGet, "/auth", app.requireGuest(spaIndex))
	router.HandlerFunc(http.MethodGet, "/", app.requireGuest(spaIndex))

	// Static assets (JS, CSS, images) served without auth checks
	router.NotFound = fileServer

	return app.recoverPanic(app.rateLimit(app.authenticate(router)))
}
