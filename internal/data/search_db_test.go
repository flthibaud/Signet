package data

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSearchReportsArchivedAtLikeListForUser(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}
	ctx := context.Background()

	userID := seedUser(t, db)
	archivedAt := time.Now().Add(-3 * time.Hour).Truncate(time.Second)

	liveArticle := seedArticle(t, db, articleFixture{title: "Kestrel plumage study"})
	archivedArticle := seedArticle(t, db, articleFixture{title: "Kestrel nesting study"})

	liveSlug := seedLink(t, db, userID, liveArticle, linkFixture{})
	archivedSlug := seedLink(t, db, userID, archivedArticle, linkFixture{
		archived: true,
		savedAt:  timePtr(archivedAt),
	})

	// Both orderings build their own SELECT list, so both need checking.
	for _, query := range []string{"", "kestrel"} {
		name := "relevance ordering"
		if query == "" {
			name = "recency ordering"
		}

		t.Run(name, func(t *testing.T) {
			results, _, err := m.Search(ctx, userID, SearchFilters{Query: query}, 100, 0)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}

			var sawLive, sawArchived bool
			for _, r := range results {
				switch r.Slug {
				case liveSlug:
					sawLive = true
					if r.ArchivedAt != nil {
						t.Errorf("ArchivedAt = %v for a live link, want nil", r.ArchivedAt)
					}
				case archivedSlug:
					sawArchived = true
					if r.ArchivedAt == nil {
						t.Fatal("ArchivedAt = nil for an archived link, want a timestamp")
					}
					if !r.ArchivedAt.Equal(archivedAt) {
						t.Errorf("ArchivedAt = %v, want %v", r.ArchivedAt, archivedAt)
					}
				}
			}
			if !sawLive || !sawArchived {
				t.Errorf("expected both links in the results, saw live=%v archived=%v", sawLive, sawArchived)
			}
		})
	}
}

func TestSearchRanksAndHighlights(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}
	ctx := context.Background()

	userID := seedUser(t, db)
	articleID := seedArticle(t, db, articleFixture{
		title:       "A study of peregrines",
		description: "Peregrine falcons hunt at remarkable speed.",
		textContent: "The peregrine falcon is the fastest animal on the planet.",
	})
	slug := seedLink(t, db, userID, articleID, linkFixture{})

	results, hasMore, err := m.Search(ctx, userID, SearchFilters{Query: "peregrine"}, 10, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if hasMore {
		t.Error("hasMore = true, want false")
	}

	var got *SearchResult
	for _, r := range results {
		if r.Slug == slug {
			got = r
		}
	}
	if got == nil {
		t.Fatalf("the seeded link is missing from %d results", len(results))
	}

	if got.Rank <= 0 {
		t.Errorf("Rank = %v, want a positive score for a matching document", got.Rank)
	}
	if !strings.Contains(got.Snippet, HighlightStart) || !strings.Contains(got.Snippet, HighlightEnd) {
		t.Errorf("Snippet = %q, want it to carry the highlight markers", got.Snippet)
	}
	if got.Title != "A study of peregrines" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.ReadingTime <= 0 {
		t.Errorf("ReadingTime = %v, want the value trigger_calc_reading_time derived", got.ReadingTime)
	}
}

func TestSearchMatchesAPrefix(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}

	userID := seedUser(t, db)
	articleID := seedArticle(t, db, articleFixture{title: "Postgres full text search"})
	slug := seedLink(t, db, userID, articleID, linkFixture{})

	results, _, err := m.Search(context.Background(), userID, SearchFilters{Query: "postg"}, 10, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, r := range results {
		if r.Slug == slug {
			return
		}
	}
	t.Error("a prefix of the title did not match the article")
}

func TestSearchFiltersAndSince(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}
	ctx := context.Background()

	userID := seedUser(t, db)
	old := time.Now().Add(-30 * 24 * time.Hour).Truncate(time.Second)
	recent := time.Now().Add(-time.Hour).Truncate(time.Second)

	oldSlug := seedLink(t, db, userID,
		seedArticle(t, db, articleFixture{title: "Cormorant survey", publishedAt: &old}),
		linkFixture{})
	recentSlug := seedLink(t, db, userID,
		seedArticle(t, db, articleFixture{title: "Cormorant census", publishedAt: &recent}),
		linkFixture{isStarred: true})

	has := func(results []*SearchResult, slug string) bool {
		for _, r := range results {
			if r.Slug == slug {
				return true
			}
		}
		return false
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	results, _, err := m.Search(ctx, userID, SearchFilters{Query: "cormorant", Since: &cutoff}, 10, 0)
	if err != nil {
		t.Fatalf("Search with Since: %v", err)
	}
	if has(results, oldSlug) {
		t.Error("a link published before the cutoff was returned")
	}
	if !has(results, recentSlug) {
		t.Error("a link published after the cutoff was missing")
	}

	results, _, err = m.Search(ctx, userID, SearchFilters{
		Query:       "cormorant",
		LinkFilters: LinkFilters{IsStarred: boolPtr(true)},
	}, 10, 0)
	if err != nil {
		t.Fatalf("Search with a filter: %v", err)
	}
	if has(results, oldSlug) {
		t.Error("an unstarred link was returned under is_starred=true")
	}
	if !has(results, recentSlug) {
		t.Error("the starred link was missing under is_starred=true")
	}
}

func TestSearchWithOnlyStopwordsMatchesNothing(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}

	userID := seedUser(t, db)
	seedLink(t, db, userID, seedArticle(t, db, articleFixture{title: "Anything at all"}), linkFixture{})

	results, hasMore, err := m.Search(context.Background(), userID,
		SearchFilters{Query: "the", Language: "english_ua"}, 10, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("got %d results for a stopword-only query, want 0", len(results))
	}
	if hasMore {
		t.Error("hasMore = true, want false")
	}
}
