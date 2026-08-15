package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/opml"
	"github.com/flthibaud/signet/internal/validator"
)

// maxOPMLBytes bounds an uploaded subscription list. A thousand feeds fit in
// well under a megabyte; past that the file is not a subscription list.
const maxOPMLBytes = 2 << 20 // 2 MiB

// maxOPMLEntries bounds how much work one request can queue. Every entry costs
// an HTTP fetch of its feed, so this is a rate limit as much as a size limit.
const maxOPMLEntries = 1000

// importOPMLHandler accepts a subscription list and starts importing it.
func (app *application) importOPMLHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	r.Body = http.MaxBytesReader(w, r.Body, maxOPMLBytes)

	entries, err := opml.Parse(r.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		switch {
		case errors.As(err, &maxBytesError):
			app.badRequestResponse(w, r, fmt.Errorf("the file must not be larger than %d MB", maxOPMLBytes>>20))
		case errors.Is(err, opml.ErrTooDeep):
			app.badRequestResponse(w, r, errors.New("the file nests its folders too deeply"))
		default:
			app.badRequestResponse(w, r, errors.New("the file could not be read as OPML"))
		}
		return
	}

	entries, v := validateOPMLEntries(entries)
	if !v.Valid() {
		app.failedValidationResponse(w, r, v.Errors)
		return
	}

	imp, err := app.services.OPMLService.StartImport(r.Context(), user.ID, entries)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	// the subscriptions are not created yet. The client follows the job
	// through GET /v1/opml/imports/latest.
	err = app.writeJSON(w, http.StatusAccepted, envelope{"import": imp}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// validateOPMLEntries drops what cannot be subscribed to and reports what makes
// the whole file unusable.
func validateOPMLEntries(entries []opml.Entry) ([]opml.Entry, *validator.Validator) {
	v := validator.New()

	seen := make(map[string]struct{}, len(entries))
	kept := make([]opml.Entry, 0, len(entries))

	for _, entry := range entries {
		if !validator.IsURL(entry.XMLURL) {
			continue
		}
		if _, dup := seen[entry.XMLURL]; dup {
			continue
		}
		seen[entry.XMLURL] = struct{}{}
		kept = append(kept, entry)
	}

	v.Check(len(kept) > 0, "file", "no feed was found in this file")
	v.Check(len(kept) <= maxOPMLEntries, "file", fmt.Sprintf("must not contain more than %d feeds", maxOPMLEntries))

	return kept, v
}

// latestOPMLImportHandler reports on the user's most recent import.
func (app *application) latestOPMLImportHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	imp, err := app.models.OPMLImports.GetLatestForUser(r.Context(), user.ID)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			app.notFoundResponse(w, r)
		default:
			app.serverErrorResponse(w, r, err)
		}
		return
	}

	err = app.writeJSON(w, http.StatusOK, envelope{"import": imp}, nil)
	if err != nil {
		app.serverErrorResponse(w, r, err)
	}
}

// subscriptionsToOPML turns what the listing returns into what the file needs.
func subscriptionsToOPML(subscriptions []*data.Subscription) []opml.Entry {
	entries := make([]opml.Entry, 0, len(subscriptions))

	for _, sub := range subscriptions {
		title := sub.Feed.Title
		if sub.CustomTitle != nil && *sub.CustomTitle != "" {
			title = *sub.CustomTitle
		}

		entry := opml.Entry{
			Title:   title,
			XMLURL:  sub.Feed.Url,
			HTMLURL: sub.Feed.SiteUrl,
		}
		if sub.Folder != nil {
			entry.Folder = sub.Folder.Name
		}

		entries = append(entries, entry)
	}

	return entries
}

// exportOPMLHandler writes the user's subscriptions as an OPML file.
func (app *application) exportOPMLHandler(w http.ResponseWriter, r *http.Request) {
	user := app.contextGetUser(r)

	subscriptions, err := app.models.Subscriptions.GetAllForUser(r.Context(), user.ID)
	if err != nil {
		app.serverErrorResponse(w, r, err)
		return
	}

	entries := subscriptionsToOPML(subscriptions)

	filename := fmt.Sprintf("signet-subscriptions-%s.opml", time.Now().Format("2006-01-02"))

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")

	if err := opml.Write(w, "Signet subscriptions", entries); err != nil {
		app.logger.PrintError(err, map[string]string{
			"context": "writing an OPML export",
			"user_id": user.ID.String(),
		})
	}
}
