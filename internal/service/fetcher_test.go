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
	"unicode/utf8"

	"github.com/flthibaud/signet/internal/data"
	"github.com/mmcdole/gofeed"
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

// A feed item that carries only <description> (Content stays empty in plain
// RSS 2.0) must still get a body when the article page cannot be scraped —
// otherwise the article is stored empty and never fetched again.
func TestCreateArticleFromItemFallsBackToDescription(t *testing.T) {
	// The article page is gone, so fetchWithReadability fails and the
	// fallback to the feed item's own text runs.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s, _ := newSyncTestService(t)

	item := &gofeed.Item{
		Title:       "Protected post",
		Link:        srv.URL + "/post",
		Description: "<p>Full text from the feed</p>",
		// Content deliberately left empty: only <content:encoded> fills it.
	}

	article, err := s.createArticleFromItem(context.Background(), item, srv.URL+"/post", "deadbeef", "simple_ua")
	if err != nil {
		t.Fatalf("createArticleFromItem: %v", err)
	}
	if article.OriginalHTML != "<p>Full text from the feed</p>" {
		t.Errorf("OriginalHTML = %q, want the item description", article.OriginalHTML)
	}
	if !strings.Contains(article.TextContent, "Full text from the feed") {
		t.Errorf("TextContent = %q, want the description converted to markdown", article.TextContent)
	}
}

// PostgreSQL rejects NUL bytes and invalid UTF-8 in text columns. A feed item
// carrying either (a JSON Feed title with \u0000 does) would fail the insert on
// every sync — and since a failed item keeps the feed's ETag from advancing,
// the whole feed would be re-downloaded and the article page re-scraped
// forever. Every string must be cleaned before the insert.
func TestCreateArticleFromItemCleansUnstorableStrings(t *testing.T) {
	// The article page is gone, so the fallback to the feed item's own text runs.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	s, _ := newSyncTestService(t)

	item := &gofeed.Item{
		Title:       "bad \x00 title \xff",
		Link:        srv.URL + "/post",
		Description: "<p>body \x00 with \xff</p>",
		Author:      &gofeed.Person{Name: "au\x00thor"},
	}

	article, err := s.createArticleFromItem(context.Background(), item, srv.URL+"/post", "deadbeef", "simple_ua")
	if err != nil {
		t.Fatalf("createArticleFromItem: %v", err)
	}
	if article.Title != "bad  title " {
		t.Errorf("Title = %q, want the NUL byte and the invalid byte stripped", article.Title)
	}
	for field, value := range map[string]string{
		"Description":  article.Description,
		"Author":       article.Author,
		"OriginalHTML": article.OriginalHTML,
		"TextContent":  article.TextContent,
	} {
		if strings.ContainsRune(value, 0) {
			t.Errorf("%s = %q, still contains a NUL byte", field, value)
		}
		if !utf8.ValidString(value) {
			t.Errorf("%s = %q, still contains invalid UTF-8", field, value)
		}
	}
}
