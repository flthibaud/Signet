package data

import (
	"context"
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
		exists, err := model.Exists(ctx, userID, articleID)
		if err != nil {
			t.Fatalf("Exists(%s): %v", userID, err)
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
