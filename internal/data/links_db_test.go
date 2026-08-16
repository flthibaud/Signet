package data

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestListForUserReturnsEveryDeclaredField(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}
	ctx := context.Background()

	userID := seedUser(t, db)
	feedID := seedFeed(t, db, "Test feed", "https://example.test/feed.png")
	published := time.Now().Add(-24 * time.Hour).Truncate(time.Second)

	articleID := seedArticle(t, db, articleFixture{
		url:         "https://example.test/the-original-article",
		title:       "The original article",
		description: "A description",
		author:      "An author",
		imageURL:    "https://example.test/cover.png",
		textContent: "A body long enough to earn a reading time.",
		publishedAt: &published,
	})

	slug := seedLink(t, db, userID, articleID, linkFixture{
		feedID:          &feedID,
		isRead:          true,
		isStarred:       true,
		readingProgress: 0.42,
		anchorIndex:     7,
	})

	links, hasMore, err := m.ListForUser(ctx, userID, LinkFilters{}, 10, 0)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if hasMore {
		t.Error("hasMore = true, want false for a single link")
	}
	if len(links) != 1 {
		t.Fatalf("got %d links, want 1", len(links))
	}

	got := links[0]
	if got.Slug != slug {
		t.Errorf("Slug = %q, want %q", got.Slug, slug)
	}
	if got.ArticleUrl != "https://example.test/the-original-article" {
		t.Errorf("ArticleUrl = %q, want the article's url", got.ArticleUrl)
	}
	if got.ReadingProgressAnchorIndex != 7 {
		t.Errorf("ReadingProgressAnchorIndex = %d, want 7", got.ReadingProgressAnchorIndex)
	}
	if got.ArchivedAt != nil {
		t.Errorf("ArchivedAt = %v, want nil for a link that is not archived", got.ArchivedAt)
	}
	if !got.IsRead || !got.IsStarred {
		t.Errorf("IsRead/IsStarred = %v/%v, want true/true", got.IsRead, got.IsStarred)
	}
	if got.ReadingProgress < 0.41 || got.ReadingProgress > 0.43 {
		t.Errorf("ReadingProgress = %v, want ~0.42", got.ReadingProgress)
	}
	if got.FeedID == nil || *got.FeedID != feedID {
		t.Errorf("FeedID = %v, want %d", got.FeedID, feedID)
	}
	if got.UserID != userID {
		t.Errorf("UserID = %v, want %v", got.UserID, userID)
	}
	if got.Title != "The original article" || got.Author != "An author" || got.Description != "A description" {
		t.Errorf("article metadata mismatch: %+v", got)
	}
	if got.ImageURL != "https://example.test/cover.png" {
		t.Errorf("ImageURL = %q, want the article's own image", got.ImageURL)
	}
	if got.ReadingTime <= 0 {
		t.Errorf("ReadingTime = %v, want the value trigger_calc_reading_time derived", got.ReadingTime)
	}
	if got.FeedTitle == nil || *got.FeedTitle != "Test feed" {
		t.Errorf("FeedTitle = %v, want \"Test feed\"", got.FeedTitle)
	}
	if !got.PublishedAt.Equal(published) {
		t.Errorf("PublishedAt = %v, want %v", got.PublishedAt, published)
	}
}

func TestListForUserReportsArchivedAt(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}

	userID := seedUser(t, db)
	articleID := seedArticle(t, db, articleFixture{})
	archivedAt := time.Now().Add(-time.Hour).Truncate(time.Second)
	seedLink(t, db, userID, articleID, linkFixture{archived: true, savedAt: timePtr(archivedAt)})

	archived := true
	links, _, err := m.ListForUser(context.Background(), userID, LinkFilters{Archived: &archived}, 10, 0)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("got %d links, want 1", len(links))
	}
	if links[0].ArchivedAt == nil {
		t.Fatal("ArchivedAt = nil, want a timestamp for an archived link")
	}
	if !links[0].ArchivedAt.Equal(archivedAt) {
		t.Errorf("ArchivedAt = %v, want %v", links[0].ArchivedAt, archivedAt)
	}
}

