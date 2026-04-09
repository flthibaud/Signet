package data

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Feed struct {
	ID            int64     `json:"id"`
	Url           string    `json:"url"`
	Title         string    `json:"title"`
	SiteUrl       string    `json:"site_url"`
	ImageUrl      string    `json:"image_url,omitempty"`
	IsActive      bool      `json:"is_active"`
	LastFetchedAt time.Time `json:"last_fetched_at"`
	CreatedAt     time.Time `json:"created_at"`

	HttpEtag             string     `json:"-"`
	HttpLastModified     string     `json:"-"`
	FetchingSince        *time.Time `json:"-"`
	ConsecutiveFailures  int        `json:"-"`
}

type FeedModel struct {
	DB *sql.DB
}

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

func (m FeedModel) Insert(ctx context.Context, feed *Feed) error {
	query := `
		INSERT INTO feeds (url, original_title, site_url, image_url, last_fetched_at, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`

	err := m.DB.QueryRowContext(ctx,
		query,
		feed.Url,
		feed.Title,
		feed.SiteUrl,
		feed.ImageUrl,
		feed.LastFetchedAt,
		feed.IsActive,
		feed.CreatedAt,
	).Scan(&feed.ID)

	if err != nil {
		return err
	}

	return nil
}

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

func (m FeedModel) UpdateLastFetched(ctx context.Context, feedID int64) error {
	query := `
		UPDATE feeds
		SET last_fetched_at = $1
		WHERE id = $2`

	_, err := m.DB.ExecContext(ctx, query, time.Now(), feedID)
	if err != nil {
		return err
	}

	return nil
}

// GetFeedsToSync atomically claims a batch of feeds ready for synchronization.
// Uses FOR UPDATE SKIP LOCKED to prevent thundering herd.
func (m FeedModel) GetFeedsToSync(ctx context.Context, batchSize int) ([]*Feed, error) {
	query := `
		UPDATE feeds
		SET fetching_since = NOW()
		WHERE id IN (
			SELECT id FROM feeds f
			WHERE f.is_active = TRUE
			  AND (f.fetching_since IS NULL OR f.fetching_since < NOW() - INTERVAL '10 minutes')
			  AND EXISTS (SELECT 1 FROM subscriptions s WHERE s.feed_id = f.id)
			  AND (f.last_fetched_at IS NULL OR f.last_fetched_at < NOW() - INTERVAL '15 minutes')
			ORDER BY f.last_fetched_at ASC NULLS FIRST
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, url, http_etag, http_last_modified`

	rows, err := m.DB.QueryContext(ctx, query, batchSize)
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
