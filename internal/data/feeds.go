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