func TestListForUserFallsBackToFeedImage(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}

	userID := seedUser(t, db)
	feedID := seedFeed(t, db, "Test feed", "https://example.test/feed.png")
	articleID := seedArticle(t, db, articleFixture{imageURL: ""})
	seedLink(t, db, userID, articleID, linkFixture{feedID: &feedID})

	links, _, err := m.ListForUser(context.Background(), userID, LinkFilters{}, 10, 0)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("got %d links, want 1", len(links))
	}
	if links[0].ImageURL != "https://example.test/feed.png" {
		t.Errorf("ImageURL = %q, want the feed's image", links[0].ImageURL)
	}
}

func TestNullArticleMetadataDoesNotBreakTheQueries(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}
	ctx := context.Background()

	userID := seedUser(t, db)
	// No feed: the image has nothing to fall back to but the empty string.
	slug := seedLink(t, db, userID, seedArticleWithNulls(t, db), linkFixture{})
	// A second, well-formed link, to prove the bad row cannot poison the page.
	goodSlug := seedLink(t, db, userID, seedArticle(t, db, articleFixture{author: "An author"}), linkFixture{})

	links, _, err := m.ListForUser(ctx, userID, LinkFilters{}, 10, 0)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2", len(links))
	}
	for _, l := range links {
		if l.Slug == slug && (l.Description != "" || l.Author != "" || l.ImageURL != "") {
			t.Errorf("NULL metadata should read as empty strings, got %+v", l)
		}
		if l.Slug == goodSlug && l.Author != "An author" {
			t.Errorf("the well-formed row lost its author: %+v", l)
		}
	}

	detail, err := m.GetBySlug(ctx, slug, userID)
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if detail.Author != "" || detail.ImageURL != "" {
		t.Errorf("NULL metadata should read as empty strings, got %+v", detail)
	}

	results, _, err := m.Search(ctx, userID, SearchFilters{}, 10, 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("got %d search results, want 2", len(results))
	}
}

func TestListForUserFilters(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}
	ctx := context.Background()

	userID := seedUser(t, db)
	otherUserID := seedUser(t, db)
	feedID := seedFeed(t, db, "Test feed", "")

	readSlug := seedLink(t, db, userID, seedArticle(t, db, articleFixture{}), linkFixture{isRead: true})
	starredSlug := seedLink(t, db, userID, seedArticle(t, db, articleFixture{}), linkFixture{isStarred: true})
	archivedSlug := seedLink(t, db, userID, seedArticle(t, db, articleFixture{}), linkFixture{archived: true})
	feedSlug := seedLink(t, db, userID, seedArticle(t, db, articleFixture{}), linkFixture{feedID: &feedID})

	// Another user's link to the same article must never leak into the results.
	seedLink(t, db, otherUserID, seedArticle(t, db, articleFixture{}), linkFixture{})

	slugsOf := func(links []*LinkWithArticle) []string {
		out := make([]string, len(links))
		for i, l := range links {
			out[i] = l.Slug
		}
		return out
	}
	contains := func(slugs []string, want string) bool {
		for _, s := range slugs {
			if s == want {
				return true
			}
		}
		return false
	}

	tests := []struct {
		name    string
		filters LinkFilters
		want    []string
		absent  []string
	}{
		{
			name:    "unfiltered spans the user's library only",
			filters: LinkFilters{},
			want:    []string{readSlug, starredSlug, archivedSlug, feedSlug},
		},
		{
			name:    "is_read",
			filters: LinkFilters{IsRead: boolPtr(true)},
			want:    []string{readSlug},
			absent:  []string{starredSlug, feedSlug},
		},
		{
			name:    "is_starred",
			filters: LinkFilters{IsStarred: boolPtr(true)},
			want:    []string{starredSlug},
			absent:  []string{readSlug, feedSlug},
		},
		{
			name:    "archived",
			filters: LinkFilters{Archived: boolPtr(true)},
			want:    []string{archivedSlug},
			absent:  []string{readSlug, starredSlug, feedSlug},
		},
		{
			name:    "not archived",
			filters: LinkFilters{Archived: boolPtr(false)},
			want:    []string{readSlug, starredSlug, feedSlug},
			absent:  []string{archivedSlug},
		},
		{
			name:    "feed_id",
			filters: LinkFilters{FeedID: &feedID},
			want:    []string{feedSlug},
			absent:  []string{readSlug, starredSlug},
		},
		{
			name:    "combined filters narrow rather than widen",
			filters: LinkFilters{IsRead: boolPtr(true), Archived: boolPtr(false), FeedID: &feedID},
			absent:  []string{readSlug, starredSlug, archivedSlug, feedSlug},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links, _, err := m.ListForUser(ctx, userID, tt.filters, 100, 0)
			if err != nil {
				t.Fatalf("ListForUser: %v", err)
			}
			slugs := slugsOf(links)
			for _, want := range tt.want {
				if !contains(slugs, want) {
					t.Errorf("missing %s from %v", want, slugs)
				}
			}
			for _, absent := range tt.absent {
				if contains(slugs, absent) {
					t.Errorf("unexpected %s in %v", absent, slugs)
				}
			}
		})
	}
}

