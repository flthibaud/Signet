package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/validator"
)

const maxFolderNameLength = 255

func validateFolderName(v *validator.Validator, name string) {
	v.Check(name != "", "name", "must be provided")
	v.Check(len(name) <= maxFolderNameLength, "name", "must not be more than 255 characters long")
}

func (app *application) listFoldersHandler(w http.ResponseWriter, r *http.Request) {
	userID := app.contextGetUser(r).ID

	folders, err := app.models.Folders.GetAllForUser(r.Context(), userID)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"folders": folders}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) createFolderHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name"`
	}

	err := app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	name := strings.TrimSpace(input.Name)

	v := validator.New()
	validateFolderName(v, name)
	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	folder := &data.Folder{
		UserID: app.contextGetUser(r).ID,
		Name:   name,
	}

	err = app.models.Folders.Insert(r.Context(), folder)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrDuplicateFolder):
			v.AddError("name", "you already have a folder with this name")
			app.failedValidationResponse(w, r, v.Errors)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusCreated, envelope{"folder": folder}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) updateFolderHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	var input struct {
		Name string `json:"name"`
	}

	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	name := strings.TrimSpace(input.Name)

	v := validator.New()
	validateFolderName(v, name)
	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	userID := app.contextGetUser(r).ID

	err = app.models.Folders.Update(r.Context(), userID, id, name)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		case errors.Is(err, data.ErrDuplicateFolder):
			v.AddError("name", "you already have a folder with this name")
			app.failedValidationResponse(w, r, v.Errors)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	folder, err := app.models.Folders.Get(r.Context(), userID, id)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"folder": folder}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) deleteFolderHandler(w http.ResponseWriter, r *http.Request) {
	id, err := app.readIDParam(r)
	if err != nil {
		app.notFoundResponse(w, r)
		return
	}

	userID := app.contextGetUser(r).ID

	err = app.models.Folders.Delete(r.Context(), userID, id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	// Deleting a folder never unsubscribes: its feeds come back as unfiled.
	err = app.writeJSON(w, http.StatusOK, envelope{"message": "folder successfully deleted"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
