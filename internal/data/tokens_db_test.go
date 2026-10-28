package data

import (
	"errors"
	"testing"
	"time"
)

// GetForToken is the query every authenticated request goes through: it must
// resolve a live token to its owner, and treat a token that is expired, minted
// for another scope, or unknown as no session at all.
func TestGetForToken(t *testing.T) {
	db := testDB(t)
	users := UserModel{DB: db}
	tokens := TokenModel{DB: db}

	userID := seedUser(t, db)

	live, err := tokens.New(userID, time.Hour, ScopeAuthentication)
	if err != nil {
		t.Fatal(err)
	}

	user, expiry, err := users.GetForToken(ScopeAuthentication, live.Plaintext)
	if err != nil {
		t.Fatalf("live token: %v", err)
	}
	if user.ID != userID {
		t.Errorf("user = %v, want %v", user.ID, userID)
	}
	// tokens.expiry is TIMESTAMP(0): the stored value is rounded to the second.
	if d := expiry.Sub(live.Expiry); d < -time.Second || d > time.Second {
		t.Errorf("expiry = %v, want about %v", expiry, live.Expiry)
	}

	expired, err := generateToken(userID, -time.Minute, ScopeAuthentication)
	if err != nil {
		t.Fatal(err)
	}
	if err := tokens.Insert(expired); err != nil {
		t.Fatal(err)
	}

	unknown, err := generateToken(userID, time.Hour, ScopeAuthentication)
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name, scope, plaintext string
	}{
		{"wrong scope", "other-scope", live.Plaintext},
		{"expired", ScopeAuthentication, expired.Plaintext},
		{"never stored", ScopeAuthentication, unknown.Plaintext},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := users.GetForToken(tc.scope, tc.plaintext); !errors.Is(err, ErrRecordNotFound) {
				t.Errorf("err = %v, want ErrRecordNotFound", err)
			}
		})
	}
}

// Refresh slides a live session out to a full TTL, and must not bring back one
// that has already lapsed.
func TestTokenRefresh(t *testing.T) {
	db := testDB(t)
	tokens := TokenModel{DB: db}

	userID := seedUser(t, db)

	ageing, err := tokens.New(userID, time.Minute, ScopeAuthentication)
	if err != nil {
		t.Fatal(err)
	}

	newExpiry, err := tokens.Refresh(ScopeAuthentication, ageing.Hash, 24*time.Hour)
	if err != nil {
		t.Fatalf("refresh live token: %v", err)
	}
	if want := time.Now().Add(24 * time.Hour); newExpiry.Before(want.Add(-time.Minute)) {
		t.Errorf("new expiry = %v, want about %v", newExpiry, want)
	}

	lapsed, err := generateToken(userID, -time.Minute, ScopeAuthentication)
	if err != nil {
		t.Fatal(err)
	}
	if err := tokens.Insert(lapsed); err != nil {
		t.Fatal(err)
	}
	if _, err := tokens.Refresh(ScopeAuthentication, lapsed.Hash, 24*time.Hour); !errors.Is(err, ErrRecordNotFound) {
		t.Errorf("refresh lapsed token: err = %v, want ErrRecordNotFound", err)
	}
}
