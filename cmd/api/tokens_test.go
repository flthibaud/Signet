package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSessionDueForRefresh(t *testing.T) {
	const ttl = 30 * 24 * time.Hour

	tests := []struct {
		name      string
		remaining time.Duration
		want      bool
	}{
		{"freshly issued", ttl, false},
		{"just inside the window", 28 * 24 * time.Hour, false},
		{"aged past the ratio", 26 * 24 * time.Hour, true},
		{"nearly lapsed", time.Minute, true},
		{"already lapsed", -time.Hour, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sessionDueForRefresh(time.Now().Add(tt.remaining), ttl)
			if got != tt.want {
				t.Errorf("sessionDueForRefresh(now+%s, %s) = %v, want %v", tt.remaining, ttl, got, tt.want)
			}
		})
	}
}

// TestSessionRefreshCadence pins the practical consequence of the ratio: an
// active session writes to the tokens table rarely, not on every request.
func TestSessionRefreshCadence(t *testing.T) {
	const ttl = 30 * 24 * time.Hour

	issued := time.Now()
	expiry := issued.Add(ttl)

	var elapsed time.Duration
	for !sessionDueForRefresh(expiry.Add(-elapsed), ttl) {
		elapsed += time.Hour
		if elapsed > ttl {
			t.Fatal("session never became due for refresh")
		}
	}

	if elapsed < 24*time.Hour {
		t.Errorf("session refreshes after only %s of use; expected at least a day between writes", elapsed)
	}
}

// TestDeleteAuthenticationTokenHandlerAnonymous covers the path where no token
// reached the handler: it must still clear the cookie and answer 200 without
// touching the database.
func TestDeleteAuthenticationTokenHandlerAnonymous(t *testing.T) {
	app := &application{}

	req := httptest.NewRequest(http.MethodDelete, "/v1/tokens/authentication", nil)
	rr := httptest.NewRecorder()

	app.deleteAuthenticationTokenHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusOK)
	}

	cookie := findCookie(rr.Result().Cookies(), authCookieName)
	if cookie == nil {
		t.Fatal("no auth cookie in response")
	}
	if cookie.MaxAge >= 0 || cookie.Value != "" {
		t.Errorf("cookie not expired: value = %q, MaxAge = %d", cookie.Value, cookie.MaxAge)
	}
}

func TestSetAuthCookie(t *testing.T) {
	app := &application{}
	app.config.env = "production"

	rr := httptest.NewRecorder()
	app.setAuthCookie(rr, httptest.NewRequest(http.MethodPost, "/v1/tokens/authentication", nil),
		"TOKENVALUE", time.Now().Add(2*time.Hour))

	cookie := findCookie(rr.Result().Cookies(), authCookieName)
	if cookie == nil {
		t.Fatal("no auth cookie in response")
	}
	if !cookie.HttpOnly || !cookie.Secure {
		t.Errorf("cookie not hardened: HttpOnly = %v, Secure = %v", cookie.HttpOnly, cookie.Secure)
	}
	// Allow a second of slack for the clock between call and assertion.
	if cookie.MaxAge < int(2*time.Hour/time.Second)-1 || cookie.MaxAge > int(2*time.Hour/time.Second) {
		t.Errorf("MaxAge = %d, want ~%d", cookie.MaxAge, int(2*time.Hour/time.Second))
	}
}

// A non-production instance still has to mark the cookie Secure when the
// request itself arrived over HTTPS — otherwise a deployment that forgot to set
// ENV hands out a session cookie readable off a plain-HTTP request.
func TestSetAuthCookieSecureOverHTTPS(t *testing.T) {
	app := &application{}
	app.config.env = "development"

	tests := []struct {
		name       string
		setup      func(r *http.Request)
		wantSecure bool
	}{
		{
			name:       "plain http",
			setup:      func(*http.Request) {},
			wantSecure: false,
		},
		{
			name:       "terminated TLS reported by the proxy",
			setup:      func(r *http.Request) { r.Header.Set("X-Forwarded-Proto", "https") },
			wantSecure: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/v1/tokens/authentication", nil)
			r.TLS = nil
			tt.setup(r)

			rr := httptest.NewRecorder()
			app.setAuthCookie(rr, r, "TOKENVALUE", time.Now().Add(time.Hour))

			cookie := findCookie(rr.Result().Cookies(), authCookieName)
			if cookie == nil {
				t.Fatal("no auth cookie in response")
			}
			if cookie.Secure != tt.wantSecure {
				t.Errorf("Secure = %v, want %v", cookie.Secure, tt.wantSecure)
			}
		})
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}
