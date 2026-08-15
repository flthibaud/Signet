// Package validator collects the problems with one request's input instead of
// stopping at the first, so a 422 can name every bad field at once and the
// frontend can attach each message to its own form control.
//
// A Validator is filled by the handler that parsed the request, then handed to
// failedValidationResponse, which writes its Errors map as the {field: message}
// error envelope.
package validator

import (
	"net/url"
	"regexp"
)

// EmailRX matches the HTML specification's definition of a valid email address
// (https://html.spec.whatwg.org/#valid-e-mail-address).
//
// It is a syntax check, not a deliverability one: no pattern can tell whether
// an address receives mail, so this only rejects input that cannot be an
// address at all.
var (
	EmailRX = regexp.MustCompile("^[a-zA-Z0-9.!#$%&'*+\\/=?^_`{|}~-]+@[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?(?:\\.[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?)*$")
)

// Validator accumulates validation failures, one message per field. Create it
// with New; the zero value's map is nil and AddError would panic on it.
type Validator struct {
	Errors map[string]string
}

// New returns a Validator with no errors recorded.
func New() *Validator {
	return &Validator{Errors: make(map[string]string)}
}

// Valid reports whether nothing failed, and so whether the handler may proceed.
func (v *Validator) Valid() bool {
	return len(v.Errors) == 0
}

// AddError records message against key, keeping the first message added for a
// given key. Later checks on the same field therefore cannot overwrite the
// earliest, most specific reason it was rejected.
func (v *Validator) AddError(key, message string) {
	if _, exists := v.Errors[key]; !exists {
		v.Errors[key] = message
	}
}

// Check records message against key when ok is false. It is the usual entry
// point: a handler runs every Check it has, then tests Valid once.
func (v *Validator) Check(ok bool, key, message string) {
	if !ok {
		v.AddError(key, message)
	}
}

// PermittedValue reports whether value is one of permittedValues — for fields
// that accept a fixed set, such as a sort key or a status.
func PermittedValue[T comparable](value T, permittedValues ...T) bool {
	for i := range permittedValues {
		if value == permittedValues[i] {
			return true
		}
	}
	return false
}

// Matches returns true if a string value matches a specific regexp pattern.
func Matches(value string, rx *regexp.Regexp) bool {
	return rx.MatchString(value)
}

// Unique reports whether values contains no duplicates.
func Unique[T comparable](values []T) bool {
	uniqueValues := make(map[T]bool)
	for _, value := range values {
		uniqueValues[value] = true
	}
	return len(values) == len(uniqueValues)
}

// IsURL reports whether str is a syntactically valid absolute http(s) URL.
// Only the http and https schemes are accepted so that values like
// "ftp://..." or "javascript://..." are rejected.
func IsURL(str string) bool {
	u, err := url.Parse(str)
	if err != nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
