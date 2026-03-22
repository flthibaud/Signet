package data

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/text/unicode/norm"
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

func (m LinkModel) Insert(ctx context.Context, link *Link) error {
	query := `
		INSERT INTO links (user_id, article_id, slug, feed_id, saved_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	err := m.DB.QueryRow(
		query,
		link.UserID,
		link.ArticleID,
		link.Slug,
		link.FeedID,
		time.Now(),
		time.Now(),
	).Scan(&link.ID)

	if err != nil {
		return err
	}

	return nil
}

func (m LinkModel) ListForUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*LinkWithArticle, int, error) {
	query := `
		SELECT count(*) OVER(),
			l.id, l.article_id, l.slug, l.feed_id, l.is_read, l.is_starred,
			l.saved_at, l.created_at, l.updated_at,
			a.title, a.description, a.author, a.image_url, a.reading_time_minutes, a.published_at,
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

func (m LinkModel) GetByID(ctx context.Context, id int64, userID uuid.UUID) (*LinkDetail, error) {
	query := `
		SELECT l.id, l.article_id, l.slug, l.feed_id, l.is_read, l.is_starred,
			l.saved_at, l.created_at, l.updated_at,
			a.title, a.author, a.image_url, a.reading_time_minutes,
			a.text_content, a.published_at,
			f.original_title
		FROM links l
		JOIN articles a ON l.article_id = a.id
		LEFT JOIN feeds f ON l.feed_id = f.id
		WHERE l.id = $1 AND l.user_id = $2`

	var link LinkDetail
	err := m.DB.QueryRowContext(ctx, query, id, userID).Scan(
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

	args := []any{}
	for i, uid := range userIDs {
		if i > 0 {
			query += ", "
		}
		slug := baseSlug
		if i > 0 {
			slug = fmt.Sprintf("%s-%d", baseSlug, i)
		}
		paramBase := i * 5
		query += fmt.Sprintf("($%d, $%d, $%d, $%d, NOW(), NOW())",
			paramBase+1, paramBase+2, paramBase+3, paramBase+4)
		args = append(args, uid, articleID, feedID, slug)
	}

	query += " ON CONFLICT (user_id, article_id) DO NOTHING"

	_, err := m.DB.ExecContext(ctx, query, args...)
	return err
}

func (m LinkModel) Update(ctx context.Context, link *Link) error {
	return nil
}

func (m LinkModel) Delete(ctx context.Context, id int64) error {
	return nil
}

func (m LinkModel) GenerateUniqueSlug(ctx context.Context, userID uuid.UUID, title string) (string, error) {
	baseSlug := slugify(title)
	slug := baseSlug

	var count int

	for {
		var exists bool
		query := `
			SELECT EXISTS(
				SELECT 1 FROM links
				WHERE user_id = $1 AND slug = $2
			)`
		err := m.DB.QueryRow(query, userID, slug).Scan(&exists)
		if err != nil {
			return "", err
		}

		if !exists {
			break
		}

		count++
		slug = fmt.Sprintf("%s-%d", baseSlug, count)
	}

	return slug, nil
}

// slugify convertit une chaine en format URL-friendly (kebab-case)
// Elle gère le français (é -> e, ç -> c, œ -> oe) et l'anglais.
func slugify(title string) string {
	title = strings.ToLower(title)

	title = strings.ReplaceAll(title, "œ", "oe")
	title = strings.ReplaceAll(title, "æ", "ae")

	// Normaliser en NFD (Décomposition : "é" devient "e" + "accent")
	t := norm.NFD.String(title)

	var b strings.Builder
	for _, r := range t {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	slug := b.String()

	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug = re.ReplaceAllString(slug, "-")

	slug = strings.Trim(slug, "-")

	return slug
}
