package data

import (
	"database/sql"
	"time"
)

type Article struct {
	ID          int64     `json:"id"`
	Url         string    `json:"url"`
	Hash        string    `json:"hash"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Author      string    `json:"author"`
	ImageURL    string    `json:"image_url,omitempty"`
	PageType    string    `json:"page_type,omitempty"`
	ReadingTime float64   `json:"reading_time"`
	Content     string    `json:"content,omitempty"`
	TextContent string    `json:"text_content"`
	PublishedAt time.Time `json:"-"`
	CreatedAt   time.Time `json:"-"`
}

type ArticleModel struct {
	DB *sql.DB
}

// 1. Vérifier si un article existe
func (m ArticleModel) GetIDByURL(url string) (int64, error) {
	var id int64
	query := `SELECT id FROM articles WHERE url = $1`
	err := m.DB.QueryRow(query, url).Scan(&id)
	return id, err
}

// 2. Insérer un article (On gère le conflit ici)
func (m ArticleModel) Insert(article *Article) (int64, error) {
	query := `
		INSERT INTO articles (url, hash, title, content, text_content, published_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (url) DO UPDATE SET title = EXCLUDED.title
		RETURNING id`

	var id int64
	err := m.DB.QueryRow(query,
		article.Url, article.Hash, article.Title, article.Content, article.TextContent, article.PublishedAt,
	).Scan(&id)

	// Si l'insert ne renvoie rien (cas rare de conflit sans update), on récupère l'ID
	if err == sql.ErrNoRows {
		return m.GetIDByURL(article.Url)
	}
	return id, err
}

func (m ArticleModel) Get(id int64) (*Article, error) {
	var article Article
	query := `
		SELECT id, url, hash, title, description, author, image_url, page_type, reading_time, content, text_content, published_at, created_at
		FROM articles
		WHERE id = $1
	`

	err := m.DB.QueryRow(query, id).Scan(
		&article.ID,
		&article.Url,
		&article.Hash,
		&article.Title,
		&article.Description,
		&article.Author,
		&article.ImageURL,
		&article.PageType,
		&article.ReadingTime,
		&article.Content,
		&article.TextContent,
		&article.PublishedAt,
		&article.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &article, nil
}

func (m ArticleModel) Update(article *Article) error {
	return nil
}

func (m ArticleModel) Delete(id int64) error {
	return nil
}
