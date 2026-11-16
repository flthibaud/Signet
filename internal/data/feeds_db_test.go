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
	if _, err := m.MarkFeedFailed(ctx, id, 10); err != nil {
		t.Fatal(err)
	}
	// Without the stamp, a scheduler pass draining due feeds would claim the
	// feed it just failed straight back.
	if s := readFeedState(t, db, id); !s.lastFetchedAt.After(before.lastFetchedAt) {
		t.Fatal("a failure should stamp last_fetched_at")
	}

	for range 8 {
		claimFeed(t, db, id)
		if _, err := m.MarkFeedFailed(ctx, id, 10); err != nil {
			t.Fatal(err)
		}
	}
	s := readFeedState(t, db, id)
	if !s.active {
		t.Fatal("after 9 failures the feed should still be active")
	}

	claimFeed(t, db, id)
	if _, err := m.MarkFeedFailed(ctx, id, 10); err != nil {
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
			failures, err := m.MarkFeedFailed(ctx, id, 10)
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

func TestMarkFeedFailedUsesTheGivenThreshold(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	m := FeedModel{DB: db}

	id := seedFeed(t, db, "Test feed", "")
	for i := range 2 {
		claimFeed(t, db, id)
		failures, err := m.MarkFeedFailed(ctx, id, 2)
		if err != nil {
			t.Fatal(err)
		}
		if failures != i+1 {
			t.Fatalf("failures = %d, want %d", failures, i+1)
		}
	}
	if s := readFeedState(t, db, id); s.active {
		t.Error("the feed should be deactivated once it reaches the threshold it was given")
	}
}

// A sync the shutdown cut short gives its claim back and nothing else: the feed
// did nothing wrong, and must be claimable again at once.
func TestUnclaimOnlyReleasesTheClaim(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	m := FeedModel{DB: db}

	id := seedFeed(t, db, "Test feed", "")
	exec(t, db, `UPDATE feeds SET consecutive_failures = 3, last_fetched_at = to_timestamp(0) WHERE id = $1`, id)
	claimFeed(t, db, id)

	if err := m.Unclaim(ctx, id); err != nil {
		t.Fatal(err)
	}

	var claimed bool
	if err := db.QueryRowContext(ctx, `SELECT fetching_since IS NOT NULL FROM feeds WHERE id = $1`, id).Scan(&claimed); err != nil {
		t.Fatal(err)
	}
	s := readFeedState(t, db, id)
	if claimed || s.failures != 3 || !s.lastFetchedAt.Equal(time.Unix(0, 0)) {
		t.Errorf("claimed=%v failures=%d last_fetched_at=%v, want only the claim cleared", claimed, s.failures, s.lastFetchedAt)
	}
}

// The windows are a century long and the fixtures backdated before 1970, so the
// claim cannot touch rows this test does not own, even on a development
// database: the oldest real timestamp is MarkDueForSync's epoch.
func TestGetFeedsToSyncClaimsOnlyDueFeeds(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	m := FeedModel{DB: db}

	const year = 365 * 24 * time.Hour
	dueAfter, lockWindow, deadRetry := 100*year, 10*time.Minute, 150*year

	userID := seedUser(t, db)
	feed := func(lastFetched, fetchingSince string, active, subscribed bool) int64 {
		id := seedFeed(t, db, "Test feed", "")
		exec(t, db, `UPDATE feeds SET last_fetched_at = `+lastFetched+`, fetching_since = `+fetchingSince+`,
			is_active = $2, http_etag = 'etag' WHERE id = $1`, id, active)
		if subscribed {
			seedSubscription(t, db, userID, id, nil)
		}
		return id
	}

	const old, older = "'1900-01-01'", "'1800-01-01'"
	due := feed(old, "NULL", true, true)
	feed("to_timestamp(0)", "NULL", true, true) // fetched within the window
	feed(old, "NOW()", true, true)              // claimed by a live sync
	staleClaim := feed(old, "NOW() - interval '1 hour'", true, true)
	feed(old, "NULL", false, true)              // deactivated, retried too recently
	deadOld := feed(older, "NULL", false, true) // deactivated, due a retry
	feed(old, "NULL", true, false)              // nobody subscribed

	claimed, err := m.GetFeedsToSync(ctx, 1000, dueAfter, lockWindow, deadRetry)
	if err != nil {
		t.Fatal(err)
	}

	got := map[int64]bool{}
	for _, f := range claimed {
		got[f.ID] = true
		if f.ID == due && f.HttpEtag != "etag" {
			t.Errorf("claimed feed etag = %q, want the stored one for the conditional fetch", f.HttpEtag)
		}
	}
	want := map[int64]bool{due: true, staleClaim: true, deadOld: true}
	for id := range want {
		if !got[id] {
			t.Errorf("feed %d was not claimed", id)
		}
	}
	if len(got) != len(want) {
		t.Errorf("claimed %v, want exactly %v", got, want)
	}
}
