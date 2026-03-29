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
	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/flthibaud/omnivore-go/internal/data"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
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
	imageURL := getFeedImageURL(parsedFeed)
	if imageURL == "" && parsedFeed.Link != "" {
		imageURL = fetchFaviconURL(s.client, parsedFeed.Link)
	}

	feed := &data.Feed{
		Url:           feedURL,
		Title:         parsedFeed.Title,
		SiteUrl:       parsedFeed.Link,
		ImageUrl:      imageURL,
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

// createArticleFromItem crée un article depuis un RSS item
func (s *FeedService) createArticleFromItem(ctx context.Context, item *gofeed.Item, hash string) (*data.Article, error) {
	strip := bluemonday.StrictPolicy()

	parsed, err := s.fetchWithReadability(item.Link)

	var title, originalHTML, textContent string

	if err != nil || parsed.Title() == "Just a moment..." || parsed.Title() == "" {
		title = item.Title
		originalHTML = item.Content
		textContent, _ = htmltomarkdown.ConvertString(originalHTML)
	} else {
		title = parsed.Title()
		originalHTML = renderToValidUTF8(parsed.RenderHTML)
		textContent, _ = htmltomarkdown.ConvertString(originalHTML)
	}

	// Crée l'article
	article := &data.Article{
		Url:          item.Link,
		Hash:         hash,
		Title:        title,
		Description:  strip.Sanitize(item.Description),
		Author:       getAuthor(item),
		ImageURL:     getItemImageURL(item),
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

// ImportArticlesForSubscribers fetches a feed with conditional HTTP headers
// and distributes new articles to all subscribers via bulk insert.
func (s *FeedService) ImportArticlesForSubscribers(ctx context.Context, feed *data.Feed) error {
	// 1. Build HTTP request with conditional headers
	req, err := http.NewRequestWithContext(ctx, "GET", feed.Url, nil)
	if err != nil {
		s.models.Feeds.MarkFeedFailed(ctx, feed.ID)
		return err
	}
	if feed.HttpEtag != "" {
		req.Header.Set("If-None-Match", feed.HttpEtag)
	}
	if feed.HttpLastModified != "" {
		req.Header.Set("If-Modified-Since", feed.HttpLastModified)
	}

	// 2. Execute request
	resp, err := s.client.Do(req)
	if err != nil {
		s.models.Feeds.MarkFeedFailed(ctx, feed.ID)
		return err
	}
	defer resp.Body.Close()

	// 3. Handle 304 Not Modified
	if resp.StatusCode == http.StatusNotModified {
		return s.models.Feeds.ReleaseFeed(ctx, feed.ID, feed.HttpEtag, feed.HttpLastModified)
	}

	// 4. Handle error status codes
	if resp.StatusCode != http.StatusOK {
		s.models.Feeds.MarkFeedFailed(ctx, feed.ID)
		return fmt.Errorf("feed %d returned status %d", feed.ID, resp.StatusCode)
	}

	// 5. Parse feed
	parsedFeed, err := s.parser.Parse(resp.Body)
	if err != nil {
		s.models.Feeds.MarkFeedFailed(ctx, feed.ID)
		return err
	}

	// 6. Get HTTP caching headers from response
	newEtag := resp.Header.Get("ETag")
	newLastModified := resp.Header.Get("Last-Modified")

	// 7. Get all subscriber IDs
	subscriberIDs, err := s.models.Subscriptions.GetSubscriberIDs(ctx, feed.ID)
	if err != nil {
		s.models.Feeds.MarkFeedFailed(ctx, feed.ID)
		return err
	}

	// 8. Process each item
	for _, item := range parsedFeed.Items {
		hash := hashURL(item.Link)

		// Check if article already exists
		article, err := s.models.Articles.GetByHash(ctx, hash)
		if err != nil && err != data.ErrRecordNotFound {
			continue
		}

		// Create article if new
		if article == nil {
			article, err = s.createArticleFromItem(ctx, item, hash)
			if err != nil {
				continue
			}
		}

		// Bulk insert links for all subscribers
		if len(subscriberIDs) > 0 {
			baseSlug := slugFromHash(hash)
			_ = s.models.Links.BulkInsertForArticle(ctx, subscriberIDs, article.ID, feed.ID, baseSlug)
		}
	}

	// 9. Release feed with updated caching headers
	return s.models.Feeds.ReleaseFeed(ctx, feed.ID, newEtag, newLastModified)
}

// slugFromHash generates a URL-friendly slug from an article hash.
func slugFromHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
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

func getFeedImageURL(feed *gofeed.Feed) string {
	if feed.Image != nil {
		return feed.Image.URL
	}
	return ""
}

// fetchFaviconURL tente de récupérer l'URL du favicon d'un site.
// Cherche d'abord un <link rel="icon"> dans le HTML, sinon fallback sur /favicon.ico.
func fetchFaviconURL(client *http.Client, siteURL string) string {
	parsed, err := url.Parse(siteURL)
	if err != nil {
		return ""
	}

	fallback := parsed.Scheme + "://" + parsed.Host + "/favicon.ico"

	resp, err := client.Get(siteURL)
	if err != nil {
		return fallback
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fallback
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return fallback
	}

	if href := findIconLink(doc); href != "" {
		// Résout les URLs relatives
		ref, err := url.Parse(href)
		if err != nil {
			return href
		}
		return parsed.ResolveReference(ref).String()
	}

	return fallback
}

// findIconLink parcourt le HTML pour trouver un <link rel="icon"> ou <link rel="shortcut icon">.
func findIconLink(n *html.Node) string {
	if n.Type == html.ElementNode && n.DataAtom.String() == "link" {
		var rel, href string
		for _, attr := range n.Attr {
			switch attr.Key {
			case "rel":
				rel = strings.ToLower(attr.Val)
			case "href":
				href = attr.Val
			}
		}
		if href != "" && (rel == "icon" || rel == "shortcut icon" || rel == "apple-touch-icon") {
			return href
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := findIconLink(c); result != "" {
			return result
		}
	}
	return ""
}

func getItemImageURL(item *gofeed.Item) string {
	if item.Image != nil {
		return item.Image.URL
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
