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

func (m SubscriptionModel) Exists(ctx context.Context, userID uuid.UUID, feedID int64) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM subscriptions
			WHERE user_id = $1 AND feed_id = $2
		)`
	var exists bool
	err := m.DB.QueryRowContext(ctx, query, userID, feedID).Scan(&exists)
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

// GetSubscriberIDs returns all user IDs subscribed to a given feed.
func (m SubscriptionModel) GetSubscriberIDs(ctx context.Context, feedID int64) ([]uuid.UUID, error) {
	query := `SELECT user_id FROM subscriptions WHERE feed_id = $1`

	rows, err := m.DB.QueryContext(ctx, query, feedID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var userIDs []uuid.UUID
	for rows.Next() {
		var uid uuid.UUID
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, uid)
	}

	return userIDs, rows.Err()
}

// Delete removes a subscription belonging to the given user. It returns
// ErrRecordNotFound if no matching subscription exists (either it never existed
// or it belongs to another user).
func (m SubscriptionModel) Delete(ctx context.Context, userID uuid.UUID, id int64) error {
	query := `DELETE FROM subscriptions WHERE id = $1 AND user_id = $2`

	result, err := m.DB.ExecContext(ctx, query, id, userID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrRecordNotFound
	}

	return nil
}

func (m SubscriptionModel) Insert(ctx context.Context, subscription *Subscription) error {
	query := `
		INSERT INTO subscriptions (user_id, feed_id, custom_title, custom_icon, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`

	err := m.DB.QueryRowContext(ctx,
		query,
		subscription.UserID,
		subscription.FeedID,
		subscription.CustomTitle,
		subscription.CustomIcon,
		time.Now(),
	).Scan(&subscription.ID, &subscription.CreatedAt)

	if err != nil {
		return err
	}

	return nil
}
