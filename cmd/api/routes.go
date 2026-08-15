package main

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/flthibaud/signet"
	"github.com/julienschmidt/httprouter"
)

func (app *application) routes() http.Handler {
	router := httprouter.New()

	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)

	// Liveness (process is up) and readiness (dependencies reachable) are
	// separate on purpose.
	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthcheckHandler)
	router.HandlerFunc(http.MethodGet, "/v1/readiness", app.readinessHandler)

	// Auth
	router.HandlerFunc(http.MethodPost, "/v1/users", app.registerUserHandler)
	router.HandlerFunc(http.MethodPost, "/v1/tokens/authentication", app.createAuthenticationTokenHandler)
	router.HandlerFunc(http.MethodDelete, "/v1/tokens/authentication", app.deleteAuthenticationTokenHandler)

	// Users
	router.HandlerFunc(http.MethodGet, "/v1/users/me", app.getCurrentUserHandler)

	// Subscriptions
	router.HandlerFunc(http.MethodGet, "/v1/subscriptions", app.listSubscriptionsHandler)
	router.HandlerFunc(http.MethodPost, "/v1/subscriptions", app.createSubscriptionHandler)
	router.HandlerFunc(http.MethodPatch, "/v1/subscriptions/:id", app.updateSubscriptionHandler)
	router.HandlerFunc(http.MethodDelete, "/v1/subscriptions/:id", app.deleteSubscriptionHandler)

	// Folders
	router.HandlerFunc(http.MethodGet, "/v1/folders", app.listFoldersHandler)
	router.HandlerFunc(http.MethodPost, "/v1/folders", app.createFolderHandler)
	router.HandlerFunc(http.MethodPatch, "/v1/folders/:id", app.updateFolderHandler)
	router.HandlerFunc(http.MethodDelete, "/v1/folders/:id", app.deleteFolderHandler)

	router.HandlerFunc(http.MethodPost, "/v1/opml/import", app.importOPMLHandler)
	router.HandlerFunc(http.MethodGet, "/v1/opml/imports/latest", app.latestOPMLImportHandler)
	router.HandlerFunc(http.MethodGet, "/v1/opml/export", app.exportOPMLHandler)

	// Articles d'un feed
	// router.HandlerFunc(http.MethodGet, "/v1/subscriptions/:id/articles", app.listSubscriptionArticlesHandler)

	// Liste des articles (tous les articles de tous les feeds, triés par date de publication)
	router.HandlerFunc(http.MethodGet, "/v1/links", app.listLinksHandler)

	// Recherche full-text dans la bibliotheque de l'utilisateur
	router.HandlerFunc(http.MethodGet, "/v1/search", app.searchHandler)

	// Recuperer le contenu d'un article
	router.HandlerFunc(http.MethodGet, "/v1/links/:slug", app.getLinkHandler)
	router.HandlerFunc(http.MethodPatch, "/v1/links/:slug", app.updateLinkHandler)

	// Serve embedded frontend (React Router SPA build)
	distFS, err := fs.Sub(signet.FrontendDist, "frontend/build/client")
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

	// Anything the router did not match is a static asset (JS, CSS, images),
	// served without auth checks — except under the API prefix, where an unknown
	// path is a client error and has to come back as the JSON error envelope
	// rather than as the file server's plain-text 404.
	router.NotFound = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, apiPrefix) {
			app.notFoundResponse(w, r)
			return
		}
		fileServer.ServeHTTP(w, r)
	})

	return app.secureHeaders(app.recoverPanic(app.rateLimit(app.authenticate(app.requireAuthenticatedUser(router)))))
}