func TestListForUserPagination(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}
	ctx := context.Background()

	userID := seedUser(t, db)
	base := time.Now().Add(-72 * time.Hour).Truncate(time.Second)
	for i := range 3 {
		published := base.Add(time.Duration(i) * time.Hour)
		articleID := seedArticle(t, db, articleFixture{publishedAt: &published})
		seedLink(t, db, userID, articleID, linkFixture{})
	}

	page1, hasMore, err := m.ListForUser(ctx, userID, LinkFilters{}, 2, 0)
	if err != nil {
		t.Fatalf("ListForUser page 1: %v", err)
	}
	if !hasMore {
		t.Error("hasMore = false on page 1, want true with 3 links and a limit of 2")
	}
	if len(page1) != 2 {
		t.Fatalf("page 1 has %d links, want 2 (the lookahead row must be trimmed)", len(page1))
	}

	page2, hasMore, err := m.ListForUser(ctx, userID, LinkFilters{}, 2, 2)
	if err != nil {
		t.Fatalf("ListForUser page 2: %v", err)
	}
	if hasMore {
		t.Error("hasMore = true on the last page, want false")
	}
	if len(page2) != 1 {
		t.Fatalf("page 2 has %d links, want 1", len(page2))
	}

	// Newest first, and the pages must not overlap.
	if !page1[0].PublishedAt.After(page1[1].PublishedAt) {
		t.Error("page 1 is not ordered by published_at descending")
	}
	if !page1[1].PublishedAt.After(page2[0].PublishedAt) {
		t.Error("page 2 does not continue where page 1 stopped")
	}
}

func TestGetBySlugReturnsEveryDeclaredField(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}
	ctx := context.Background()

	userID := seedUser(t, db)
	feedID := seedFeed(t, db, "Test feed", "")
	published := time.Now().Add(-12 * time.Hour).Truncate(time.Second)
	archivedAt := time.Now().Add(-2 * time.Hour).Truncate(time.Second)

	articleID := seedArticle(t, db, articleFixture{
		url:         "https://example.test/detail",
		title:       "Detail article",
		author:      "An author",
		imageURL:    "https://example.test/detail.png",
		textContent: "The body of the article.",
		publishedAt: &published,
	})

	slug := seedLink(t, db, userID, articleID, linkFixture{
		feedID:          &feedID,
		readingProgress: 0.75,
		anchorIndex:     12,
		archived:        true,
		savedAt:         timePtr(archivedAt),
	})

	got, err := m.GetBySlug(ctx, slug, userID)
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}

	if got.ArticleUrl != "https://example.test/detail" {
		t.Errorf("ArticleUrl = %q, want the article's url", got.ArticleUrl)
	}
	if got.ReadingProgressAnchorIndex != 12 {
		t.Errorf("ReadingProgressAnchorIndex = %d, want 12", got.ReadingProgressAnchorIndex)
	}
	if got.ArchivedAt == nil || !got.ArchivedAt.Equal(archivedAt) {
		t.Errorf("ArchivedAt = %v, want %v", got.ArchivedAt, archivedAt)
	}
	if got.TextContent != "The body of the article." {
		t.Errorf("TextContent = %q", got.TextContent)
	}
	if got.Title != "Detail article" || got.Author != "An author" {
		t.Errorf("article metadata mismatch: %+v", got)
	}
	if got.UserID != userID {
		t.Errorf("UserID = %v, want %v", got.UserID, userID)
	}
	if !got.PublishedAt.Equal(published) {
		t.Errorf("PublishedAt = %v, want %v", got.PublishedAt, published)
	}
}

