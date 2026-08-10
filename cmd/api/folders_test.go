package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flthibaud/signet/internal/data"
	"github.com/julienschmidt/httprouter"
)

// newFolderRequest builds a request carrying an :id route param and a user in
// the context, so a handler can be exercised without a router or a database.
// Only the paths that return before touching the DB are testable this way.
func newFolderRequest(t *testing.T, app *application, method, target, id, body string) *http.Request {
	t.Helper()

	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if id != "" {
		params := httprouter.Params{{Key: "id", Value: id}}
		req = req.WithContext(context.WithValue(req.Context(), httprouter.ParamsKey, params))
	}
	return app.contextSetUser(req, data.AnonymousUser)
}

func TestCreateFolderHandlerValidation(t *testing.T) {
	app := &application{}

	tests := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"empty body is a bad request", ``, http.StatusBadRequest},
		{"malformed JSON", `{`, http.StatusBadRequest},
		{"unknown field", `{"color": "#fff"}`, http.StatusBadRequest},
		{"missing name", `{}`, http.StatusUnprocessableEntity},
		{"empty name", `{"name": ""}`, http.StatusUnprocessableEntity},
		{"whitespace-only name", `{"name": "   "}`, http.StatusUnprocessableEntity},
		{"name too long", `{"name": "` + strings.Repeat("a", 256) + `"}`, http.StatusUnprocessableEntity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newFolderRequest(t, app, http.MethodPost, "/v1/folders", "", tt.body)
			rr := httptest.NewRecorder()

			app.createFolderHandler(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rr.Code, tt.wantStatus, rr.Body.String())
			}
		})
	}
}

func TestUpdateFolderHandlerValidation(t *testing.T) {
	app := &application{}

	tests := []struct {
		name       string
		id         string
		body       string
		wantStatus int
	}{
		{"non-numeric id", "abc", `{"name": "Tech"}`, http.StatusNotFound},
		{"zero id", "0", `{"name": "Tech"}`, http.StatusNotFound},
		{"malformed JSON", "1", `{`, http.StatusBadRequest},
		{"empty name", "1", `{"name": "  "}`, http.StatusUnprocessableEntity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newFolderRequest(t, app, http.MethodPatch, "/v1/folders/"+tt.id, tt.id, tt.body)
			rr := httptest.NewRecorder()

			app.updateFolderHandler(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rr.Code, tt.wantStatus, rr.Body.String())
			}
		})
	}
}

// TestUpdateSubscriptionHandlerValidation covers the reason folder_id is read
// as a raw message: an absent key must not be taken for an explicit null and
// quietly unfile the subscription.
func TestUpdateSubscriptionHandlerValidation(t *testing.T) {
	app := &application{}

	tests := []struct {
		name       string
		id         string
		body       string
		wantStatus int
	}{
		{"non-numeric id", "abc", `{"folder_id": 1}`, http.StatusNotFound},
		{"empty body is a bad request", "1", ``, http.StatusBadRequest},
		{"unknown field", "1", `{"folder": 1}`, http.StatusBadRequest},
		{"absent folder_id", "1", `{}`, http.StatusUnprocessableEntity},
		{"folder_id of the wrong type", "1", `{"folder_id": "tech"}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := newFolderRequest(t, app, http.MethodPatch, "/v1/subscriptions/"+tt.id, tt.id, tt.body)
			rr := httptest.NewRecorder()

			app.updateSubscriptionHandler(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rr.Code, tt.wantStatus, rr.Body.String())
			}
		})
	}
}
