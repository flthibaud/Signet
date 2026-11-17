package data

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

// TestBulkInsertForArticle covers the array-parameter form of the insert: that
// the uuid[] cast round-trips, that every subscriber gets a row, and that a
// user who already holds the article is skipped rather than erroring.
func TestBulkInsertForArticle(t *testing.T) {
	db := testDB(t)
	model := LinkModel{DB: db}
	ctx := context.Background()

	feedID := seedFeed(t, db, "Bulk insert feed", "")
	articleID := seedArticle(t, db, articleFixture{title: "Bulk insert article"})

	first := seedUser(t, db)
	second := seedUser(t, db)

	// The second user already has this article, so the insert must skip them
	// without failing the batch.
	seedLink(t, db, second, articleID, linkFixture{feedID: &feedID})

	err := model.BulkInsertForArticle(ctx, []uuid.UUID{first, second}, articleID, feedID, "bulk-insert-slug")
	if err != nil {
		t.Fatalf("BulkInsertForArticle: %v", err)
	}

	for _, userID := range []uuid.UUID{first, second} {
		var exists bool
		err := db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM links WHERE user_id = $1 AND article_id = $2)`,
			userID, articleID).Scan(&exists)
		if err != nil {
			t.Fatalf("link lookup for %s: %v", userID, err)
		}
		if !exists {
			t.Errorf("user %s has no link for the article", userID)
		}
	}

	// The conflicting user keeps the link they already had rather than gaining
	// a second one.
	var count int
	err = db.QueryRowContext(ctx,
		`SELECT count(*) FROM links WHERE user_id = $1 AND article_id = $2`,
		second, articleID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("counting links: %v", err)
	}
	if count != 1 {
		t.Errorf("existing subscriber has %d links, want 1", count)
	}
}

func TestBulkInsertForArticleNoUsers(t *testing.T) {
	db := testDB(t)
	model := LinkModel{DB: db}

	if err := model.BulkInsertForArticle(context.Background(), nil, 1, 1, "slug"); err != nil {
		t.Errorf("BulkInsertForArticle with no users: %v", err)
	}
}

// Two articles whose hashes share their first 12 characters get the same slug.
// The user who already holds the first one must not make the insert fail for
// every other subscriber.
func TestBulkInsertForArticleSlugCollision(t *testing.T) {
	db := testDB(t)
	model := LinkModel{DB: db}
	ctx := context.Background()

	feedID := seedFeed(t, db, "Slug collision feed", "")
	held := seedArticle(t, db, articleFixture{title: "Already held"})
	incoming := seedArticle(t, db, articleFixture{title: "Same slug"})

	colliding := seedUser(t, db)
	other := seedUser(t, db)
	slug := seedLink(t, db, colliding, held, linkFixture{feedID: &feedID})

	if err := model.BulkInsertForArticle(ctx, []uuid.UUID{colliding, other}, incoming, feedID, slug); err != nil {
		t.Fatalf("BulkInsertForArticle: %v", err)
	}

	for _, tc := range []struct {
		user uuid.UUID
		want bool
	}{{other, true}, {colliding, false}} {
		var exists bool
		err := db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM links WHERE user_id = $1 AND article_id = $2)`,
			tc.user, incoming).Scan(&exists)
		if err != nil {
			t.Fatalf("link lookup for %s: %v", tc.user, err)
		}
		if exists != tc.want {
			t.Errorf("user %s has a link to the new article: %v, want %v", tc.user, exists, tc.want)
		}
	}
}

// A subscriber deleted while the sync ran breaks a foreign key, which the next
// sync will not hit again, so it must not be reported as unstorable.
func TestBulkInsertForArticleMissingUserIsNotUnstorable(t *testing.T) {
	db := testDB(t)
	model := LinkModel{DB: db}

	feedID := seedFeed(t, db, "Deleted subscriber feed", "")
	articleID := seedArticle(t, db, articleFixture{title: "Deleted subscriber article"})

	err := model.BulkInsertForArticle(context.Background(), []uuid.UUID{uuid.New()}, articleID, feedID, "deleted-user-slug")
	if err == nil {
		t.Fatal("BulkInsertForArticle for a missing user succeeded, want a foreign key error")
	}
	if errors.Is(err, ErrUnstorable) {
		t.Errorf("err = %v, want it not to wrap ErrUnstorable", err)
	}
}
