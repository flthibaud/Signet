package main

import (
	"errors"
	"net/http"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/validator"
)

func (app *application) listLinksHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	p, err := app.readPagination(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	qs := r.URL.Query()
	var filters data.LinkFilters

	if filters.IsRead, err = readOptionalBool(qs, "is_read"); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if filters.IsStarred, err = readOptionalBool(qs, "is_starred"); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	archived, err := readOptionalBool(qs, "archived")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if archived == nil {
		archived = new(bool)
	}
	filters.Archived = archived
	if filters.FeedID, err = readOptionalInt64(qs, "feed_id"); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	links, hasMore, err := app.models.Links.ListForUser(r.Context(), user.ID, filters, p.Limit(), p.Offset())
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{
		"links": links,
		"metadata": envelope{
			"current_page": p.Page,
			"page_size":    p.PageSize,
			"has_more":     hasMore,
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
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
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
		IsRead                     *bool    `json:"is_read"`
		IsStarred                  *bool    `json:"is_starred"`
		Archived                   *bool    `json:"archived"`
		ReadingProgress            *float64 `json:"reading_progress"`
		ReadingProgressAnchorIndex *int     `json:"reading_progress_anchor_index"`
	}

	err = app.readJSON(w, r, &input)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	v := validator.New()
	if input.ReadingProgress != nil {
		v.Check(*input.ReadingProgress >= 0 && *input.ReadingProgress <= 1,
			"reading_progress", "must be between 0 and 1")
	}
	if input.ReadingProgressAnchorIndex != nil {
		v.Check(*input.ReadingProgressAnchorIndex >= 0,
			"reading_progress_anchor_index", "must not be negative")
	}
	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	upd := data.LinkUpdate{
		IsRead:                     input.IsRead,
		IsStarred:                  input.IsStarred,
		Archived:                   input.Archived,
		ReadingProgress:            input.ReadingProgress,
		ReadingProgressAnchorIndex: input.ReadingProgressAnchorIndex,
	}

	if upd.IsEmpty() {
		err = app.writeJSON(w, http.StatusOK, envelope{"message": "no changes applied"}, nil)
		if err != nil {
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.models.Links.Update(r.Context(), user.ID, slug, upd)
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
