package data

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	"github.com/flthibaud/signet/internal/validator"
	"github.com/google/uuid"
)

// TestValidateTokenPlaintextAcceptsGeneratedToken is the one that matters: the
// validator derives the expected length from the encoding, so it has to keep
// accepting whatever generateToken emits even if the entropy or encoding
// changes underneath it.
func TestValidateTokenPlaintextAcceptsGeneratedToken(t *testing.T) {
	token, err := generateToken(uuid.New(), time.Hour, ScopeAuthentication)
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}

	v := validator.New()
	ValidateTokenPlaintext(v, token.Plaintext)
	if !v.Valid() {
		t.Errorf("generated token %q rejected by its own validator: %v", token.Plaintext, v.Errors)
	}
}

func TestValidateTokenPlaintext(t *testing.T) {
	valid := strings.Repeat("A", tokenPlaintextLength)

	tests := []struct {
		name      string
		token     string
		wantValid bool
	}{
		{"well formed", valid, true},
		{"empty", "", false},
		{"one short", valid[:len(valid)-1], false},
		{"one long", valid + "A", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validator.New()
			ValidateTokenPlaintext(v, tt.token)
			if v.Valid() != tt.wantValid {
				t.Errorf("ValidateTokenPlaintext(%q) valid = %v, want %v (errors: %v)",
					tt.token, v.Valid(), tt.wantValid, v.Errors)
			}
		})
	}
}

// TestHashTokenPlaintextMatchesStoredHash guards the round trip logout and
// refresh depend on: the hash derived from a plaintext presented by a client
// must be the one generateToken stored.
func TestHashTokenPlaintextMatchesStoredHash(t *testing.T) {
	token, err := generateToken(uuid.New(), time.Hour, ScopeAuthentication)
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}

	if !bytes.Equal(HashTokenPlaintext(token.Plaintext), token.Hash) {
		t.Error("HashTokenPlaintext does not reproduce the stored hash")
	}

	want := sha256.Sum256([]byte(token.Plaintext))
	if !bytes.Equal(token.Hash, want[:]) {
		t.Error("stored hash is not the SHA-256 of the plaintext")
	}
}

func TestGenerateTokenIsUnique(t *testing.T) {
	userID := uuid.New()
	seen := make(map[string]struct{}, 100)

	for range 100 {
		token, err := generateToken(userID, time.Hour, ScopeAuthentication)
		if err != nil {
			t.Fatalf("generateToken: %v", err)
		}
		if _, dup := seen[token.Plaintext]; dup {
			t.Fatalf("generateToken repeated %q", token.Plaintext)
		}
		seen[token.Plaintext] = struct{}{}
	}
}
