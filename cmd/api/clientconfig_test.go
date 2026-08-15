package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The SPA hides its sign-up tab on what this endpoint reports, so the flag has
// to survive the envelope under the exact key the frontend reads.
func TestClientConfigReportsRegistrationFlag(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
	}{
		{"open", true},
		{"closed", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &application{config: config{registrationEnabled: tt.enabled}}

			rr := httptest.NewRecorder()
			app.clientConfigHandler(rr, httptest.NewRequest(http.MethodGet, "/v1/config", nil))

			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
			}

			var body struct {
				Config struct {
					RegistrationEnabled bool `json:"registration_enabled"`
				} `json:"config"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
				t.Fatalf("decoding the response: %v", err)
			}

			if body.Config.RegistrationEnabled != tt.enabled {
				t.Errorf("registration_enabled = %v, want %v", body.Config.RegistrationEnabled, tt.enabled)
			}
		})
	}
}
