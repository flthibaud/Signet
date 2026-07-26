package main

import (
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/validator"
)

// maxSearchQueryLength bounds the query string. Long inputs are never
// legitimate searches and ts_headline cost grows with the number of lexemes.
const maxSearchQueryLength = 200

// minSearchQueryLength rejects the short queries that search-as-you-type
// produces on the way to a real one.
const minSearchQueryLength = 2

// readLanguageTag determines which language to stem the *query* with.
func readLanguageTag(r *http.Request) string {
	if tag := strings.TrimSpace(r.URL.Query().Get("lang")); tag != "" {
		return tag
	}

	// "fr-FR,fr;q=0.9,en;q=0.8" — the first entry is the preferred one, and
	// quality-ordering the rest would not change which config we pick.
	header := r.Header.Get("Accept-Language")
	if header == "" {
		return ""
	}
	first, _, _ := strings.Cut(header, ",")
	tag, _, _ := strings.Cut(first, ";")
	return strings.TrimSpace(tag)
}

// searchHandler runs a full-text search over the authenticated user's library.
func (app *application) searchHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)
	p := app.readPagination(r)

	qs := r.URL.Query()
	var filters data.SearchFilters
	var err error

	filters.Query = strings.TrimSpace(qs.Get("q"))
	filters.Language = data.ResolveTextSearchConfig(readLanguageTag(r))

	if filters.IsRead, err = readOptionalBool(qs, "is_read"); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if filters.IsStarred, err = readOptionalBool(qs, "is_starred"); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if filters.Archived, err = readOptionalBool(qs, "archived"); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if filters.FeedID, err = readOptionalInt64(qs, "feed_id"); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if filters.Since, err = readOptionalTime(qs, "since"); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	queryLength := utf8.RuneCountInString(filters.Query)

	v := validator.New()
	v.Check(queryLength <= maxSearchQueryLength,
		"q", fmt.Sprintf("must not be more than %d characters long", maxSearchQueryLength))
	v.Check(queryLength == 0 || queryLength >= minSearchQueryLength,
		"q", fmt.Sprintf("must be at least %d characters long", minSearchQueryLength))
	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	results, hasMore, err := app.models.Links.Search(r.Context(), user.ID, filters, p.Limit(), p.Offset())
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{
		"results": results,
		"metadata": envelope{
			"query":        filters.Query,
			"current_page": p.Page,
			"page_size":    p.PageSize,
			"has_more":     hasMore,
		},
	}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}
