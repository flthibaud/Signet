package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	readability "codeberg.org/readeck/go-readability/v2"
	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
	readabilitymd "github.com/flthibaud/signet/internal/readability"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

var (
	ErrInvalidFeed  = errors.New("invalid feed format")
	ErrFeedNotFound = errors.New("feed not found or unreachable")
)

// UserAgent is the User-Agent sent on RSS fetches. It matches the browser the
// scraper impersonates (see browserUserAgent) so we never announce two different
// browsers from the same deployment.
const UserAgent = browserUserAgent

// feedProcessTimeout bounds how long a single feed sync may run. It must stay
// below the GetFeedsToSync lock window (10 minutes) so we always release the
// feed before another worker considers the lock stale and reclaims it.
const feedProcessTimeout = 8 * time.Minute

// scrapeTimeout bounds a single article fetch, whichever transport handles it.
const scrapeTimeout = 30 * time.Second

type FeedService struct {
	models      data.Models
	client      *http.Client
	parser      *gofeed.Parser
	readability *readabilitymd.Readability
	logger      *jsonlog.Logger
	fetchCfg    FetchConfig

	// scrape is the transport used for article pages: a browser-impersonating
	// TLS client by default. scrapeStdlib is the same net/http client the RSS
	// polling uses, kept reachable as a fallback. solver is the browser sidecar,
	// nil when unconfigured.
	scrape       pageFetcher
	scrapeStdlib pageFetcher
	solver       *solverClient

	// scrapeLimiters rate-limits readability fetches per source domain (1 req/s),
	// so distributing many new articles from one site doesn't hammer it.
	scrapeLimiters sync.Map // map[string]*rate.Limiter

	// challengedHosts remembers hosts that answered with an anti-bot challenge,
	// so their other articles skip straight to the sidecar.
	challengedHosts sync.Map // map[string]time.Time (expiry)
}

func NewFeedService(models data.Models, logger *jsonlog.Logger, cfg FetchConfig) *FeedService {
	cfg.setDefaults()

	client := &http.Client{Timeout: scrapeTimeout}
	stdlib := &stdlibFetcher{client: client}

	scrape, err := newScrapeFetcher(cfg.TLSImpersonate, scrapeTimeout, stdlib)
	if err != nil && logger != nil {
		logger.PrintError(err, nil)
	}

	s := &FeedService{
		models:       models,
		client:       client,
		parser:       gofeed.NewParser(),
		readability:  readabilitymd.NewReadability(),
		logger:       logger,
		fetchCfg:     cfg,
		scrape:       scrape,
		scrapeStdlib: stdlib,
	}

	if cfg.SolverURL != "" {
		s.solver = newSolverClient(cfg.SolverURL, cfg.SolverTimeout)
	}

	return s
}

// CreateFromURL crée un feed depuis une URL RSS
func (s *FeedService) CreateFromURL(ctx context.Context, feedURL string) (*data.Feed, error) {
	// 1. Fetch le RSS (avec User-Agent : certains serveurs rejettent les
	// requêtes sans en-tête UA).
	req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFeedNotFound, err)
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFeedNotFound, err)
	}
	defer resp.Body.Close()

	body := io.Reader(resp.Body)

	if resp.StatusCode != http.StatusOK {
		// Unlike background polling, this runs in front of a user waiting on the
		// "add feed" form, so a Cloudflare block is a visible failure. Retry once
		// with the impersonated client — the double fetch is fine here: it's a
		// rare, manual action, not the 15-minute poll.
		retried, retryErr := s.retryFeedFetch(ctx, feedURL, resp.StatusCode)
		if retryErr != nil {
			return nil, fmt.Errorf("%w: status %d", ErrFeedNotFound, resp.StatusCode)
		}
		body = bytes.NewReader(retried)
	}

	// 2. Parse le RSS
	parsedFeed, err := s.parser.Parse(body)
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

// retryFeedFetch re-fetches a feed URL with the impersonated client after the
// stdlib client was rejected. It only fires on statuses that look like anti-bot
// filtering — a 404 stays a 404, and the RSS polling path never calls this.
func (s *FeedService) retryFeedFetch(ctx context.Context, feedURL string, status int) ([]byte, error) {
	if s.scrape == s.scrapeStdlib {
		return nil, fmt.Errorf("no impersonated client available")
	}
	switch status {
	case http.StatusForbidden, http.StatusServiceUnavailable, http.StatusTooManyRequests:
	default:
		return nil, fmt.Errorf("status %d is not an anti-bot block", status)
	}

	parsed, err := url.Parse(feedURL)
	if err != nil {
		return nil, err
	}

	page, err := s.scrape.fetch(ctx, parsed)
	if err != nil {
		return nil, err
	}
	if page.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", page.StatusCode)
	}

	s.logFetch("feed fetch recovered with impersonated client", parsed, strconv.Itoa(status), nil)
	return page.Body, nil
}

