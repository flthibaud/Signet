package main

import "net/http"

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
