package data

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/lib/pq"
)

// Feed is one RSS/Atom source, shared by every user subscribed to it — feeds
// are keyed by URL and stored once, with subscriptions carrying the per-user
// half of the relationship.
//
// The unexported-from-JSON fields are sync bookkeeping the API never returns:
// HttpEtag and HttpLastModified drive conditional requests, FetchingSince is
// the scheduler's claim on the feed (see SyncLockWindow), and
// ConsecutiveFailures tracks how long it has been failing.
type Feed struct {
	ID            int64     `json:"id"`
	Url           string    `json:"url"`
	Title         string    `json:"title"`
	SiteUrl       string    `json:"site_url"`
	ImageUrl      string    `json:"image_url,omitempty"`
	IsActive      bool      `json:"is_active"`
	LastFetchedAt time.Time `json:"last_fetched_at"`
	CreatedAt     time.Time `json:"created_at"`

	HttpEtag            string     `json:"-"`
	HttpLastModified    string     `json:"-"`
	FetchingSince       *time.Time `json:"-"`
	ConsecutiveFailures int        `json:"-"`
}

// FeedModel gives access to the feeds table.
type FeedModel struct {
	DB *sql.DB
}

// Get returns the feed with the given ID. Feeds are not user-scoped, so no
// ownership check applies. Returns sql.ErrNoRows if no feed has that ID —
// unlike GetByURL, which maps it to ErrRecordNotFound.
func (m FeedModel) Get(ctx context.Context, id int64) (*Feed, error) {
	var feed Feed

	query := `
		SELECT id, url, original_title, site_url, is_active, created_at, image_url, last_fetched_at
		FROM feeds
		WHERE id = $1`

	err := m.DB.QueryRowContext(ctx, query, id).Scan(
		&feed.ID,
		&feed.Url,
		&feed.Title,
		&feed.SiteUrl,
		&feed.IsActive,
		&feed.CreatedAt,
		&feed.ImageUrl,
		&feed.LastFetchedAt,
	)

	if err != nil {
		return nil, err
	}

	return &feed, nil
}

// Insert stores a feed and fills in the generated columns on feed, including
// its ID and conditional-request headers.
//
// A feed already known to the instance is not an error: the ON CONFLICT clause
// makes the URL's existing row win and returns it, so two users subscribing to
// the same feed at once both end up pointing at one row. The no-op SET is there
// because DO NOTHING would return no row at all, leaving the caller without an
// ID.
func (m FeedModel) Insert(ctx context.Context, feed *Feed) error {
	query := `
		INSERT INTO feeds (url, original_title, site_url, image_url, last_fetched_at, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (url) DO UPDATE SET url = feeds.url
		RETURNING id, COALESCE(original_title, ''), COALESCE(site_url, ''), COALESCE(image_url, ''),
		          COALESCE(last_fetched_at, $5), COALESCE(is_active, $6), COALESCE(created_at, $7),
		          http_etag, http_last_modified`

	err := m.DB.QueryRowContext(ctx,
		query,
		feed.Url,
		feed.Title,
		feed.SiteUrl,
		feed.ImageUrl,
		feed.LastFetchedAt,
		feed.IsActive,
		feed.CreatedAt,
	).Scan(
		&feed.ID,
		&feed.Title,
		&feed.SiteUrl,
		&feed.ImageUrl,
		&feed.LastFetchedAt,
		&feed.IsActive,
		&feed.CreatedAt,
		&feed.HttpEtag,
		&feed.HttpLastModified,
	)

	if err != nil {
		return err
	}

	return nil
}

// MarkDueForSync makes feeds eligible for the scheduler's next pass and clears
// their conditional-request headers.
//
// Both halves matter when someone subscribes to a feed the instance already
// knows. ImportArticlesForSubscribers returns on a 304 before creating any
// links, so keeping the ETag would leave the new subscriber with an empty
// library until the feed happened to publish something. Clearing the headers
// forces a 200 and a full re-read.
//
// last_fetched_at is set to the epoch rather than NULL because Get and GetByURL
// scan that column into a plain time.Time.
func (m FeedModel) MarkDueForSync(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}

	query := `
		UPDATE feeds
		SET last_fetched_at = to_timestamp(0),
		    fetching_since = NULL,
		    http_etag = '',
		    http_last_modified = ''
		WHERE id = ANY($1)`

	_, err := m.DB.ExecContext(ctx, query, pq.Array(ids))
	return err
}

