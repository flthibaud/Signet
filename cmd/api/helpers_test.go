package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestReadOptionalBool(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		want    *bool
		wantErr bool
	}{
		{"absent", "", nil, false},
		{"true", "flag=true", ptr(true), false},
		{"false", "flag=false", ptr(false), false},
		{"numeric true", "flag=1", ptr(true), false},
		{"invalid", "flag=banana", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qs, _ := url.ParseQuery(tt.query)
			got, err := readOptionalBool(qs, "flag")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadOptionalInt64(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		want    *int64
		wantErr bool
	}{
		{"absent", "", nil, false},
		{"valid", "feed_id=42", ptr(int64(42)), false},
		{"zero rejected", "feed_id=0", nil, true},
		{"negative rejected", "feed_id=-3", nil, true},
		{"not a number", "feed_id=abc", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			qs, _ := url.ParseQuery(tt.query)
			got, err := readOptionalInt64(qs, "feed_id")
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if (got == nil) != (tt.want == nil) || (got != nil && *got != *tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadPagination(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantPage   int
		wantSize   int
		wantOffset int
		wantErr    bool
	}{
		{name: "defaults", query: "", wantPage: 1, wantSize: 20, wantOffset: 0},
		{name: "valid", query: "page=3&page_size=50", wantPage: 3, wantSize: 50, wantOffset: 100},
		{name: "empty values fall back to defaults", query: "page=&page_size=", wantPage: 1, wantSize: 20, wantOffset: 0},
		{name: "page not a number", query: "page=abc", wantErr: true},
		{name: "page zero", query: "page=0", wantErr: true},
		{name: "page negative", query: "page=-1", wantErr: true},
		{name: "page_size not a number", query: "page_size=abc", wantErr: true},
		{name: "page_size zero", query: "page_size=0", wantErr: true},
		{name: "page_size above max", query: "page_size=101", wantErr: true},
		{name: "deepest allowed offset", query: "page=501", wantPage: 501, wantSize: 20, wantOffset: 10000},
		{name: "offset too deep", query: "page=502", wantErr: true},
		{name: "offset too deep at max page_size", query: "page=102&page_size=100", wantErr: true},
		{name: "page too large to multiply", query: "page=9223372036854775807", wantErr: true},
	}

	app := &application{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/v1/links?"+tt.query, nil)

			got, err := app.readPagination(r)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if got.Page != tt.wantPage || got.PageSize != tt.wantSize {
				t.Errorf("got page=%d page_size=%d, want page=%d page_size=%d",
					got.Page, got.PageSize, tt.wantPage, tt.wantSize)
			}
			if got.Offset() != tt.wantOffset {
				t.Errorf("Offset() = %d, want %d", got.Offset(), tt.wantOffset)
			}
		})
	}
}

func ptr[T any](v T) *T { return &v }
