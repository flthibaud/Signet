package data

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Subscription struct {
	ID          int64     `json:"id"`
	UserID      uuid.UUID `json:"-"`
	FeedID      int64     `json:"-"`
	CustomTitle *string   `json:"custom_title"`
	CustomIcon  *string   `json:"custom_icon"`
	// Category    *string   `json:"category"`
	CreatedAt time.Time `json:"created_at"`

	// Embedded
	Feed        Feed `json:"feed"`
	UnreadCount int  `json:"unread_count"`
}

type SubscriptionModel struct {
	DB *sql.DB
}

type SubscriptionDisplay struct {
	ID          int64  `json:"id"`
	FeedID      int64  `json:"feed_id"`
	DisplayName string `json:"display_name"`
	FeedUrl     string `json:"feed_url"`
}

func (m SubscriptionModel) Exists(ctx context.Context, userID uuid.UUID, feedID int64) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM subscriptions
			WHERE user_id = $1 AND feed_id = $2
		)`
	var exists bool
	err := m.DB.QueryRow(query, userID, feedID).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

func (m SubscriptionModel) GetAllForUser(ctx context.Context, userID uuid.UUID) ([]*Subscription, error) {
	query := `
		SELECT 
			-- Subscription
			s.id,
			s.user_id,
			s.feed_id,
			s.custom_title,
			s.custom_icon,
			s.created_at,
			
			-- Feed
			f.id,
			f.url,
			f.original_title,
			f.site_url,
			f.image_url,
			f.last_fetched_at,
			f.is_active,
			
			-- Unread count
			COUNT(l.id) FILTER (WHERE l.is_read = FALSE) AS unread_count
			
		FROM subscriptions s
		INNER JOIN feeds f ON s.feed_id = f.id
		LEFT JOIN links l ON l.feed_id = f.id AND l.user_id = s.user_id
		WHERE s.user_id = $1
		GROUP BY s.id, f.id
		ORDER BY s.created_at DESC`

	rows, err := m.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	subscriptions := []*Subscription{}

	for rows.Next() {
		var s Subscription
		var f Feed

		err := rows.Scan(
			// Subscription
			&s.ID,
			&s.UserID,
			&s.FeedID,
			&s.CustomTitle,
			&s.CustomIcon,
			&s.CreatedAt,

			// Feed
			&f.ID,
			&f.Url,
			&f.Title,
			&f.SiteUrl,
			&f.ImageUrl,
			&f.LastFetchedAt,
			&f.IsActive,

			// Unread count
			&s.UnreadCount,
		)
		if err != nil {
			return nil, err
		}

		s.Feed = f
		subscriptions = append(subscriptions, &s)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return subscriptions, nil
}

func (m SubscriptionModel) Insert(ctx context.Context, subscription *Subscription) error {
	query := `
		INSERT INTO subscriptions (user_id, feed_id, custom_title, custom_icon, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`

	err := m.DB.QueryRowContext(ctx,
		query,
		subscription.UserID,
		subscription.FeedID,
		subscription.CustomTitle,
		subscription.CustomIcon,
		time.Now(),
	).Scan(&subscription.ID)

	if err != nil {
		return err
	}

	return nil
}
