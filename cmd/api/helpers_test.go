package main

import (
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

func ptr[T any](v T) *T { return &v }
