package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flthibaud/signet/internal/data"
)

func TestReadFeedBody(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr error
	}{
		{"under the cap", 1024, nil},
		{"exactly at the cap", maxFeedBytes, nil},
		{"one byte over", maxFeedBytes + 1, errFeedTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := readFeedBody(bytes.NewReader(bytes.Repeat([]byte("x"), tt.size)))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && len(body) != tt.size {
				t.Errorf("read %d bytes, want %d", len(body), tt.size)
			}
		})
	}
}

// A feed server that never stops sending must not be able to grow the process
// until it dies — the read has to give up and report a bad feed.
func TestCreateFromURLRejectsOversizedFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		if _, err := w.Write([]byte(`<rss version="2.0"><channel><title>flood</title>`)); err != nil {
			return
		}
		chunk := bytes.Repeat([]byte("<item><title>x</title></item>"), 1024)
		// Bounded so a bug in the cap fails the test instead of hanging it.
		for written := 0; written < 4*maxFeedBytes; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	s := NewFeedService(data.Models{}, nil, FetchConfig{AllowPrivateNetworks: true})

	_, err := s.CreateFromURL(context.Background(), srv.URL+"/feed.xml")
	if !errors.Is(err, ErrInvalidFeed) {
		t.Fatalf("err = %v, want it to wrap ErrInvalidFeed", err)
	}
	if !strings.Contains(err.Error(), errFeedTooLarge.Error()) {
		t.Errorf("err = %v, want the size limit to be what rejected it", err)
	}
}

// The cap only means anything if it applies after decompression: a few hundred
// kilobytes of gzip expand to gigabytes.
func TestCreateFromURLRejectsGzipBomb(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	zeros := make([]byte, 1<<20)
	for i := 0; i < 32; i++ { // 32 MiB in, a few KiB out
		if _, err := zw.Write(zeros); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("Content-Encoding", "gzip")
		w.Write(compressed.Bytes())
	}))
	defer srv.Close()

	s := NewFeedService(data.Models{}, nil, FetchConfig{AllowPrivateNetworks: true})

	_, err := s.CreateFromURL(context.Background(), srv.URL+"/feed.xml")
	if !errors.Is(err, ErrInvalidFeed) {
		t.Fatalf("err = %v, want it to wrap ErrInvalidFeed", err)
	}
	if !strings.Contains(err.Error(), errFeedTooLarge.Error()) {
		t.Errorf("err = %v, want the decompressed body to hit the size limit", err)
	}
}
