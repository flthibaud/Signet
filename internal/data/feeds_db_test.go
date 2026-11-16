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

// claimFeed stands in for GetFeedsToSync: ReleaseFeed and MarkFeedFailed only
// finish a sync that holds a claim.
func claimFeed(t *testing.T, db *sql.DB, id int64) {
	t.Helper()
	exec(t, db, `UPDATE feeds SET fetching_since = NOW() WHERE id = $1`, id)
}

// A feed switched off after repeated failures must come back: on its own once a
// fetch succeeds, and at once when someone subscribes to it again.
func TestFeedDeactivationIsNotPermanent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	m := FeedModel{DB: db}

	id := seedFeed(t, db, "Test feed", "")
	// Backdate the last attempt, so the stamp below is visible.
	exec(t, db, `UPDATE feeds SET last_fetched_at = NOW() - interval '1 hour' WHERE id = $1`, id)
	before := readFeedState(t, db, id)

	claimFeed(t, db, id)
	if _, err := m.MarkFeedFailed(ctx, id); err != nil {
		t.Fatal(err)
	}
	// Without the stamp, a scheduler pass draining due feeds would claim the
	// feed it just failed straight back.
	if s := readFeedState(t, db, id); !s.lastFetchedAt.After(before.lastFetchedAt) {
		t.Fatal("a failure should stamp last_fetched_at")
	}

	for range 8 {
		claimFeed(t, db, id)
		if _, err := m.MarkFeedFailed(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	s := readFeedState(t, db, id)
	if !s.active {
		t.Fatal("after 9 failures the feed should still be active")
	}

	claimFeed(t, db, id)
	if _, err := m.MarkFeedFailed(ctx, id); err != nil {
		t.Fatal(err)
	}
	if s = readFeedState(t, db, id); s.active {
		t.Fatal("after 10 failures the feed should be deactivated")
	}

	claimFeed(t, db, id)
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

// A subscribe landing while the feed is mid-sync must not be undone by that
// sync finishing: the sync read the subscriber list before the new user, so its
// ETag would make the next pass a 304 and leave them with nothing.
func TestSubscribeMidSyncKeepsFeedDue(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	m := FeedModel{DB: db}

	for _, finish := range []struct {
		name string
		run  func(id int64) error
	}{
		{"release", func(id int64) error { return m.ReleaseFeed(ctx, id, `"new-etag"`, "") }},
		{"failure", func(id int64) error {
			failures, err := m.MarkFeedFailed(ctx, id)
			if err == nil && failures != 0 {
				t.Errorf("failures = %d, want 0 for a feed whose claim was reset", failures)
			}
			return err
		}},
	} {
		t.Run(finish.name, func(t *testing.T) {
			id := seedFeed(t, db, "Test feed", "")
			exec(t, db, `UPDATE feeds SET http_etag = '"old-etag"' WHERE id = $1`, id)
			claimFeed(t, db, id)

			if err := m.MarkDueForSync(ctx, []int64{id}); err != nil {
				t.Fatal(err)
			}
			if err := finish.run(id); err != nil {
				t.Fatal(err)
			}

			var etag string
			var lastFetched time.Time
			var failures int
			err := db.QueryRowContext(ctx,
				`SELECT http_etag, last_fetched_at, consecutive_failures FROM feeds WHERE id = $1`, id,
			).Scan(&etag, &lastFetched, &failures)
			if err != nil {
				t.Fatal(err)
			}
			if etag != "" || !lastFetched.Equal(time.Unix(0, 0)) || failures != 0 {
				t.Errorf("etag=%q last_fetched_at=%v failures=%d, want the reset left as MarkDueForSync wrote it",
					etag, lastFetched, failures)
			}
		})
	}
}
