package readability

import (
	"os"
	"strings"
	"testing"
)

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

	sourceHTML, err := os.ReadFile(dir + "/source.html")
	if err != nil {
		t.Skipf("no source.html: %v", err)
	}

	expectedMD, err := os.ReadFile(dir + "/expected.md")
	if err != nil {
		t.Skipf("no expected.md: %v", err)
	}

	r := NewReadability()
	got, err := r.HTMLToMarkdown(string(sourceHTML))
	if err != nil {
		t.Fatal(err)
	}

	expected := strings.TrimSpace(string(expectedMD))
	if got == expected {
		return
	}

	os.WriteFile(dir+"/actual.md", []byte(got), 0644)

	gotLines := strings.Split(got, "\n")
	expectedLines := strings.Split(expected, "\n")

	minLen := len(gotLines)
	if len(expectedLines) < minLen {
		minLen = len(expectedLines)
	}

	for i := 0; i < minLen; i++ {
		if gotLines[i] != expectedLines[i] {
			t.Errorf("first difference at line %d:\n  got:      %q\n  expected: %q", i+1, gotLines[i], expectedLines[i])
			break
		}
	}

	if len(gotLines) != len(expectedLines) {
		t.Errorf("line count differs: got %d, expected %d", len(gotLines), len(expectedLines))
	}

	t.Logf("wrote actual output to %s/actual.md for comparison", dir)
}
