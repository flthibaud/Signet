package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/flthibaud/omnivore-go/internal/data"
	"github.com/google/uuid"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
)

var (
	ErrInvalidFeed  = errors.New("invalid feed format")
	ErrFeedNotFound = errors.New("feed not found or unreachable")
)

type FeedService struct {
	models data.Models
	client *http.Client
	parser *gofeed.Parser
}

func NewFeedService(models data.Models) *FeedService {
	return &FeedService{
		models: models,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		parser: gofeed.NewParser(),
	}
}

// CreateFromURL crée un feed depuis une URL RSS
func (s *FeedService) CreateFromURL(ctx context.Context, feedURL string) (*data.Feed, error) {
	// 1. Fetch le RSS
	resp, err := s.client.Get(feedURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFeedNotFound, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrFeedNotFound, resp.StatusCode)
	}

	// 2. Parse le RSS
	parsedFeed, err := s.parser.Parse(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFeed, err)
	}

	// 3. Extrait les métadonnées
	feed := &data.Feed{
		Url:           feedURL,
		Title:         parsedFeed.Title,
		SiteUrl:       parsedFeed.Link,
		ImageUrl:      getImageURL(parsedFeed),
		LastFetchedAt: time.Now(),
		IsActive:      true,
		CreatedAt:     time.Now(),
	}

	// 4. Insert en base
	err = s.models.Feeds.Insert(ctx, feed)
	if err != nil {
		return nil, err
	}

	return feed, nil
}

// ImportArticles importe les articles d'un feed pour un user
func (s *FeedService) ImportArticles(ctx context.Context, feedID int64, userID uuid.UUID) error {
	// 1. Récupère le feed
	feed, err := s.models.Feeds.Get(ctx, feedID)
	if err != nil {
		return err
	}

	// 2. Fetch & parse le RSS avec context
	req, _ := http.NewRequestWithContext(ctx, "GET", feed.Url, nil)
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	parsedFeed, err := s.parser.Parse(resp.Body)
	if err != nil {
		return err
	}

	// 3. Pour chaque item du feed
	imported := 0
	for _, item := range parsedFeed.Items {
		hash := hashURL(item.Link)

		// Check si article existe déjà
		article, err := s.models.Articles.GetByHash(ctx, hash)
		if err != nil && err != data.ErrRecordNotFound {
			continue
		}

		// Si article n'existe pas, le créer
		if article == nil {
			article, err = s.createArticleFromItem(ctx, item, hash)
			if err != nil {
				continue
			}
		}

		// Crée le lien pour ce user
		exists, _ := s.models.Links.Exists(ctx, userID, article.ID)
		if !exists {
			slug, _ := s.models.Links.GenerateUniqueSlug(ctx, userID, article.Title)

			link := &data.Link{
				UserID:    userID,
				ArticleID: article.ID,
				FeedID:    &feedID,
				Slug:      slug,
			}

			err = s.models.Links.Insert(ctx, link)
			if err == nil {
				imported++
			}
		}
	}

	// 4. Met à jour last_fetched_at
	return s.models.Feeds.UpdateLastFetched(ctx, feedID)
}

// createArticleFromItem crée un article depuis un RSS item
func (s *FeedService) createArticleFromItem(ctx context.Context, item *gofeed.Item, hash string) (*data.Article, error) {
	p := bluemonday.UGCPolicy()

	parsed, err := s.fetchWithReadability(item.Link)

	var title, originalHTML, textContent string

	if err != nil || parsed.Title() == "Just a moment..." || parsed.Title() == "" {
		title = item.Title
		originalHTML = p.Sanitize(item.Content)
		textContent = p.Sanitize(item.Description)
	} else {
		title = parsed.Title()
		originalHTML = renderToValidUTF8(parsed.RenderHTML)
		textContent = renderToValidUTF8(parsed.RenderText)
	}

	// Crée l'article
	article := &data.Article{
		Url:          item.Link,
		Hash:         hash,
		Title:        title,
		Description:  item.Description,
		Author:       getAuthor(item),
		ImageURL:     item.Image.URL,
		PageType:     "article",
		ReadingTime:  0, // @TODO: Calculer le temps de lecture
		OriginalHTML: originalHTML,
		TextContent:  textContent,
		PublishedAt:  getPublishedDate(item),
	}

	err = s.models.Articles.Insert(ctx, article)
	if err != nil {
		return nil, err
	}

	return article, nil
}

// fetchWithReadability tente de fetch avec un User-Agent réaliste
func (s *FeedService) fetchWithReadability(articleURL string) (readability.Article, error) {
	baseURL, err := url.Parse(articleURL)
	if err != nil {
		return readability.Article{}, err
	}

	// Crée une requête HTTP custom avec User-Agent
	req, err := http.NewRequest("GET", baseURL.String(), nil)
	if err != nil {
		return readability.Article{}, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return readability.Article{}, err
	}
	defer resp.Body.Close()

	// Parse avec readability
	return readability.FromReader(resp.Body, baseURL)
}

// hashURL calcule le hash SHA256 d'une URL
func hashURL(url string) string {
	h := sha256.New()
	h.Write([]byte(url))
	return hex.EncodeToString(h.Sum(nil))
}

func getImageURL(feed *gofeed.Feed) string {
	if feed.Image != nil {
		return feed.Image.URL
	}
	return ""
}

func getAuthor(item *gofeed.Item) string {
	if item.Author != nil {
		return item.Author.Name
	}
	return ""
}

func getPublishedDate(item *gofeed.Item) time.Time {
	if item.PublishedParsed != nil {
		return *item.PublishedParsed
	}
	return time.Now()
}

func renderToValidUTF8(render func(w io.Writer) error) string {
	var sb strings.Builder
	if err := render(&sb); err != nil {
		return ""
	}
	return strings.ToValidUTF8(sb.String(), "")
}
