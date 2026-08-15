package main

import (
	"errors"
	"net/http"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/validator"
)

func (app *application) registerUserHandler(w http.ResponseWriter, r *http.Request) {
	// The gate comes before anything else for the same reason the password is
	// validated before it is hashed, below: a rejected caller must not cost a
	// bcrypt round.
	open, err := app.registrationOpen()
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}
	if !open {
		app.registrationClosedResponse(w, r)
		return
	}

	// Create an anonymous struct to hold the expected data from the request body.
	var input struct {
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}

	// Parse the request body into the anonymous struct.
	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	user := &data.User{
		Username: input.Username,
		Email:    input.Email,
	}

	// The password is checked before it is hashed, not after. bcrypt refuses
	// anything over 72 bytes outright, so hashing first turned what should be a
	// field error into a 500; and its ~250ms of CPU is not something an
	// unauthenticated caller should be able to spend on input already known to
	// be unusable.
	v := validator.New()
	if data.ValidatePasswordPlaintext(v, input.Password); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	err = user.Password.Set(input.Password)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	// Validate the user struct and return the error messages to the client if any of
	// the checks fail.
	if data.ValidateUser(v, user); !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}
	// Insert the user data into the database.
	err = app.models.Users.Insert(user)
	if err != nil {
		switch {
		// If we get a ErrDuplicateEmail error, use the v.AddError() method to manually
		// add a message to the validator instance, and then call our
		// failedValidationResponse() helper.
		case errors.Is(err, data.ErrDuplicateEmail):
			v.AddError("email", "a user with this email address already exists")
			app.failedValidationResponse(w, r, v.Errors)
		case errors.Is(err, data.ErrDuplicateUsername):
			v.AddError("username", "a user with this username already exists")
			app.failedValidationResponse(w, r, v.Errors)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}
	// Write a JSON response containing the user data along with a 201 Created status
	// code.
	err = app.writeJSON(w, http.StatusCreated, envelope{"user": user}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// registrationOpen reports whether a new account may be created. A closed
// instance still lets the very first one through: there is no CLI to create a
// user with, so an install started with the default — closed — would otherwise
// be locked out of itself.
func (app *application) registrationOpen() (bool, error) {
	if app.config.registrationEnabled {
		return true, nil
	}

	hasUsers, err := app.models.Users.HasAny()
	if err != nil {
		return false, err
	}

	return !hasUsers, nil
}

func (app *application) getCurrentUserHandler(w http.ResponseWriter, r *http.Request) {
	// requireAuthenticatedUser has already rejected anonymous callers.
	user := app.contextGetUser(r)

	err := app.writeJSON(w, http.StatusOK, envelope{"user": user}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
