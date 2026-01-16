package data

import (
	"database/sql"
	"time"

	"github.com/google/uuid"
)

type Link struct {
	ID                                int64     `json:"id"`
	UserID                            uuid.UUID `json:"user_id"`
	ArticleID                         int64     `json:"article_id"`
	Slug                              string    `json:"slug"`
	ArticleHash                       string    `json:"article_hash"`
	ArticleUrl                        string    `json:"article_url"`
	IsRead                            bool      `json:"is_read"`
	IsFavorite                        bool      `json:"is_favorite"`
	ArticleReadingProgressAnchorIndex int       `json:"article_reading_progress_anchor_index"`
	FeedID                            *int64    `json:"feed_id"`
	SavedAt                           time.Time `json:"saved_at"`
	ArchivedAt                        time.Time `json:"archived_at"`
	UpdatedAt                         time.Time `json:"updated_at"`
}

type LinkModel struct {
	DB *sql.DB
}

type ImportStats struct {
	TotalFound int64 `json:"total_found"`
	Inserted   int64 `json:"new_inserted"`
	Skipped    int64 `json:"duplicates_skipped"`
}

func (m LinkModel) Insert(userID uuid.UUID, articleID int64, feedID int64, slug string, publishedAt time.Time) error {
	query := `
		INSERT INTO links (user_id, article_id, feed_id, slug, published_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, article_id) DO NOTHING`

	_, err := m.DB.Exec(query, userID, articleID, feedID, slug, publishedAt)
	return err
}

func (m LinkModel) Get(id int64) (*Link, error) {
	return nil, nil
}

func (m LinkModel) Update(link *Link) error {
	return nil
}

func (m LinkModel) Delete(id int64) error {
	return nil
}
