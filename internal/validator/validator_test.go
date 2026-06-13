package validator

import "testing"

func TestValidAndCheck(t *testing.T) {
	v := New()
	if !v.Valid() {
		t.Fatal("a new validator should be valid")
	}

	v.Check(true, "field", "should not be added")
	if !v.Valid() {
		t.Fatal("a passing check should not add an error")
	}

	v.Check(false, "field", "must be provided")
	if v.Valid() {
		t.Fatal("a failing check should make the validator invalid")
	}
	if got := v.Errors["field"]; got != "must be provided" {
		t.Fatalf("unexpected error message: got %q", got)
	}
}

func TestAddErrorDoesNotOverwrite(t *testing.T) {
	v := New()
	v.AddError("field", "first")
	v.AddError("field", "second")

	if got := v.Errors["field"]; got != "first" {
		t.Fatalf("AddError should keep the first message, got %q", got)
	}
}

func TestPermittedValue(t *testing.T) {
	if !PermittedValue("b", "a", "b", "c") {
		t.Error("expected b to be permitted")
	}
	if PermittedValue("z", "a", "b", "c") {
		t.Error("expected z to be rejected")
	}
	if !PermittedValue(2, 1, 2, 3) {
		t.Error("expected 2 to be permitted")
	}
}

func TestUnique(t *testing.T) {
	if !Unique([]string{"a", "b", "c"}) {
		t.Error("expected distinct slice to be unique")
	}
	if Unique([]string{"a", "b", "a"}) {
		t.Error("expected slice with a duplicate to be non-unique")
	}
	if !Unique([]int{}) {
		t.Error("expected empty slice to be unique")
	}
}

func TestMatchesEmailRX(t *testing.T) {
	tests := []struct {
		name  string
		email string
		want  bool
	}{
		{"simple", "user@example.com", true},
		{"subdomain", "user@mail.example.co.uk", true},
		{"plus tag", "user+tag@example.com", true},
		{"missing @", "userexample.com", false},
		{"missing domain", "user@", false},
		{"missing local part", "@example.com", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Matches(tt.email, EmailRX); got != tt.want {
				t.Errorf("Matches(%q) = %v, want %v", tt.email, got, tt.want)
			}
		})
	}
}

func TestIsURL(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"https with path", "https://example.com/feed.xml", true},
		{"http", "http://example.com", true},
		{"empty", "", false},
		{"no scheme", "example.com/feed", false},
		{"scheme only", "http://", false},
		{"relative path", "/feed.xml", false},
		{"ftp scheme", "ftp://example.com/feed.xml", false},
		{"javascript scheme", "javascript://example.com", false},
		{"mailto scheme", "mailto:user@example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsURL(tt.url); got != tt.want {
				t.Errorf("IsURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}
