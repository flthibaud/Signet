package data

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Link struct {
	ID                                int64     `json:"id"`
	UserID                            uuid.UUID `json:"-"`
	ArticleID                         int64     `json:"-"`
	Slug                              string    `json:"slug"`
	ArticleHash                       string    `json:"-"`
	ArticleUrl                        string    `json:"article_url"`
	IsRead                            bool      `json:"is_read"`
	IsStarred                         bool      `json:"is_starred"`
	ArticleReadingProgressAnchorIndex int       `json:"article_reading_progress_anchor_index"`
	FeedID                            *int64    `json:"feed_id"`
	CreatedAt                         time.Time `json:"created_at"`
	SavedAt                           time.Time `json:"saved_at"`
	ArchivedAt                        time.Time `json:"archived_at"`
	UpdatedAt                         time.Time `json:"updated_at"`
}

// LinkWithArticle combines a user's link state with the article content.
type LinkWithArticle struct {
	Link
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Author      string    `json:"author"`
	ImageURL    string    `json:"image_url,omitempty"`
	ReadingTime float64   `json:"reading_time_minutes"`
	FeedTitle   *string   `json:"feed_title,omitempty"`
	PublishedAt time.Time `json:"published_at"`
}

// LinkDetail is used for the single article detail view, includes text content.
type LinkDetail struct {
	Link
	Title       string    `json:"title"`
	Author      string    `json:"author"`
	ImageURL    string    `json:"image_url,omitempty"`
	ReadingTime float64   `json:"reading_time_minutes"`
	TextContent string    `json:"text_content"`
	FeedTitle   *string   `json:"feed_title,omitempty"`
	PublishedAt time.Time `json:"published_at"`
}

type LinkModel struct {
	DB *sql.DB
}

func (m LinkModel) ListForUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*LinkWithArticle, int, error) {
	query := `
		SELECT count(*) OVER(),
			l.id, l.article_id, l.slug, l.feed_id, l.is_read, l.is_starred,
			l.saved_at, l.created_at, l.updated_at,
			a.title, a.description, a.author, COALESCE(NULLIF(a.image_url, ''), f.image_url) AS image_url, a.reading_time_minutes, a.published_at,
			f.original_title
		FROM links l
		JOIN articles a ON l.article_id = a.id
		LEFT JOIN feeds f ON l.feed_id = f.id
		WHERE l.user_id = $1
			AND l.archived_at IS NULL
		ORDER BY a.published_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := m.DB.QueryContext(ctx, query, userID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	totalRecords := 0
	var links []*LinkWithArticle

	for rows.Next() {
		var link LinkWithArticle
		err := rows.Scan(
			&totalRecords,
			&link.ID,
			&link.ArticleID,
			&link.Slug,
			&link.FeedID,
			&link.IsRead,
			&link.IsStarred,
			&link.SavedAt,
			&link.CreatedAt,
			&link.UpdatedAt,
			&link.Title,
			&link.Description,
			&link.Author,
			&link.ImageURL,
			&link.ReadingTime,
			&link.PublishedAt,
			&link.FeedTitle,
		)
		if err != nil {
			return nil, 0, err
		}
		link.UserID = userID
		links = append(links, &link)
	}

	if err = rows.Err(); err != nil {
		return nil, 0, err
	}

	return links, totalRecords, nil
}

func (m LinkModel) GetBySlug(ctx context.Context, slug string, userID uuid.UUID) (*LinkDetail, error) {
	query := `
		SELECT l.id, l.article_id, l.slug, l.feed_id, l.is_read, l.is_starred,
			l.saved_at, l.created_at, l.updated_at,
			a.title, a.author, a.image_url, a.reading_time_minutes,
			a.text_content, a.published_at,
			f.original_title
		FROM links l
		JOIN articles a ON l.article_id = a.id
		LEFT JOIN feeds f ON l.feed_id = f.id
		WHERE l.slug = $1 AND l.user_id = $2`

	var link LinkDetail
	err := m.DB.QueryRowContext(ctx, query, slug, userID).Scan(
		&link.ID,
		&link.ArticleID,
		&link.Slug,
		&link.FeedID,
		&link.IsRead,
		&link.IsStarred,
		&link.SavedAt,
		&link.CreatedAt,
		&link.UpdatedAt,
		&link.Title,
		&link.Author,
		&link.ImageURL,
		&link.ReadingTime,
		&link.TextContent,
		&link.PublishedAt,
		&link.FeedTitle,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("link not found")
		}
		return nil, err
	}

	link.UserID = userID
	return &link, nil
}

func (m LinkModel) Exists(ctx context.Context, userID uuid.UUID, articleID int64) (bool, error) {
	query := `
		SELECT EXISTS(
			SELECT 1 FROM links
			WHERE user_id = $1 AND article_id = $2
		)`

	var exists bool
	err := m.DB.QueryRowContext(ctx, query, userID, articleID).Scan(&exists)
	if err != nil {
		return false, err
	}

	return exists, nil
}

// BulkInsertForArticle creates links for multiple users in a single query.
// Uses ON CONFLICT DO NOTHING to skip users who already have this article.
func (m LinkModel) BulkInsertForArticle(ctx context.Context, userIDs []uuid.UUID, articleID int64, feedID int64, baseSlug string) error {
	if len(userIDs) == 0 {
		return nil
	}

	query := `
		INSERT INTO links (user_id, article_id, feed_id, slug, saved_at, updated_at)
		VALUES `

	// Each user has its own slug namespace (UNIQUE(user_id, slug)), so every
	// subscriber gets the same baseSlug. The previous per-position "-i" suffix
	// produced non-deterministic, ugly slugs (subscriber order isn't stable).
	args := []any{}
	for i, uid := range userIDs {
		if i > 0 {
			query += ", "
		}
		paramBase := i * 4
		query += fmt.Sprintf("($%d, $%d, $%d, $%d, NOW(), NOW())",
			paramBase+1, paramBase+2, paramBase+3, paramBase+4)
		args = append(args, uid, articleID, feedID, baseSlug)
	}

	query += " ON CONFLICT (user_id, article_id) DO NOTHING"

	_, err := m.DB.ExecContext(ctx, query, args...)
	return err
}

// SetReadStatus updates the read flag of a user's link, identified by slug.
// It returns ErrRecordNotFound if no matching link exists for that user.
func (m LinkModel) SetReadStatus(ctx context.Context, userID uuid.UUID, slug string, isRead bool) error {
	query := `
		UPDATE links
		SET is_read = $1, updated_at = NOW()
		WHERE slug = $2 AND user_id = $3`

	result, err := m.DB.ExecContext(ctx, query, isRead, slug, userID)
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

