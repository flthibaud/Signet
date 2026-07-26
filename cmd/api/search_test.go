package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/flthibaud/signet/internal/data"
)

func TestReadOptionalTime(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    *time.Time
		wantErr bool
	}{
		{name: "absent", value: ""},
		{name: "malformed", value: "yesterday", wantErr: true},
		{name: "date only is not RFC3339", value: "2026-07-26", wantErr: true},
		{name: "unix timestamp", value: "1753488000", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qs := url.Values{}
			if tt.value != "" {
				qs.Set("since", tt.value)
			}

			got, err := readOptionalTime(qs, "since")
			if tt.wantErr {
				if err == nil {
					t.Fatalf("readOptionalTime(%q) = %v, want error", tt.value, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("readOptionalTime(%q) unexpected error: %v", tt.value, err)
			}
			if got != nil {
				t.Errorf("readOptionalTime(%q) = %v, want nil", tt.value, got)
			}
		})
	}

	// A valid timestamp round-trips, including its offset.
	qs := url.Values{}
	qs.Set("since", "2026-07-26T09:30:00+02:00")
	got, err := readOptionalTime(qs, "since")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil || !got.Equal(time.Date(2026, 7, 26, 7, 30, 0, 0, time.UTC)) {
		t.Errorf("readOptionalTime = %v, want 2026-07-26T07:30:00Z", got)
	}
}

// TestSearchHandlerParamValidation exercises the paths that reject a request
// before it reaches the database, so no DB is needed.
func TestSearchHandlerParamValidation(t *testing.T) {
	app := &application{}

	tests := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{"non-boolean is_read", "is_read=maybe", http.StatusBadRequest},
		{"non-boolean archived", "archived=1.5", http.StatusBadRequest},
		{"non-numeric feed_id", "feed_id=abc", http.StatusBadRequest},
		{"zero feed_id", "feed_id=0", http.StatusBadRequest},
		{"malformed since", "since=last-week", http.StatusBadRequest},
		{"over-long query", "q=" + strings.Repeat("a", maxSearchQueryLength+1), http.StatusUnprocessableEntity},
		{"one rune below the minimum", "q=" + strings.Repeat("a", minSearchQueryLength-1), http.StatusUnprocessableEntity},
		{"too-short query padded with spaces",
			"q=%20%20" + strings.Repeat("a", minSearchQueryLength-1) + "%20%20", http.StatusUnprocessableEntity},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/search?"+tt.query, nil)
			req = app.contextSetUser(req, data.AnonymousUser)
			rr := httptest.NewRecorder()

			app.searchHandler(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d (body: %s)", rr.Code, tt.wantStatus, rr.Body.String())
			}
		})
	}
}