func plainText(s string) string {
	stripped := bluemonday.StrictPolicy().Sanitize(s)
	return strings.TrimSpace(html.UnescapeString(stripped))
}

// createArticleFromItem crée un article depuis un RSS item
func (s *FeedService) createArticleFromItem(ctx context.Context, item *gofeed.Item, hash string) (*data.Article, error) {
	parsed, err := s.fetchWithReadability(ctx, item.Link)

	var title, originalHTML, textContent string

	// Anti-bot interstitials no longer reach this point: fetchWithReadability
	// detects them on the response itself and returns an error, so an empty
	// title here just means readability found nothing usable.
	if err != nil || parsed.Title() == "" {
		title = item.Title
		originalHTML = item.Content
		textContent, _ = s.readability.HTMLToMarkdown(originalHTML)
	} else {
		title = parsed.Title()
		originalHTML = renderToValidUTF8(parsed.RenderHTML)
		textContent, _ = s.readability.HTMLToMarkdown(originalHTML)
	}

	// Crée l'article
	article := &data.Article{
		Url:          item.Link,
		Hash:         hash,
		Title:        title,
		Description:  plainText(item.Description),
		Author:       getAuthor(item),
		ImageURL:     getItemImageURL(item),
		PageType:     "article",
		ReadingTime:  estimateReadingTime(textContent),
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
	// Bound the whole feed processing below the lock window so we always release
	// the feed before another worker can reclaim the (stale) lock.
	ctx, cancel := context.WithTimeout(ctx, feedProcessTimeout)
	defer cancel()

	// Cap browser solves for this run: each one can take a minute, and letting
	// them eat the whole feedProcessTimeout would starve the remaining items and
	// stop the ETag from advancing, making the next tick redo everything.
	ctx = withSolveBudget(ctx, s.fetchCfg.SolverMaxPerFeed)

	// 1. Build HTTP request with conditional headers
	req, err := http.NewRequestWithContext(ctx, "GET", feed.Url, nil)
	if err != nil {
		return s.markFailed(ctx, feed.ID, err)
	}
	req.Header.Set("User-Agent", UserAgent)
	if feed.HttpEtag != "" {
		req.Header.Set("If-None-Match", feed.HttpEtag)
	}
	if feed.HttpLastModified != "" {
		req.Header.Set("If-Modified-Since", feed.HttpLastModified)
	}

	// 2. Execute request
	resp, err := s.client.Do(req)
	if err != nil {
		return s.markFailed(ctx, feed.ID, err)
	}
	defer resp.Body.Close()

	// 3. Handle 304 Not Modified
	if resp.StatusCode == http.StatusNotModified {
		return s.models.Feeds.ReleaseFeed(ctx, feed.ID, feed.HttpEtag, feed.HttpLastModified)
	}

	// 4. Handle error status codes
	if resp.StatusCode != http.StatusOK {
		return s.markFailed(ctx, feed.ID, fmt.Errorf("feed %d returned status %d", feed.ID, resp.StatusCode))
	}

	// 5. Parse feed
	parsedFeed, err := s.parser.Parse(resp.Body)
	if err != nil {
		return s.markFailed(ctx, feed.ID, err)
	}

	// 6. Get HTTP caching headers from response. If the server omits one, keep
	// the previous value rather than clearing it, so we don't lose conditional
	// fetching on the next tick.
	newEtag := resp.Header.Get("ETag")
	if newEtag == "" {
		newEtag = feed.HttpEtag
	}
	newLastModified := resp.Header.Get("Last-Modified")
	if newLastModified == "" {
		newLastModified = feed.HttpLastModified
	}

	// 7. Get all subscriber IDs
	subscriberIDs, err := s.models.Subscriptions.GetSubscriberIDs(ctx, feed.ID)
	if err != nil {
		return s.markFailed(ctx, feed.ID, err)
	}

	// 8. Process each item. If any item fails, we must NOT persist the new
	// ETag/Last-Modified, otherwise the next fetch gets a 304 and the items we
	// missed are never retried.
	itemsFailed := false
	for _, item := range parsedFeed.Items {
		// Stop if we've hit the processing deadline (or the app is shutting
		// down): the remaining items will be retried on the next tick.
		if ctx.Err() != nil {
			itemsFailed = true
			break
		}

		hash := hashURL(item.Link)

		// Check if article already exists
		article, err := s.models.Articles.GetByHash(ctx, hash)
		if err != nil && err != data.ErrRecordNotFound {
			itemsFailed = true
			continue
		}

		// Create article if new
		if article == nil {
			article, err = s.createArticleFromItem(ctx, item, hash)
			if err != nil {
				itemsFailed = true
				continue
			}
		}

		// Bulk insert links for all subscribers
		if len(subscriberIDs) > 0 {
			baseSlug := slugFromHash(hash)
			if err := s.models.Links.BulkInsertForArticle(ctx, subscriberIDs, article.ID, feed.ID, baseSlug); err != nil {
				itemsFailed = true
			}
		}
	}

	// 9. Release feed. Only advance the caching headers when every item
	// succeeded; on partial failure we keep the previous headers so the feed is
	// fully re-fetched (no 304) next time.
	//
	// Use a context detached from cancellation: when we break out of the loop on
	// a hit deadline, ctx is already done and the release (which clears the lock)
	// must still run.
	releaseCtx, cancelRelease := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancelRelease()

	if itemsFailed {
		return s.models.Feeds.ReleaseFeed(releaseCtx, feed.ID, feed.HttpEtag, feed.HttpLastModified)
	}
	return s.models.Feeds.ReleaseFeed(releaseCtx, feed.ID, newEtag, newLastModified)
}

// markFailed records a feed failure (clearing the lock, incrementing the
// counter, deactivating after the threshold) and returns an error mentioning
// the deactivation so the caller logs it. The DB write uses a detached context
// so it still runs if the processing deadline has already fired.
func (s *FeedService) markFailed(ctx context.Context, feedID int64, cause error) error {
	dbCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()

	failures, err := s.models.Feeds.MarkFeedFailed(dbCtx, feedID)
	if err != nil {
		return errors.Join(cause, fmt.Errorf("marking feed %d failed: %w", feedID, err))
	}
	if failures >= 10 {
		return fmt.Errorf("feed %d deactivated after %d consecutive failures: %w", feedID, failures, cause)
	}
	return cause
}

// estimateReadingTime returns an estimated reading time in minutes from the
// article text, assuming ~200 words per minute. Capped at 500 to satisfy the
// articles.reading_time_minutes CHECK constraint.
func estimateReadingTime(text string) float64 {
	words := len(strings.Fields(text))
	if words == 0 {
		return 0
	}
	minutes := float64(words) / 200.0
	if minutes > 500 {
		return 500
	}
	return minutes
}

// slugFromHash generates a URL-friendly slug from an article hash.
func slugFromHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

// fetchWithReadability récupère la page d'un article et en extrait le contenu.
//
// Le fetch passe par l'échelle anti-bot (voir fetchPage) : client TLS imitant un
// navigateur, puis sidecar navigateur si un challenge JS persiste. Une erreur
// ici fait retomber l'appelant sur le contenu de l'item RSS.
func (s *FeedService) fetchWithReadability(ctx context.Context, articleURL string) (readability.Article, error) {
	baseURL, err := url.Parse(articleURL)
	if err != nil {
		return readability.Article{}, err
	}

	// Respecte un rate limit par domaine pour ne pas marteler le site source
	// quand on scrape plusieurs nouveaux articles du même flux.
	if err := s.scrapeLimiter(baseURL.Host).Wait(ctx); err != nil {
		return readability.Article{}, err
	}

	page, err := s.fetchPage(ctx, baseURL)
	if err != nil {
		return readability.Article{}, err
	}

	// Parse avec readability
	return readability.FromReader(bytes.NewReader(page.Body), page.URL)
}

// scrapeLimiter returns the per-domain rate limiter for readability fetches,
// creating it (1 req/s) on first use.
func (s *FeedService) scrapeLimiter(domain string) *rate.Limiter {
	if v, ok := s.scrapeLimiters.Load(domain); ok {
		return v.(*rate.Limiter)
	}
	limiter := rate.NewLimiter(rate.Every(time.Second), 1)
	actual, _ := s.scrapeLimiters.LoadOrStore(domain, limiter)
	return actual.(*rate.Limiter)
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
