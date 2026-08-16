package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
)

type envelope map[string]any

// Pagination holds page/page_size values parsed from query string parameters.
type Pagination struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// Limit returns the SQL LIMIT value.
func (p Pagination) Limit() int {
	return p.PageSize
}

// Offset returns the SQL OFFSET value.
func (p Pagination) Offset() int {
	return (p.Page - 1) * p.PageSize
}

const (
	defaultPageSize = 20
	maxPageSize     = 100

	// maxOffset caps how deep a client may paginate. The list and search
	// queries end in `ORDER BY ... LIMIT n OFFSET m`, which Postgres answers by
	// walking and discarding the first m rows — cost grows with the offset, and
	// nothing beyond a few hundred pages is a real user scrolling.
	maxOffset = 10_000
)

// readPagination extracts page and page_size from query params with defaults
// and bounds. Like the readOptional* helpers, it rejects values it cannot use
// rather than silently falling back to the default.
func (app *application) readPagination(r *http.Request) (Pagination, error) {
	qs := r.URL.Query()

	page := 1
	if v := qs.Get("page"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return Pagination{}, errors.New("invalid page parameter: must be a positive integer")
		}
		page = n
	}

	pageSize := defaultPageSize
	if v := qs.Get("page_size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > maxPageSize {
			return Pagination{}, fmt.Errorf("invalid page_size parameter: must be between 1 and %d", maxPageSize)
		}
		pageSize = n
	}

	// Division rather than (page-1)*pageSize so an in-range page that would
	// overflow the multiplication is still caught here.
	if page-1 > maxOffset/pageSize {
		return Pagination{}, fmt.Errorf("invalid page parameter: must not skip more than %d records; narrow the query or raise page_size", maxOffset)
	}

	return Pagination{Page: page, PageSize: pageSize}, nil
}

// readOptionalBool parses an optional boolean query parameter. It returns nil
// when the parameter is absent, and an error when the value isn't a boolean.
func readOptionalBool(qs url.Values, key string) (*bool, error) {
	v := qs.Get(key)
	if v == "" {
		return nil, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return nil, fmt.Errorf("invalid %s parameter: must be true or false", key)
	}
	return &b, nil
}

// readOptionalInt64 parses an optional positive integer query parameter. It
// returns nil when the parameter is absent.
func readOptionalInt64(qs url.Values, key string) (*int64, error) {
	v := qs.Get(key)
	if v == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 1 {
		return nil, fmt.Errorf("invalid %s parameter: must be a positive integer", key)
	}
	return &n, nil
}

func readOptionalTime(qs url.Values, key string) (*time.Time, error) {
	v := qs.Get(key)
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return nil, fmt.Errorf("invalid %s parameter: must be an RFC3339 timestamp", key)
	}
	return &t, nil
}

// readIDParam returns the ":id" path segment as an int64. Zero and negative
// values are rejected alongside unparseable ones, since no row this API exposes
// has such an ID and letting one through would only turn into a "not found" a
// query later.
func (app *application) readIDParam(r *http.Request) (int64, error) {
	params := httprouter.ParamsFromContext(r.Context())
	id, err := strconv.ParseInt(params.ByName("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid id parameter")
	}
	return id, nil
}

// readSlugParam returns the ":slug" path segment, which identifies a link
// within one user's library.
func (app *application) readSlugParam(r *http.Request) (string, error) {
	params := httprouter.ParamsFromContext(r.Context())
	slug := params.ByName("slug")
	if slug == "" {
		return "", errors.New("invalid slug parameter")
	}
	return slug, nil
}

// writeJSON sends data as the response body, with status and any extra headers
// (headers may be nil).
//
// It marshals into a buffer before touching the ResponseWriter: encoding
// straight into the writer would have already sent a 200 and a partial body by
// the time an encoding error surfaced, leaving no way to report it. Returning
// the error here lets the caller fall back to serverErrorResponse.
func (app *application) writeJSON(w http.ResponseWriter, status int, data envelope, headers http.Header) error {
	js, err := json.Marshal(data)
	if err != nil {
		return err
	}

	// A trailing newline, so the output is readable straight from a terminal.
	js = append(js, '\n')

	for key, value := range headers {
		w.Header()[key] = value
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(js)

	return nil
}

// readJSON decodes the request body into dst, returning errors phrased for the
// client rather than Go's default decoder messages, which leak type names.
//
// The body is capped at 1MB and unknown fields are refused, so a typo in a
// field name is reported instead of being silently ignored.
func (app *application) readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	maxBytes := 1_048_576
	r.Body = http.MaxBytesReader(w, r.Body, int64(maxBytes))

	// Initialize the json.Decoder, and call the DisallowUnknownFields() method on it
	// before decoding. This means that if the JSON from the client now includes any
	// field which cannot be mapped to the target destination, the decoder will return
	// an error instead of just ignoring the field.
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	// Decode the request body to the destination.
	err := dec.Decode(dst)
	if err != nil {
		var syntaxError *json.SyntaxError
		var unmarshalTypeError *json.UnmarshalTypeError
		var invalidUnmarshalError *json.InvalidUnmarshalError
		// Add a new maxBytesError variable.
		var maxBytesError *http.MaxBytesError

		switch {
		case errors.As(err, &syntaxError):
			return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)

		case errors.Is(err, io.ErrUnexpectedEOF):
			return errors.New("body contains badly-formed JSON")

		case errors.As(err, &unmarshalTypeError):
			if unmarshalTypeError.Field != "" {
				return fmt.Errorf("body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
			}

			return fmt.Errorf("body contains incorrect JSON type (at character %d)", unmarshalTypeError.Offset)
		case errors.Is(err, io.EOF):
			return errors.New("body must not be empty")
		// If the JSON contains a field which cannot be mapped to the target destination
		// then Decode() will now return an error message in the format "json: unknown
		// field "<name>"". We check for this, extract the field name from the error,
		// and interpolate it into our custom error message. Note that there's an open
		// issue at https://github.com/golang/go/issues/29035 regarding turning this
		// into a distinct error type in the future.
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
			return fmt.Errorf("body contains unknown key %s", fieldName)
		// Use the errors.As() function to check whether the error has the type
		// *http.MaxBytesError. If it does, then it means the request body exceeded our
		// size limit of 1MB and we return a clear error message.
		case errors.As(err, &maxBytesError):
			return fmt.Errorf("body must not be larger than %d bytes", maxBytesError.Limit)

		case errors.As(err, &invalidUnmarshalError):
			panic(err)

		default:
			return err
		}
	}

	// Call Decode() again, using a pointer to an empty anonymous struct as the
	// destination. If the request body only contained a single JSON value this will
	// return an io.EOF error. So if we get anything else, we know that there is
	// additional data in the request body and we return our own custom error message.
	err = dec.Decode(&struct{}{})
	if err != io.EOF {
		return errors.New("body must only contain a single JSON value")
	}

	return nil
}
