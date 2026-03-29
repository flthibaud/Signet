package readability

import (
	"os"
	"strings"
	"testing"
)

func TestHTMLToMarkdown_Numerama(t *testing.T) {
	sourceHTML, err := os.ReadFile("test/test-pages/numerama/source.html")
	if err != nil {
		t.Fatal(err)
	}

	expectedMD, err := os.ReadFile("test/test-pages/numerama/expected.md")
	if err != nil {
		t.Fatal(err)
	}

	r := NewReadability()
	got, err := r.HTMLToMarkdown(string(sourceHTML))
	if err != nil {
		t.Fatal(err)
	}

	expected := strings.TrimSpace(string(expectedMD))

	if got != expected {
		// Write actual output for debugging
		os.WriteFile("test/test-pages/numerama/actual.md", []byte(got), 0644)

		// Find first difference for better error message
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

		t.Log("Wrote actual output to test/test-pages/numerama/actual.md for comparison")
	}
}

func TestHTMLToMarkdown_LeMonde(t *testing.T) {
	sourceHTML, err := os.ReadFile("test/test-pages/lemonde/source.html")
	if err != nil {
		t.Fatal(err)
	}

	expectedMD, err := os.ReadFile("test/test-pages/lemonde/expected.md")
	if err != nil {
		t.Fatal(err)
	}

	r := NewReadability()
	got, err := r.HTMLToMarkdown(string(sourceHTML))
	if err != nil {
		t.Fatal(err)
	}

	expected := strings.TrimSpace(string(expectedMD))

	if got != expected {
		// Write actual output for debugging
		os.WriteFile("test/test-pages/lemonde/actual.md", []byte(got), 0644)

		// Find first difference for better error message
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

		t.Log("Wrote actual output to test/test-pages/lemonde/actual.md for comparison")
	}
}
