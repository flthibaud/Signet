package data

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Article is a piece of content stored once for the whole instance, however
// many users saved it. Hash is what makes that possible: the SHA256 of the
// normalized URL (see service.HashURL), which is the deduplication key rather
// than Url itself. The per-user half — read state, progress, slug — lives in
// Link.
//
// OriginalHTML and Language are working data the API never returns: the first
// is the extracted source kept for re-processing, the second the PostgreSQL
// text search configuration the row's tsvector was built with.
type Article struct {
	ID           int64     `json:"id"`
	Url          string    `json:"url"`
	Hash         string    `json:"hash"`
	Title        string    `json:"title"`
	Description  string    `json:"description,omitempty"`
	Author       string    `json:"author"`
	ImageURL     string    `json:"image_url,omitempty"`
	PageType     string    `json:"page_type,omitempty"`
	ReadingTime  float64   `json:"reading_time_minutes"`
	OriginalHTML string    `json:"-"`
	TextContent  string    `json:"text_content"`
	Language     string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	PublishedAt  time.Time `json:"published_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ArticleModel gives access to the articles table.
type ArticleModel struct {
	DB *sql.DB
}

// IDByHash returns the ID of the article stored under a normalized-URL hash.
// This is the deduplication lookup, run for every item of every feed that
// changed: it reads the ID alone because the body columns can run to hundreds
// of KB a row, and a feed that sends no ETag would pull them all back on every
// sync. Returns ErrRecordNotFound when the article is new to the instance.
func (m ArticleModel) IDByHash(ctx context.Context, hash string) (int64, error) {
	var id int64
	err := m.DB.QueryRowContext(ctx, `SELECT id FROM articles WHERE hash = $1`, hash).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrRecordNotFound
	}
	return id, err
}

// Insert stores an article and sets its generated ID. An article the instance
// already has is not an error — see the upsert note below.
func (m ArticleModel) Insert(ctx context.Context, article *Article) error {
	// ON CONFLICT (hash) DO UPDATE is a no-op upsert: if a concurrent worker
	// already inserted this article (two feeds can share the same URL), we don't
	// fail, we just return the existing id. DO NOTHING would return no row.
	query := `
		INSERT INTO articles (url, hash, title, description, author, image_url, page_type, reading_time_minutes, original_html, text_content, language, published_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::regconfig, $12, $13)
		ON CONFLICT (hash) DO UPDATE SET hash = articles.hash
		RETURNING id
	`

	err := m.DB.QueryRowContext(ctx,
		query,
		article.Url,
		article.Hash,
		article.Title,
		article.Description,
		article.Author,
		article.ImageURL,
		article.PageType,
		article.ReadingTime,
		article.OriginalHTML,
		article.TextContent,
		article.Language,
		article.PublishedAt,
		time.Now(),
	).Scan(&article.ID)

	return err
}
