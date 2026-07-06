package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flthibaud/origami/internal/data"
	"github.com/julienschmidt/httprouter"
)

// newPatchLinkRequest builds a PATCH /v1/links/:slug request with the slug
// route param and an authenticated-ish user in the context, so the handler can
// be exercised without a router or a database (only paths that return before
// hitting the DB are testable this way).
func newPatchLinkRequest(t *testing.T, app *application, slug, body string) *http.Request {
	t.Helper()

	req := httptest.NewRequest(http.MethodPatch, "/v1/links/"+slug, strings.NewReader(body))
	params := httprouter.Params{{Key: "slug", Value: slug}}
	req = req.WithContext(context.WithValue(req.Context(), httprouter.ParamsKey, params))
	return app.contextSetUser(req, data.AnonymousUser)
}

func TestUpdateLinkHandlerValidation(t *testing.T) {
	app := &application{}

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"empty body is a bad request", ``, http.StatusBadRequest},
		{"malformed JSON", `{`, http.StatusBadRequest},
		{"unknown field", `{"nope": true}`, http.StatusBadRequest},
		{"no updatable fields is a no-op", `{}`, http.StatusOK},
		{"reading_progress above 1", `{"reading_progress": 1.5}`, http.StatusUnprocessableEntity},
		{"reading_progress below 0", `{"reading_progress": -0.1}`, http.StatusUnprocessableEntity},
		{"negative anchor index", `{"reading_progress_anchor_index": -1}`, http.StatusUnprocessableEntity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newPatchLinkRequest(t, app, "some-article", tt.body)
			rr := httptest.NewRecorder()

			app.updateLinkHandler(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rr.Code, tt.wantStatus, rr.Body.String())
			}
		})
	}
}
