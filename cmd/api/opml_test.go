package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/opml"
)

func TestValidateOPMLEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []opml.Entry
		want    int
		valid   bool
	}{
		{
			name: "keeps http and https feeds",
			entries: []opml.Entry{
				{XMLURL: "https://a.test/feed"},
				{XMLURL: "http://b.test/feed"},
			},
			want:  2,
			valid: true,
		},
		{
			name: "drops what cannot be fetched",
			entries: []opml.Entry{
				{XMLURL: "https://a.test/feed"},
				{XMLURL: "javascript:alert(1)"},
				{XMLURL: "/relative/feed.xml"},
				{XMLURL: "file:///etc/passwd"},
				{XMLURL: ""},
			},
			want:  1,
			valid: true,
		},
		{
			// Listing the same feed twice is common; one "already subscribed"
			// line per duplicate would be noise in the report.
			name: "removes duplicates",
			entries: []opml.Entry{
				{XMLURL: "https://a.test/feed", Folder: "Tech"},
				{XMLURL: "https://a.test/feed", Folder: "Dev"},
			},
			want:  1,
			valid: true,
		},
		{
			name:    "a file with no feed is a user error",
			entries: []opml.Entry{{XMLURL: "not a url"}},
			want:    0,
			valid:   false,
		},
		{
			name:    "an empty file too",
			entries: nil,
			want:    0,
			valid:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kept, v := validateOPMLEntries(tt.entries)

			if len(kept) != tt.want {
				t.Errorf("kept %d entries, want %d: %+v", len(kept), tt.want, kept)
			}
			if v.Valid() != tt.valid {
				t.Errorf("valid = %v, want %v (errors: %v)", v.Valid(), tt.valid, v.Errors)
			}
		})
	}
}

// The cap is what keeps one request from queueing an unbounded number of feed
// fetches.
func TestValidateOPMLEntriesCap(t *testing.T) {
	entries := make([]opml.Entry, maxOPMLEntries+1)
	for i := range entries {
		entries[i] = opml.Entry{XMLURL: fmt.Sprintf("https://feed-%d.test/rss", i)}
	}

	_, v := validateOPMLEntries(entries)
	if v.Valid() {
		t.Errorf("a file of %d feeds should be rejected", len(entries))
	}

	_, v = validateOPMLEntries(entries[:maxOPMLEntries])
	if !v.Valid() {
		t.Errorf("a file of exactly %d feeds should be accepted: %v", maxOPMLEntries, v.Errors)
	}
}

func TestSubscriptionsToOPML(t *testing.T) {
	custom := "My name for it"
	empty := ""

	subscriptions := []*data.Subscription{
		{
			Feed:   data.Feed{Url: "https://korben.info/feed", Title: "Korben", SiteUrl: "https://korben.info"},
			Folder: &data.Folder{ID: 1, Name: "Tech"},
		},
		{
			// A renamed subscription exports under the name the user gave it.
			Feed:        data.Feed{Url: "https://jvns.ca/atom.xml", Title: "Julia Evans"},
			CustomTitle: &custom,
		},
		{
			// An empty custom title falls back rather than exporting nothing.
			Feed:        data.Feed{Url: "https://danluu.com/atom.xml", Title: "Dan Luu"},
			CustomTitle: &empty,
		},
	}

	entries := subscriptionsToOPML(subscriptions)

	want := []opml.Entry{
		{Title: "Korben", XMLURL: "https://korben.info/feed", HTMLURL: "https://korben.info", Folder: "Tech"},
		{Title: custom, XMLURL: "https://jvns.ca/atom.xml"},
		{Title: "Dan Luu", XMLURL: "https://danluu.com/atom.xml"},
	}

	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d", len(entries), len(want))
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("entry %d:\n got %+v\nwant %+v", i, entries[i], want[i])
		}
	}
}

// The shape of the exported document is what another reader has to swallow, so
// pin it end to end rather than only the conversion.
func TestExportedDocumentIsImportable(t *testing.T) {
	entries := subscriptionsToOPML([]*data.Subscription{
		{
			Feed:   data.Feed{Url: "https://korben.info/feed", Title: "Korben & co", SiteUrl: "https://korben.info"},
			Folder: &data.Folder{ID: 1, Name: "Tech"},
		},
		{Feed: data.Feed{Url: "https://danluu.com/atom.xml", Title: "Dan Luu"}},
	})

	var buf bytes.Buffer
	if err := opml.Write(&buf, "Signet subscriptions", entries); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if !strings.Contains(buf.String(), `<opml version="2.0">`) {
		t.Errorf("missing the OPML root:\n%s", buf.String())
	}

	back, err := opml.Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("our own export does not parse: %v", err)
	}
	if len(back) != 2 {
		t.Fatalf("got %d entries back, want 2", len(back))
	}

	byURL := map[string]opml.Entry{}
	for _, e := range back {
		byURL[e.XMLURL] = e
	}
	if got := byURL["https://korben.info/feed"]; got.Folder != "Tech" || got.Title != "Korben & co" {
		t.Errorf("round trip lost something: %+v", got)
	}
	if got := byURL["https://danluu.com/atom.xml"]; got.Folder != "" {
		t.Errorf("an unfiled feed must stay at the root, got folder %q", got.Folder)
	}
}