func TestGetBySlugIsScopedToTheUser(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}
	ctx := context.Background()

	owner := seedUser(t, db)
	intruder := seedUser(t, db)
	slug := seedLink(t, db, owner, seedArticle(t, db, articleFixture{}), linkFixture{})

	if _, err := m.GetBySlug(ctx, slug, intruder); err != ErrRecordNotFound {
		t.Errorf("GetBySlug for another user = %v, want ErrRecordNotFound", err)
	}
	if _, err := m.GetBySlug(ctx, "does-not-exist-"+uuid.NewString(), owner); err != ErrRecordNotFound {
		t.Errorf("GetBySlug for an unknown slug = %v, want ErrRecordNotFound", err)
	}
}

func TestUpdateRoundTripsThroughGetBySlug(t *testing.T) {
	db := testDB(t)
	m := LinkModel{DB: db}
	ctx := context.Background()

	userID := seedUser(t, db)
	slug := seedLink(t, db, userID, seedArticle(t, db, articleFixture{}), linkFixture{})

	err := m.Update(ctx, userID, slug, LinkUpdate{
		IsRead:                     boolPtr(true),
		IsStarred:                  boolPtr(true),
		Archived:                   boolPtr(true),
		ReadingProgress:            floatPtr(0.6),
		ReadingProgressAnchorIndex: intPtr(9),
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := m.GetBySlug(ctx, slug, userID)
	if err != nil {
		t.Fatalf("GetBySlug: %v", err)
	}
	if !got.IsRead || !got.IsStarred {
		t.Errorf("IsRead/IsStarred = %v/%v, want true/true", got.IsRead, got.IsStarred)
	}
	if got.ReadingProgressAnchorIndex != 9 {
		t.Errorf("ReadingProgressAnchorIndex = %d, want 9", got.ReadingProgressAnchorIndex)
	}
	if got.ReadingProgress < 0.59 || got.ReadingProgress > 0.61 {
		t.Errorf("ReadingProgress = %v, want ~0.6", got.ReadingProgress)
	}
	if got.ArchivedAt == nil {
		t.Error("ArchivedAt = nil after archiving, want a timestamp")
	}

	// Unarchiving has to clear the column, not stamp it with a zero time.
	if err := m.Update(ctx, userID, slug, LinkUpdate{Archived: boolPtr(false)}); err != nil {
		t.Fatalf("Update (unarchive): %v", err)
	}
	got, err = m.GetBySlug(ctx, slug, userID)
	if err != nil {
		t.Fatalf("GetBySlug after unarchive: %v", err)
	}
	if got.ArchivedAt != nil {
		t.Errorf("ArchivedAt = %v after unarchiving, want nil", got.ArchivedAt)
	}

	if err := m.Update(ctx, userID, "does-not-exist-"+uuid.NewString(), LinkUpdate{IsRead: boolPtr(true)}); err != ErrRecordNotFound {
		t.Errorf("Update on an unknown slug = %v, want ErrRecordNotFound", err)
	}
}
