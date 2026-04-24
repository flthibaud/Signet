package readability

// Integration tests: full HTML pages → markdown golden files.
//
// Each site under test/test-pages/ can have one or more source files:
//
//   source.html        → expected.md         (default)
//   source-variant1.html → expected-variant1.md
//   ...
//
// When a preprocessing rule changes and the output shifts intentionally,
// regenerate all golden files with:
//
//   go test ./internal/readability/... -update
//
// To add a new site: drop source.html in a new subdirectory, run -update
// to generate the expected.md, review it, then commit both files.

import (
	"flag"
	"os"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "regenerate golden expected.md files")

// TestHTMLToMarkdown runs one sub-test per site directory, and within each
// site, one sub-test per source*.html file found.
func TestHTMLToMarkdown(t *testing.T) {
	entries, err := os.ReadDir("test/test-pages")
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		site := e.Name()
		t.Run(site, func(t *testing.T) {
			runSiteTest(t, site)
		})
	}
}

func runSiteTest(t *testing.T, site string) {
	t.Helper()
	dir := "test/test-pages/" + site

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("cannot read dir: %v", err)
	}

	found := false
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "source") || !strings.HasSuffix(name, ".html") {
			continue
		}
		found = true

		// "source-variant1.html" → suffix="-variant1" → subName="variant1"
		suffix := strings.TrimPrefix(strings.TrimSuffix(name, ".html"), "source")
		sourceFile := dir + "/source" + suffix + ".html"
		expectedFile := dir + "/expected" + suffix + ".md"

		subName := "default"
		if suffix != "" {
			subName = strings.TrimPrefix(suffix, "-")
		}

		t.Run(subName, func(t *testing.T) {
			runFileTest(t, sourceFile, expectedFile)
		})
	}

	if !found {
		t.Skip("no source*.html files")
	}
}

func runFileTest(t *testing.T, sourceFile, expectedFile string) {
	t.Helper()

	sourceHTML, err := os.ReadFile(sourceFile)
	if err != nil {
		t.Skipf("missing source file: %v", err)
	}

	r := NewReadability()
	got, err := r.HTMLToMarkdown(string(sourceHTML))
	if err != nil {
		t.Fatal(err)
	}

	// -update: overwrite the golden file and stop — no comparison needed.
	if *update {
		if err := os.WriteFile(expectedFile, []byte(got+"\n"), 0644); err != nil {
			t.Fatalf("update failed: %v", err)
		}
		t.Logf("updated %s", expectedFile)
		return
	}

	raw, err := os.ReadFile(expectedFile)
	if err != nil {
		// No golden file yet — skip rather than fail; run with -update to create it.
		t.Skipf("no golden file %s (run with -update to generate it)", expectedFile)
	}

	expected := strings.TrimSpace(string(raw))
	if got == expected {
		return
	}

	// Write the actual output next to the golden file for easy diffing.
	actualFile := strings.TrimSuffix(expectedFile, ".md") + "_actual.md"
	_ = os.WriteFile(actualFile, []byte(got+"\n"), 0644)

	gotLines := strings.Split(got, "\n")
	expectedLines := strings.Split(expected, "\n")

	minLen := min(len(gotLines), len(expectedLines))
	for i := range minLen {
		if gotLines[i] != expectedLines[i] {
			t.Errorf("first difference at line %d:\n  got:      %q\n  expected: %q", i+1, gotLines[i], expectedLines[i])
			break
		}
	}

	if len(gotLines) != len(expectedLines) {
		t.Errorf("line count: got %d, expected %d", len(gotLines), len(expectedLines))
	}

	t.Logf("actual output written to %s", actualFile)
}
