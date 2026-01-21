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

func (m LinkModel) Get(ctx context.Context, id int64) (*Link, error) {
	return nil, nil
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
