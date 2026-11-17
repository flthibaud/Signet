package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	readability "codeberg.org/readeck/go-readability/v2"
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

// extractPage runs readability over a fixed page, as fetchWithReadability would
// over a fetched one.
func extractPage(t *testing.T, page string) readability.Article {
	t.Helper()
	u, _ := url.Parse("https://example.test/post")
	article, err := readability.FromReader(strings.NewReader(page), u)
	if err != nil {
		t.Fatalf("readability: %v", err)
	}
	return article
}

func TestBuildArticle(t *testing.T) {
	body := strings.Repeat("Extracted body sentence. ", 40)
	titled := extractPage(t, `<html><head><title>Page title</title></head><body><article><p>`+body+`</p></article></body></html>`)
	untitled := extractPage(t, `<html><body><article><p>`+body+`</p></article></body></html>`)
	gone := errors.New("status 404")

	tests := []struct {
		name     string
		item     *gofeed.Item
		page     readability.Article
		fetchErr error
		link     string

		wantTitle string
		wantHTML  string // a substring of OriginalHTML
		wantText  string // a substring of TextContent
		wantURL   string
		wantImage string
	}{
		{
			// Plain RSS 2.0 fills only Description; without the fallback the
			// article would be stored empty and never fetched again.
			name:      "page gone, description-only item",
			item:      &gofeed.Item{Title: "Item title", Description: "<p>Full text from the feed</p>"},
			fetchErr:  gone,
			wantTitle: "Item title",
			wantHTML:  "<p>Full text from the feed</p>",
			wantText:  "Full text from the feed",
		},
		{
			name:      "page gone, item with content",
			item:      &gofeed.Item{Title: "Item title", Content: "<p>Encoded content</p>", Description: "<p>Summary</p>"},
			fetchErr:  gone,
			wantTitle: "Item title",
			wantHTML:  "<p>Encoded content</p>",
			wantText:  "Encoded content",
		},
		{
			name:      "page extracted",
			item:      &gofeed.Item{Title: "Item title", Description: "<p>Summary</p>"},
			page:      titled,
			wantTitle: "Page title",
			wantHTML:  "Extracted body sentence.",
			wantText:  "Extracted body sentence.",
		},
		{
			// The extracted body is the best one available whether or not the
			// page has a <title>.
			name:      "page extracted without a title",
			item:      &gofeed.Item{Title: "Item title", Description: "<p>Summary</p>"},
			page:      untitled,
			wantTitle: "Item title",
			wantHTML:  "Extracted body sentence.",
			wantText:  "Extracted body sentence.",
		},
		{
			// A frameset document has no <body>, which the Markdown converter
			// rejects; the text must not be lost with it.
			name:      "body the converter rejects",
			item:      &gofeed.Item{Title: "Item title", Content: "<frameset></frameset><p>Kept text</p>"},
			fetchErr:  gone,
			wantTitle: "Item title",
			wantText:  "Kept text",
		},
		{
			// PostgreSQL rejects NUL bytes and invalid UTF-8 in text columns, so
			// one left in any field would make the item unstorable for good.
			name: "strings the database refuses",
			item: &gofeed.Item{
				Title:       "bad \x00 title \xff",
				Description: "<p>body \x00 with \xff</p>",
				Author:      &gofeed.Person{Name: "au\x00thor"},
				Image:       &gofeed.Image{URL: "https://example.test/i\x00mg\xff.png"},
			},
			fetchErr:  gone,
			link:      "https://example.test/po\x00st\xff",
			wantTitle: "bad  title ",
			wantURL:   "https://example.test/post",
			wantImage: "https://example.test/img.png",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			link := tt.link
			if link == "" {
				link = "https://example.test/post"
			}

			a := buildArticle(tt.item, tt.page, tt.fetchErr, link, "deadbeef", "simple_ua")

			if a.Title != tt.wantTitle {
				t.Errorf("Title = %q, want %q", a.Title, tt.wantTitle)
			}
			if !strings.Contains(a.OriginalHTML, tt.wantHTML) {
				t.Errorf("OriginalHTML = %q, want it to contain %q", a.OriginalHTML, tt.wantHTML)
			}
			if !strings.Contains(a.TextContent, tt.wantText) {
				t.Errorf("TextContent = %q, want it to contain %q", a.TextContent, tt.wantText)
			}
			if tt.wantURL != "" && a.Url != tt.wantURL {
				t.Errorf("Url = %q, want %q", a.Url, tt.wantURL)
			}
			if tt.wantImage != "" && a.ImageURL != tt.wantImage {
				t.Errorf("ImageURL = %q, want %q", a.ImageURL, tt.wantImage)
			}

			for field, value := range map[string]string{
				"Url":          a.Url,
				"Title":        a.Title,
				"Description":  a.Description,
				"Author":       a.Author,
				"ImageURL":     a.ImageURL,
				"OriginalHTML": a.OriginalHTML,
				"TextContent":  a.TextContent,
			} {
				if strings.ContainsRune(value, 0) || !utf8.ValidString(value) {
					t.Errorf("%s = %q, still holds a NUL byte or invalid UTF-8", field, value)
				}
			}
		})
	}
}
