package main

import (
	"errors"
	"net/http"

	"github.com/flthibaud/origami/internal/data"
)

func (app *application) listLinksHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)
	p := app.readPagination(r)

	links, total, err := app.models.Links.ListForUser(r.Context(), user.ID, p.Limit(), p.Offset())
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{
		"links": links,
		"metadata": envelope{
			"current_page":  p.Page,
			"page_size":     p.PageSize,
			"total_records": total,
			"total_pages":   (total + p.PageSize - 1) / p.PageSize,
		},
	}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) getLinkHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)
	slug, err := app.readSlugParam(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	link, err := app.models.Links.GetBySlug(r.Context(), slug, user.ID)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"link": link}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

func (app *application) updateLinkHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)
	slug, err := app.readSlugParam(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Only the fields present in the body are applied (partial update).
	var input struct {
		IsRead *bool `json:"is_read"`
	}

	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if input.IsRead == nil {
		app.writeJSON(w, http.StatusOK, envelope{"message": "no changes applied"}, nil)
		return
	}

	err = app.models.Links.SetReadStatus(r.Context(), user.ID, slug, *input.IsRead)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"message": "link successfully updated"}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
