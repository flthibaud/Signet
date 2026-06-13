package data

import (
	"strings"
	"testing"

	"github.com/flthibaud/origami/internal/validator"
)

func TestValidateEmail(t *testing.T) {
	tests := []struct {
		name      string
		email     string
		wantValid bool
	}{
		{"valid", "user@example.com", true},
		{"empty", "", false},
		{"malformed", "not-an-email", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validator.New()
			ValidateEmail(v, tt.email)
			if v.Valid() != tt.wantValid {
				t.Errorf("ValidateEmail(%q) valid = %v, want %v (errors: %v)",
					tt.email, v.Valid(), tt.wantValid, v.Errors)
			}
		})
	}
}

func TestValidatePasswordPlaintext(t *testing.T) {
	tests := []struct {
		name      string
		password  string
		wantValid bool
	}{
		{"empty", "", false},
		{"too short (7)", strings.Repeat("a", 7), false},
		{"minimum (8)", strings.Repeat("a", 8), true},
		{"maximum (72)", strings.Repeat("a", 72), true},
		{"too long (73)", strings.Repeat("a", 73), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validator.New()
			ValidatePasswordPlaintext(v, tt.password)
			if v.Valid() != tt.wantValid {
				t.Errorf("ValidatePasswordPlaintext(len=%d) valid = %v, want %v (errors: %v)",
					len(tt.password), v.Valid(), tt.wantValid, v.Errors)
			}
		})
	}
}

func TestValidateUser(t *testing.T) {
	tests := []struct {
		name      string
		username  string
		email     string
		password  string
		wantValid bool
		wantField string // expected error key when invalid
	}{
		{"valid", "florian", "florian@example.com", "averylongpassword", true, ""},
		{"missing username", "", "florian@example.com", "averylongpassword", false, "username"},
		{"username too long", strings.Repeat("a", 501), "florian@example.com", "averylongpassword", false, "username"},
		{"bad email", "florian", "nope", "averylongpassword", false, "email"},
		{"short password", "florian", "florian@example.com", "short", false, "password"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{Username: tt.username, Email: tt.email}
			if err := user.Password.Set(tt.password); err != nil {
				t.Fatalf("Password.Set: %v", err)
			}

			v := validator.New()
			ValidateUser(v, user)

			if v.Valid() != tt.wantValid {
				t.Fatalf("ValidateUser valid = %v, want %v (errors: %v)",
					v.Valid(), tt.wantValid, v.Errors)
			}
			if tt.wantField != "" {
				if _, ok := v.Errors[tt.wantField]; !ok {
					t.Errorf("expected an error on field %q, got errors: %v", tt.wantField, v.Errors)
				}
			}
		})
	}
}