// DeleteIfOrphan removes a feed that no one is subscribed to. It exists to undo
// a feed created while adding a subscription that then failed to insert:
// creating the feed requires an HTTP fetch, so the two writes cannot share a
// transaction and the feed row would otherwise be left behind for good.
//
// The NOT EXISTS guard is what makes it safe to call unconditionally — a
// concurrent subscriber to the same URL keeps the feed.
func (m FeedModel) DeleteIfOrphan(ctx context.Context, id int64) error {
	query := `
		DELETE FROM feeds
		WHERE id = $1
		  AND NOT EXISTS (SELECT 1 FROM subscriptions s WHERE s.feed_id = feeds.id)`

	_, err := m.DB.ExecContext(ctx, query, id)
	return err
}

// GetByURL returns the feed stored under url, which is how the subscribe path
// discovers that the instance already knows a feed and avoids re-fetching it.
// The URL is matched exactly, so it must be the canonical one. Returns
// ErrRecordNotFound if no feed has that URL.
func (m FeedModel) GetByURL(ctx context.Context, url string) (*Feed, error) {
	var feed Feed

	query := `
		SELECT id, url, original_title, site_url, is_active, created_at, image_url, last_fetched_at
		FROM feeds
		WHERE url = $1`

	err := m.DB.QueryRowContext(ctx, query, url).Scan(
		&feed.ID,
		&feed.Url,
		&feed.Title,
		&feed.SiteUrl,
		&feed.IsActive,
		&feed.CreatedAt,
		&feed.ImageUrl,
		&feed.LastFetchedAt,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &feed, nil
}

// SyncLockWindow is how long a claimed feed (fetching_since) stays reserved
// before another worker may consider the lock stale and reclaim it. It must
// stay above service.feedProcessTimeout so a slow-but-alive sync is never
// picked up twice.
const SyncLockWindow = 10 * time.Minute

// syncIntervalSlack shrinks the refresh interval when testing eligibility.
// last_fetched_at is stamped when a sync *finishes*, i.e. slightly after the
// tick that started it, so an exact comparison would always push a feed to the
// tick after the next one and halve the effective refresh rate.
const syncIntervalSlack = 0.9

// GetFeedsToSync atomically claims a batch of feeds ready for synchronization.
// A feed is due once refreshInterval has elapsed since its last successful
// fetch. Uses FOR UPDATE SKIP LOCKED to prevent thundering herd.
func (m FeedModel) GetFeedsToSync(ctx context.Context, batchSize int, refreshInterval time.Duration) ([]*Feed, error) {
	query := `
		UPDATE feeds
		SET fetching_since = NOW()
		WHERE id IN (
			SELECT id FROM feeds f
			WHERE f.is_active = TRUE
			  AND (f.fetching_since IS NULL OR f.fetching_since < NOW() - make_interval(secs => $2::double precision))
			  AND EXISTS (SELECT 1 FROM subscriptions s WHERE s.feed_id = f.id)
			  AND (f.last_fetched_at IS NULL OR f.last_fetched_at < NOW() - make_interval(secs => $3::double precision))
			ORDER BY f.last_fetched_at ASC NULLS FIRST
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, url, http_etag, http_last_modified`

	rows, err := m.DB.QueryContext(ctx, query,
		batchSize,
		SyncLockWindow.Seconds(),
		refreshInterval.Seconds()*syncIntervalSlack,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var feeds []*Feed
	for rows.Next() {
		var f Feed
		err := rows.Scan(&f.ID, &f.Url, &f.HttpEtag, &f.HttpLastModified)
		if err != nil {
			return nil, err
		}
		feeds = append(feeds, &f)
	}

	return feeds, rows.Err()
}

// ReleaseFeed marks a feed as successfully synced, updating HTTP caching headers.
func (m FeedModel) ReleaseFeed(ctx context.Context, feedID int64, etag, lastModified string) error {
	query := `
		UPDATE feeds
		SET fetching_since = NULL,
		    last_fetched_at = NOW(),
		    consecutive_failures = 0,
		    http_etag = $2,
		    http_last_modified = $3
		WHERE id = $1`

	_, err := m.DB.ExecContext(ctx, query, feedID, etag, lastModified)
	return err
}

// MarkFeedFailed increments the failure counter and deactivates the feed after 10 consecutive failures.
func (m FeedModel) MarkFeedFailed(ctx context.Context, feedID int64) (int, error) {
	query := `
		UPDATE feeds
		SET fetching_since = NULL,
		    consecutive_failures = consecutive_failures + 1,
		    is_active = CASE WHEN consecutive_failures + 1 >= 10 THEN FALSE ELSE is_active END
		WHERE id = $1
		RETURNING consecutive_failures`

	var failures int
	err := m.DB.QueryRowContext(ctx, query, feedID).Scan(&failures)
	return failures, err
}
