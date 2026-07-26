package data

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestTsqueryExpr(t *testing.T) {
	expr := tsqueryExpr(2, "french_ua")

	// The whole point of the CASE is that a stopword-only query, which
	// websearch_to_tsquery renders as the empty string, must not be fed to the
	// tsquery cast — '' is not a valid tsquery and would error at runtime.
	if !strings.Contains(expr, "NULL::tsquery") {
		t.Errorf("expr must collapse an empty query to NULL, got %q", expr)
	}
	if !strings.Contains(expr, "':*'") {
		t.Errorf("expr must append the prefix marker, got %q", expr)
	}
	if strings.Count(expr, "$2") != 2 {
		t.Errorf("expr must reference $2 in both branches, got %q", expr)
	}
	if !strings.Contains(expr, "'french_ua'") {
		t.Errorf("expr must use the given config, got %q", expr)
	}
}

func TestSearchQueryExpr(t *testing.T) {
	t.Run("neutral language yields a single tsquery", func(t *testing.T) {
		expr := searchQueryExpr(2, SimpleTextSearchConfig)
		if strings.Contains(expr, "COALESCE") {
			t.Errorf("no OR needed when both halves are the same config, got %q", expr)
		}
		if strings.Count(expr, SimpleTextSearchConfig) != 2 {
			t.Errorf("expr should reference the neutral config twice, got %q", expr)
		}
	})

	t.Run("empty language is treated as neutral", func(t *testing.T) {
		if searchQueryExpr(2, "") != searchQueryExpr(2, SimpleTextSearchConfig) {
			t.Error("empty language must behave like the neutral config")
		}
	})

	t.Run("real language ORs both halves", func(t *testing.T) {
		expr := searchQueryExpr(2, "french_ua")

		if !strings.Contains(expr, "french_ua") || !strings.Contains(expr, SimpleTextSearchConfig) {
			t.Errorf("expr must combine both configs, got %q", expr)
		}
		// `tsquery || NULL` is NULL, so a term that is a stopword in one config
		// must not be able to wipe out the other half.
		if !strings.HasPrefix(expr, "COALESCE(") {
			t.Errorf("expr must guard against a NULL half, got %q", expr)
		}
		// The tsquery OR joins the two CASE branches; the other `||` in the
		// expression are string concatenations building each branch.
		if strings.Count(expr, "END) || (CASE") != 1 {
			t.Errorf("expr should OR the two tsqueries exactly once, got %q", expr)
		}
	})
}

func TestResolveTextSearchConfig(t *testing.T) {
	tests := []struct {
		tag  string
		want string
	}{
		{"fr", "french_ua"},
		{"fr-FR", "french_ua"},
		{"fr_FR", "french_ua"},
		{"FR", "french_ua"},
		{" en-US ", "english_ua"},
		{"it", "italian_ua"},
		{"nb", "norwegian_ua"},
		// No stemmer ships for these, and the default parser cannot even
		// tokenize the first two — the neutral config is the honest answer.
		{"ja", SimpleTextSearchConfig},
		{"zh-Hans", SimpleTextSearchConfig},
		{"", SimpleTextSearchConfig},
		{"-", SimpleTextSearchConfig},
		{"klingon", SimpleTextSearchConfig},
		// Must not resolve to anything injectable.
		{"french_ua'; DROP TABLE articles; --", SimpleTextSearchConfig},
	}

	for _, tt := range tests {
		if got := ResolveTextSearchConfig(tt.tag); got != tt.want {
			t.Errorf("ResolveTextSearchConfig(%q) = %q, want %q", tt.tag, got, tt.want)
		}
	}
}

func TestTextSearchConfigsAreIdentifiers(t *testing.T) {
	// Every config name is interpolated straight into SQL, so the map must
	// never contain anything that isn't a bare identifier.
	for tag, config := range textSearchConfigs {
		for _, r := range config {
			if !(r >= 'a' && r <= 'z') && r != '_' {
				t.Errorf("config %q (tag %q) is not a bare lowercase identifier", config, tag)
				break
			}
		}
	}
}

func TestSearchFiltersPlaceholders(t *testing.T) {
	// Search builds its WHERE from buildLinkFiltersWhere and then appends its
	// own args; this guards the placeholder numbering those two share.
	userID := uuid.New()
	since := time.Now()

	where, args := buildLinkFiltersWhere(userID, LinkFilters{
		IsRead:   boolPtr(false),
		FeedID:   int64Ptr(7),
		Archived: boolPtr(false),
	})

	args = append(args, since)
	where = append(where, "l.saved_at >= $4")

	if len(args) != 4 {
		t.Fatalf("args = %d, want 4", len(args))
	}
	joined := strings.Join(where, " AND ")
	for _, want := range []string{"$1", "$2", "$3", "$4"} {
		if !strings.Contains(joined, want) {
			t.Errorf("where %q missing placeholder %s", joined, want)
		}
	}
}

func TestHeadlineOptionsMarkers(t *testing.T) {
	// The frontend splits snippets on these markers to render highlights
	// without injecting HTML, so they must stay in sync with the options
	// string and must not contain the option separator.
	if !strings.Contains(headlineOptions, "StartSel="+HighlightStart) {
		t.Errorf("headlineOptions must use HighlightStart, got %q", headlineOptions)
	}
	if !strings.Contains(headlineOptions, "StopSel="+HighlightEnd) {
		t.Errorf("headlineOptions must use HighlightEnd, got %q", headlineOptions)
	}
	for _, marker := range []string{HighlightStart, HighlightEnd} {
		if strings.ContainsAny(marker, ",'") {
			t.Errorf("marker %q must not contain a comma or quote: it would break ts_headline option parsing", marker)
		}
	}
}
