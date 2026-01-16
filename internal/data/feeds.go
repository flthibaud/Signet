package data

import (
	"database/sql"
	"net/url"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
)

type Feed struct {
	ID            int64     `json:"id"`
	Url           string    `json:"url"`
	Title         string    `json:"title"`
	SiteUrl       string    `json:"site_url"`
	ImageUrl      string    `json:"image_url,omitempty"`
	IsActive      bool      `json:"is_active"`
	LastFetchedAt time.Time `json:"last_fetched_at"`
	CreatedAt     time.Time `json:"created_at"`
}

type FeedModel struct {
	DB *sql.DB
}

func (m FeedModel) SubscribeUser(userID uuid.UUID, url string) (*Subscription, error) {
	// Transaction recommandée ici car on touche à 2 tables
	tx, err := m.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// 1. Chercher ou Créer le Feed (Table feeds)
	var feedID int64

	// On vérifie d'abord si le feed existe
	err = tx.QueryRow("SELECT id FROM feeds WHERE url = $1", url).Scan(&feedID)
	if err == sql.ErrNoRows {
		// Le feed n'existe pas, on le crée.
		defaultTitle := extractSiteName(url)
		siteUrl := extractSiteUrl(url)

		// On insère l'URL ET le titre par défaut
		queryInsert := `
			INSERT INTO feeds (url, site_url, original_title) 
			VALUES ($1, $2, $3) 
			RETURNING id`

		err = tx.QueryRow(queryInsert, url, siteUrl, defaultTitle).Scan(&feedID)
	}

	if err != nil {
		return nil, err
	}

	// 2. Créer l'abonnement (Table subscriptions)
	var sub Subscription
	querySub := `
		INSERT INTO subscriptions (user_id, feed_id)
		VALUES ($1, $2)
		ON CONFLICT (user_id, feed_id) DO UPDATE SET user_id = EXCLUDED.user_id
		RETURNING id, feed_id, user_id, created_at`

	err = tx.QueryRow(querySub, userID, feedID).Scan(&sub.ID, &sub.FeedID, &sub.UserID, &sub.CreatedAt)
	if err != nil {
		return nil, err
	}

	return &sub, tx.Commit()
}

func extractSiteName(feedUrl string) string {
	u, err := url.Parse(feedUrl)
	if err != nil {
		return "New Feed" // Fallback si l'URL est malformée
	}

	// 1. Récupérer le host
	host := u.Hostname()

	// 2. Retirer le "www." s'il existe
	host = strings.TrimPrefix(host, "www.")

	// 3. Garder juste la partie avant le premier point
	parts := strings.Split(host, ".")
	if len(parts) > 0 {
		name := parts[0]

		// 4. Mettre la première lettre en majuscule (Capitalize)
		if len(name) > 0 {
			runes := []rune(name)
			runes[0] = unicode.ToUpper(runes[0])
			return string(runes)
		}
		return name
	}

	return host
}

func extractSiteUrl(feedUrl string) string {
	u, err := url.Parse(feedUrl)
	if err != nil {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func (m FeedModel) Get(id int64) (*Feed, error) {
	var feed Feed
	query := `
		SELECT id, url, original_title, site_url, is_active, created_at
		FROM feeds
		WHERE id = $1`

	err := m.DB.QueryRow(query, id).Scan(
		&feed.ID,
		&feed.Url,
		&feed.Title,
		&feed.SiteUrl,
		&feed.IsActive,
		&feed.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &feed, nil
}

func (m FeedModel) UpdateMetadata(feedID int64, title, imageUrl string) error {
	query := `
		UPDATE feeds
		SET title = $1, image_url = $2, last_fetched_at = NOW()
		WHERE id = $3`

	_, err := m.DB.Exec(query, title, imageUrl, feedID)
	return err
}
