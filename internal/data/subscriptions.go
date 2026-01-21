package data

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Subscription struct {
	ID          int64     `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	FeedID      int64     `json:"feed_id"`
	CustomTitle string    `json:"custom_title"`
	CustomIcon  string    `json:"custom_icon"`
	CreatedAt   time.Time `json:"created_at"`
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
