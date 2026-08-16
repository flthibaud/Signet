package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// With registration open the gate must let the request straight through to the
// body parsing, without consulting the database — app.models is the zero value
// here, so any read of it would panic rather than quietly pass.
//
// Its other two outcomes (403 once an account exists, 201 for the first one)
// both need a database whose users table the test would have to empty, which
// the fixtures deliberately never do; they are verified by hand instead.
func TestRegisterUserHandlerSkipsTheGateWhenRegistrationIsOpen(t *testing.T) {
	app := &application{config: config{registrationEnabled: true}}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/users", strings.NewReader(`{`))
	app.registerUserHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusBadRequest)
	}
}
