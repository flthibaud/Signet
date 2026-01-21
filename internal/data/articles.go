package data

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

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
	CreatedAt    time.Time `json:"-"`
	PublishedAt  time.Time `json:"-"`
	UpdatedAt    time.Time `json:"-"`
}

type ArticleModel struct {
	DB *sql.DB
}

func (m ArticleModel) GetByHash(ctx context.Context, hash string) (*Article, error) {
	var article Article

	query := `
		SELECT id, url, hash, title, description, author, image_url, page_type, reading_time_minutes, original_html, text_content, published_at, created_at, updated_at
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

// 1. Vérifier si un article existe
func (m ArticleModel) GetIDByURL(ctx context.Context, url string) (int64, error) {
	var id int64
	query := `SELECT id FROM articles WHERE url = $1`
	err := m.DB.QueryRowContext(ctx, query, url).Scan(&id)
	return id, err
}

// 2. Insérer un article
func (m ArticleModel) Insert(ctx context.Context, article *Article) error {
	query := `
		INSERT INTO articles (url, hash, title, description, author, image_url, page_type, reading_time_minutes, original_html, text_content, published_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
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
		article.PublishedAt,
		time.Now(),
	).Scan(&article.ID)

	return err
}

func (m ArticleModel) Get(ctx context.Context, id int64) (*Article, error) {
	var article Article
	query := `
		SELECT id, url, hash, title, description, author, image_url, page_type, reading_time_minutes, original_html, text_content, published_at, created_at
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
		&article.PublishedAt,
		&article.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &article, nil
}

func (m ArticleModel) Update(ctx context.Context, article *Article) error {
	return nil
}

func (m ArticleModel) Delete(ctx context.Context, id int64) error {
	return nil
}
