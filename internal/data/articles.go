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

// GetByHash returns the article stored under a normalized-URL hash. This is the
// deduplication lookup: the import path calls it to find out whether an item it
// just read is already stored, before creating a second copy of it. Returns
// ErrRecordNotFound when the article is new to the instance.
func (m ArticleModel) GetByHash(ctx context.Context, hash string) (*Article, error) {
	var article Article

	query := `
		SELECT id, url, hash, title, description, author, image_url, page_type, reading_time_minutes, original_html, text_content, language::text, published_at, created_at, updated_at
		FROM articles
		WHERE hash = $1`

	err := m.DB.QueryRowContext(ctx, query, hash).Scan(
		&article.ID,
		&article.Url,
		&article.Hash,
		&article.Title,
		&article.Description,
		&article.Author,
		&article.ImageURL,
		&article.PageType,
		&article.ReadingTime,
		&article.OriginalHTML,
		&article.TextContent,
		&article.Language,
		&article.PublishedAt,
		&article.CreatedAt,
		&article.UpdatedAt,
	)

	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}

	return &article, nil
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

// Get returns the article with the given ID, including the columns the API
// omits (OriginalHTML, Language). Articles are instance-wide, so this carries
// no ownership check: reach an article through a user's link to scope it.
func (m ArticleModel) Get(ctx context.Context, id int64) (*Article, error) {
	var article Article
	query := `
		SELECT id, url, hash, title, description, author, image_url, page_type, reading_time_minutes, original_html, text_content, language::text, published_at, created_at, updated_at
		FROM articles
		WHERE id = $1
	`

	err := m.DB.QueryRowContext(ctx, query, id).Scan(
		&article.ID,
		&article.Url,
		&article.Hash,
		&article.Title,
		&article.Description,
		&article.Author,
		&article.ImageURL,
		&article.PageType,
		&article.ReadingTime,
		&article.OriginalHTML,
		&article.TextContent,
		&article.Language,
		&article.PublishedAt,
		&article.CreatedAt,
		&article.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &article, nil
}
