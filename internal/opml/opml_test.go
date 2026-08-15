package opml

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func parseFile(t *testing.T, name string) []Entry {
	t.Helper()

	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatalf("opening %s: %v", name, err)
	}
	defer f.Close()

	entries, err := Parse(f)
	if err != nil {
		t.Fatalf("Parse(%s): %v", name, err)
	}

	return entries
}

// TestParseRealExports covers the shapes actual readers produce: a flat list, a
// single level of folders, and arbitrary nesting.
func TestParseRealExports(t *testing.T) {
	tests := []struct {
		file string
		want []Entry
	}{
		{
			file: "inoreader_flat.opml",
			want: []Entry{
				{Title: "The Go Blog", XMLURL: "https://go.dev/blog/feed.atom", HTMLURL: "https://go.dev/blog"},
				{Title: "Julia Evans", XMLURL: "https://jvns.ca/atom.xml", HTMLURL: "https://jvns.ca"},
				{Title: "Korben", XMLURL: "https://korben.info/feed", HTMLURL: "https://korben.info"},
			},
		},
		{
			file: "feedly_folders.opml",
			want: []Entry{
				{Title: "Korben", XMLURL: "https://korben.info/feed", HTMLURL: "https://korben.info", Folder: "Tech"},
				{Title: "Ars Technica", XMLURL: "https://feeds.arstechnica.com/arstechnica/index", HTMLURL: "https://arstechnica.com", Folder: "Tech"},
				{Title: "The Go Blog", XMLURL: "https://go.dev/blog/feed.atom", HTMLURL: "https://go.dev/blog", Folder: "Dev"},
				// At the root: stays unfiled rather than landing in the last folder seen.
				{Title: "Dan Luu", XMLURL: "https://danluu.com/atom.xml", HTMLURL: "https://danluu.com"},
			},
		},
		{
			file: "nested.opml",
			want: []Entry{
				{Title: "The Go Blog", XMLURL: "https://go.dev/blog/feed.atom", Folder: "Tech / Langages / Go"},
				{Title: "Eli Bendersky", XMLURL: "https://eli.thegreenplace.net/feeds/all.atom.xml", Folder: "Tech / Langages"},
				{Title: "Ars Technica", XMLURL: "https://feeds.arstechnica.com/arstechnica/index", Folder: "Tech"},
				// Beyond maxFolderDepth the path stops growing instead of
				// producing a name nothing can display.
				{Title: "Too deep", XMLURL: "https://example.com/deep.xml", Folder: "A / B / C / D / E"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			got := parseFile(t, tt.file)

			if len(got) != len(tt.want) {
				t.Fatalf("got %d entries, want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("entry %d:\n got %+v\nwant %+v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// TestParseQuirks pins the tolerances that decide whether a user silently loses
// feeds: attribute casing, title= as a synonym of text=, unescaped ampersands.
func TestParseQuirks(t *testing.T) {
	entries := parseFile(t, "quirks.opml")

	want := []Entry{
		{Title: "Uppercase attrs", XMLURL: "https://example.com/upper.xml"},
		{Title: "Title only", XMLURL: "https://example.com/title-only.xml"},
		{Title: "Rock & Roll", XMLURL: "https://example.com/rock.xml"},
		// No text and no title: the URL is a better label than an empty string.
		{Title: "https://example.com/anonymous.xml", XMLURL: "https://example.com/anonymous.xml"},
	}

	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(entries), len(want), entries)
	}
	for i := range want {
		if entries[i] != want[i] {
			t.Errorf("entry %d:\n got %+v\nwant %+v", i, entries[i], want[i])
		}
	}
}

func TestParseEmptyBody(t *testing.T) {
	entries := parseFile(t, "empty.opml")
	if len(entries) != 0 {
		t.Errorf("got %d entries, want none: %+v", len(entries), entries)
	}
}

func TestParseMalformed(t *testing.T) {
	f, err := os.Open("testdata/malformed.opml")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if _, err := Parse(f); err == nil {
		t.Error("want an error on a truncated document, got nil")
	}
}

// TestParseIgnoresExternalEntities guards the XXE case: the file must be
// rejected or read as harmless text, never resolved against the filesystem.
func TestParseIgnoresExternalEntities(t *testing.T) {
	const doc = `<?xml version="1.0"?>
<!DOCTYPE opml [<!ENTITY xxe SYSTEM "file:///etc/passwd">]>
<opml version="2.0"><body>
  <outline type="rss" text="&xxe;" xmlUrl="https://example.com/feed.xml"/>
</body></opml>`

	entries, err := Parse(strings.NewReader(doc))
	if err != nil {
		return // rejecting outright is fine too
	}

	for _, e := range entries {
		if strings.Contains(e.Title, "root:") {
			t.Fatalf("external entity was resolved: %q", e.Title)
		}
	}
}

func TestWriteGroupsByFolder(t *testing.T) {
	var buf bytes.Buffer

	err := Write(&buf, "Signet subscriptions", []Entry{
		{Title: "Dan Luu", XMLURL: "https://danluu.com/atom.xml"},
		{Title: "Korben", XMLURL: "https://korben.info/feed", Folder: "Tech"},
		{Title: "The Go Blog", XMLURL: "https://go.dev/blog/feed.atom", Folder: "Dev"},
		{Title: "Ars Technica", XMLURL: "https://feeds.arstechnica.com/arstechnica/index", Folder: "Tech"},
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := buf.String()

	if !strings.HasPrefix(got, "<?xml") {
		t.Error("missing XML declaration")
	}

	// Folders come out sorted, each holding its own feeds, and the unfiled one
	// stays at the root.
	for _, fragment := range []string{
		`<outline text="Dev">`,
		`<outline text="Tech">`,
		`text="Dan Luu"`,
	} {
		if !strings.Contains(got, fragment) {
			t.Errorf("output is missing %s:\n%s", fragment, got)
		}
	}

	if strings.Index(got, `text="Dev"`) > strings.Index(got, `text="Tech"`) {
		t.Errorf("folders are not sorted:\n%s", got)
	}
}

func TestWriteEscapesTitles(t *testing.T) {
	var buf bytes.Buffer

	err := Write(&buf, "Signet", []Entry{
		{Title: `Rock & <Roll>`, XMLURL: "https://example.com/feed.xml?a=1&b=2"},
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	got := buf.String()
	if strings.Contains(got, "& ") || strings.Contains(got, "<Roll>") {
		t.Errorf("title was not escaped:\n%s", got)
	}

	// And what is written must read back identically.
	entries, err := Parse(strings.NewReader(got))
	if err != nil {
		t.Fatalf("re-parsing our own output: %v", err)
	}
	if len(entries) != 1 || entries[0].Title != `Rock & <Roll>` {
		t.Errorf("round trip lost the title: %+v", entries)
	}
}

func TestWriteEmpty(t *testing.T) {
	var buf bytes.Buffer

	if err := Write(&buf, "Signet", nil); err != nil {
		t.Fatalf("Write: %v", err)
	}

	// A user with no subscription still gets a file every reader accepts.
	if !strings.Contains(buf.String(), "<body>") {
		t.Errorf("empty export has no body:\n%s", buf.String())
	}

	entries, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("re-parsing an empty export: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want none", len(entries))
	}
}

// TestRoundTrip is the property that matters for a user moving between
// readers: exporting then re-importing must not lose or rename anything.
func TestRoundTrip(t *testing.T) {
	original := parseFile(t, "feedly_folders.opml")

	var buf bytes.Buffer
	if err := Write(&buf, "Signet subscriptions", original); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(got) != len(original) {
		t.Fatalf("got %d entries after a round trip, want %d", len(got), len(original))
	}

	// Order changes (folders are sorted), so compare as a set keyed by URL.
	index := map[string]Entry{}
	for _, e := range got {
		index[e.XMLURL] = e
	}
	for _, want := range original {
		if index[want.XMLURL] != want {
			t.Errorf("round trip changed %s:\n got %+v\nwant %+v", want.XMLURL, index[want.XMLURL], want)
		}
	}
}

// An uploaded file decides how deep UnmarshalXML recurses, so the parser caps
// it rather than trusting the input.
func TestParseRejectsDeepNesting(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<opml version="2.0"><head><title>t</title></head><body>`)
	depth := maxNestingDepth + 10
	for range depth {
		sb.WriteString(`<outline text="f">`)
	}
	sb.WriteString(`<outline text="a" xmlUrl="https://example.com/f.xml"/>`)
	for range depth {
		sb.WriteString(`</outline>`)
	}
	sb.WriteString(`</body></opml>`)

	if _, err := Parse(strings.NewReader(sb.String())); !errors.Is(err, ErrTooDeep) {
		t.Errorf("got %v, want ErrTooDeep", err)
	}
}

// A tree within the cap still parses, so the guard cannot reject real files.
func TestParseAcceptsNestingWithinCap(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<opml version="2.0"><head><title>t</title></head><body>`)
	for range maxNestingDepth - 1 {
		sb.WriteString(`<outline text="f">`)
	}
	sb.WriteString(`<outline text="a" xmlUrl="https://example.com/f.xml"/>`)
	for range maxNestingDepth - 1 {
		sb.WriteString(`</outline>`)
	}
	sb.WriteString(`</body></opml>`)

	entries, err := Parse(strings.NewReader(sb.String()))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
}
