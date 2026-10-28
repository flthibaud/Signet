package data

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

type feedState struct {
	active        bool
	failures      int
	lastFetchedAt time.Time
}

func readFeedState(t *testing.T, db *sql.DB, id int64) feedState {
	t.Helper()

	var s feedState
	err := db.QueryRowContext(context.Background(),
		`SELECT is_active, consecutive_failures, last_fetched_at FROM feeds WHERE id = $1`, id,
	).Scan(&s.active, &s.failures, &s.lastFetchedAt)
	if err != nil {
		t.Fatalf("read feed state: %v", err)
	}
	return s
}

// A feed switched off after repeated failures must come back: on its own once a
// fetch succeeds, and at once when someone subscribes to it again.
func TestFeedDeactivationIsNotPermanent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	m := FeedModel{DB: db}

	id := seedFeed(t, db, "Test feed", "")
	// Backdate the last success, so the stamp below is visible.
	exec(t, db, `UPDATE feeds SET last_fetched_at = NOW() - interval '1 hour' WHERE id = $1`, id)
	before := readFeedState(t, db, id)

	for range 9 {
		if _, err := m.MarkFeedFailed(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	s := readFeedState(t, db, id)
	if !s.active || !s.lastFetchedAt.Equal(before.lastFetchedAt) {
		t.Fatalf("after 9 failures: active=%v last_fetched_at moved=%v, want still active and unchanged",
			s.active, !s.lastFetchedAt.Equal(before.lastFetchedAt))
	}

	if _, err := m.MarkFeedFailed(ctx, id); err != nil {
		t.Fatal(err)
	}
	s = readFeedState(t, db, id)
	if s.active {
		t.Fatal("after 10 failures the feed should be deactivated")
	}
	if !s.lastFetchedAt.After(before.lastFetchedAt) {
		t.Fatal("deactivation should stamp last_fetched_at, which spaces the daily retries")
	}

	if err := m.ReleaseFeed(ctx, id, "", ""); err != nil {
		t.Fatal(err)
	}
	if s = readFeedState(t, db, id); !s.active || s.failures != 0 {
		t.Fatalf("after a success: active=%v failures=%d, want active with 0", s.active, s.failures)
	}

	exec(t, db, `UPDATE feeds SET is_active = FALSE, consecutive_failures = 12 WHERE id = $1`, id)
	if err := m.MarkDueForSync(ctx, []int64{id}); err != nil {
		t.Fatal(err)
	}
	if s = readFeedState(t, db, id); !s.active || s.failures != 0 {
		t.Fatalf("after MarkDueForSync: active=%v failures=%d, want active with 0", s.active, s.failures)
	}
}
