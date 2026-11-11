package data

import (
	"context"
	"errors"
	"testing"
)

// TestSubscriptionInsertDuplicate pins the race-safe "already subscribed"
// answer: the second insert for the same (user, feed) must surface as
// ErrDuplicateSubscription, not as a raw unique-violation error that a handler
// would turn into a 500.
func TestSubscriptionInsertDuplicate(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	model := SubscriptionModel{DB: db}
	userID := seedUser(t, db)
	other := seedUser(t, db)
	feedID := seedFeed(t, db, "Test feed", "")

	sub := &Subscription{UserID: userID, FeedID: feedID}
	if err := model.Insert(ctx, sub); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if sub.ID == 0 || sub.CreatedAt.IsZero() {
		t.Errorf("Insert did not fill in the generated columns: %+v", sub)
	}

	dup := &Subscription{UserID: userID, FeedID: feedID}
	if err := model.Insert(ctx, dup); !errors.Is(err, ErrDuplicateSubscription) {
		t.Errorf("inserting a duplicate: got %v, want ErrDuplicateSubscription", err)
	}

	// Uniqueness is per user, not per feed.
	sameFeed := &Subscription{UserID: other, FeedID: feedID}
	if err := model.Insert(ctx, sameFeed); err != nil {
		t.Errorf("another user could not subscribe to the same feed: %v", err)
	}
}
