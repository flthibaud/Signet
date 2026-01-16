package data

import (
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
	CategoryID  *int64    `json:"category_id,omitempty"`
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

func (m SubscriptionModel) GetAllForUser(userID uuid.UUID) ([]*SubscriptionDisplay, error) {
	query := `
		SELECT 
				s.id, 
				s.feed_id, 
				COALESCE(s.custom_title, f.original_title, f.url) as display_name,
				f.url
		FROM subscriptions s
		JOIN feeds f ON s.feed_id = f.id
		WHERE s.user_id = $1
		ORDER BY display_name ASC`

	// ... (Scan des lignes et retour du slice) ...
	rows, err := m.DB.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []*SubscriptionDisplay

	for rows.Next() {
		var sub SubscriptionDisplay
		err := rows.Scan(&sub.ID, &sub.FeedID, &sub.DisplayName, &sub.FeedUrl)
		if err != nil {
			return nil, err
		}
		subs = append(subs, &sub)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return subs, nil
}
