package service

import (
	"bytes"
	"context"
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
	"github.com/flthibaud/signet/internal/safedial"
	"github.com/microcosm-cc/bluemonday"
	"github.com/mmcdole/gofeed"
	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// Errors a caller can act on when resolving a feed URL. ErrInvalidFeed means
// the document was fetched but is not RSS or Atom; ErrFeedNotFound means it
// could not be fetched at all. Both are the user's mistake rather than the
// server's, so handlers report them as a 4xx.
var (
	ErrInvalidFeed  = errors.New("invalid feed format")
	ErrFeedNotFound = errors.New("feed not found or unreachable")
)

// UserAgent is the User-Agent sent on RSS fetches. It matches the browser the
// scraper impersonates so we never announce two different
// browsers from the same deployment.
const UserAgent = browserUserAgent

// scrapeTimeout bounds a single article fetch, whichever transport handles it.
const scrapeTimeout = 30 * time.Second

// maxFeedBytes bounds how much of a feed document we pull into memory.
const maxFeedBytes = 8 << 20

// maxFaviconBytes bounds the homepage HTML we parse to find <link rel="icon">.
const maxFaviconBytes = 1 << 20

// faviconTimeout bounds the favicon lookup. It is a cosmetic extra on top of a
// feed the user is waiting for, so it gets far less patience than the fetch.
const faviconTimeout = 10 * time.Second

var errFeedTooLarge = errors.New("feed body exceeds size limit")

func readFeedBody(r io.Reader) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxFeedBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > maxFeedBytes {
		return nil, errFeedTooLarge
	}
	return body, nil
}

// FeedService fetches and parses feeds, extracts article content, and writes
// the results through the data layer. One instance is shared by the HTTP
// handlers and the scheduler's workers, so its fields must stay safe for
// concurrent use.
type FeedService struct {
	feeds       syncFeedStore
	articles    articleStore
	links       linkStore
	subscribers subscriberStore
	client      *http.Client
	parser      *gofeed.Parser
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

	// guard vets the address every outbound fetch resolves to. Feed and article
	// URLs are user-supplied, so without it the service is an SSRF proxy into
	// the deployment's own network.
	guard *safedial.Guard
}

// NewFeedService builds the service and every HTTP client it fetches with.
//
// A failure to construct the impersonating transport is logged rather than
// returned: the service falls back to the stdlib client, which fetches most
// sites fine, and refusing to start the whole app over a degraded scraper would
// be the worse trade.
func NewFeedService(models data.Models, logger *jsonlog.Logger, cfg FetchConfig) *FeedService {
	cfg.setDefaults()

	guard := safedial.NewGuard(cfg.AllowPrivateNetworks)

	// Every fetch in this service goes through this client or through one of the
	// two fetchers below, and all three carry the guard's dialer. The one
	// exception is the solver's own client, which talks to the sidecar rather
	// than to a user-supplied URL.
	client := &http.Client{
		Timeout:   scrapeTimeout,
		Transport: guard.Transport(scrapeTimeout),
	}
	stdlib := &stdlibFetcher{client: client}

	scrape, err := newScrapeFetcher(cfg.TLSImpersonate, scrapeTimeout, guard, stdlib)
	if err != nil && logger != nil {
		logger.PrintError(err, nil)
	}

	s := &FeedService{
		feeds:        models.Feeds,
		articles:     models.Articles,
		links:        models.Links,
		subscribers:  models.Subscriptions,
		client:       client,
		parser:       gofeed.NewParser(),
		logger:       logger,
		fetchCfg:     cfg,
		scrape:       scrape,
		scrapeStdlib: stdlib,
		guard:        guard,
	}

	if cfg.SolverURL != "" {
		s.solver = newSolverClient(cfg.SolverURL, cfg.SolverTimeout, guard)
	}

	return s
}

// CreateFromURL fetches an RSS/Atom document, reads its metadata and stores the
// feed. Returns ErrFeedNotFound if the URL could not be fetched, or
// ErrInvalidFeed if what came back is not a feed.
func (s *FeedService) CreateFromURL(ctx context.Context, feedURL string) (*data.Feed, error) {
	// 1. Fetch the feed. The User-Agent is explicit because some servers reject
	// requests that arrive without one.
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

	// 2. Parse the document.
	raw, err := readFeedBody(body)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFeed, err)
	}

	parsedFeed, err := s.parser.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidFeed, err)
	}

	// 3. Extract the metadata.
	imageURL := getFeedImageURL(parsedFeed)
	if imageURL == "" && parsedFeed.Link != "" {
		imageURL = fetchFaviconURL(ctx, s.client, parsedFeed.Link)
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

	// 4. Store it.
	err = s.feeds.Insert(ctx, feed)
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

// createArticleFromItem scrapes the page behind a feed item and stores the
// article built from it (see buildArticle).
func (s *FeedService) createArticleFromItem(ctx context.Context, item *gofeed.Item, link, hash, language string) (*data.Article, error) {
	page, fetchErr := s.fetchWithReadability(ctx, link)

	article := buildArticle(item, page, fetchErr, link, hash, language)
	if err := s.articles.Insert(ctx, article); err != nil {
		return nil, err
	}

	return article, nil
}

// buildArticle is the one rule that turns a feed item, and the page readability
// extracted for it, into an article. page and fetchErr are what
// fetchWithReadability returned.
//
// link is the URL settled on for the item (see itemURL) and hash its
// deduplication key. The link is stored as it came: normalization exists only
// to compute the hash, so that the same page reached through two feeds collapses
// to one article without the stored URL being rewritten under the user.
//
// language is an already-resolved PostgreSQL text search configuration, not a
// raw language tag — it is interpolated into the insert as a regconfig.
//
// Every string taken from the item or the page goes through cleanArticleString:
// one the database refuses would make the item unstorable for good.
func buildArticle(item *gofeed.Item, page readability.Article, fetchErr error, link, hash, language string) *data.Article {
	// Anti-bot interstitials never reach this point: fetchWithReadability
	// detects them on the response and returns an error.
	var pageTitle, pageHTML string
	if fetchErr == nil {
		pageTitle = page.Title()
		pageHTML = renderToValidUTF8(page.RenderHTML)
	}

	// The page and the item are taken field by field: a page without a <title>
	// still has the best body available.
	title := cleanArticleString(pageTitle)
	if title == "" {
		title = cleanArticleString(item.Title)
	}
	originalHTML := cleanArticleString(pageHTML)
	if originalHTML == "" {
		originalHTML = cleanArticleString(item.Content)
	}
	// Plain RSS 2.0 fills only Description (<content:encoded> lands in Content),
	// so a description-only item would otherwise be stored empty.
	if originalHTML == "" {
		originalHTML = cleanArticleString(item.Description)
	}

	// A body the Markdown converter chokes on is still worth reading and
	// searching, so it is kept as plain text rather than dropped.
	textContent, err := readabilitymd.NewReadability().HTMLToMarkdown(originalHTML)
	if err != nil {
		textContent = plainText(originalHTML)
	}
	textContent = cleanArticleString(textContent)

	return &data.Article{
		Url:          cleanArticleString(link),
		Hash:         hash,
		Title:        title,
		Description:  cleanArticleString(plainText(item.Description)),
		Author:       cleanArticleString(getAuthor(item)),
		ImageURL:     cleanArticleString(getItemImageURL(item)),
		PageType:     "article",
		ReadingTime:  estimateReadingTime(textContent),
		OriginalHTML: originalHTML,
		TextContent:  textContent,
		Language:     language,
		PublishedAt:  getPublishedDate(item),
	}
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

// fetchWithReadability fetches an article page and extracts its content.
//
// The fetch climbs the anti-bot ladder (see fetchPage): a browser-impersonating
// TLS client first, then the browser sidecar if a JS challenge persists. An
// error here makes the caller fall back to the RSS item's own content.
func (s *FeedService) fetchWithReadability(ctx context.Context, articleURL string) (readability.Article, error) {
	baseURL, err := url.Parse(articleURL)
	if err != nil {
		return readability.Article{}, err
	}

	// Rate limited per domain, so scraping several new articles from one feed
	// does not hammer the site they came from.
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

func getFeedImageURL(feed *gofeed.Feed) string {
	if feed.Image != nil {
		return feed.Image.URL
	}
	return ""
}

// fetchFaviconURL looks up a site's favicon, preferring a <link rel="icon"> in
// the homepage HTML and falling back to /favicon.ico. Returns "" if neither can
// be reached — a missing icon is cosmetic and never fails the surrounding work.
func fetchFaviconURL(ctx context.Context, client *http.Client, siteURL string) string {
	parsed, err := url.Parse(siteURL)
	if err != nil {
		return ""
	}

	fallback := parsed.Scheme + "://" + parsed.Host + "/favicon.ico"

	ctx, cancel := context.WithTimeout(ctx, faviconTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, siteURL, nil)
	if err != nil {
		return fallback
	}
	req.Header.Set("User-Agent", UserAgent)

	resp, err := client.Do(req)
	if err != nil {
		return fallback
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fallback
	}

	doc, err := html.Parse(io.LimitReader(resp.Body, maxFaviconBytes))
	if err != nil {
		return fallback
	}

	if href := findIconLink(doc); href != "" {
		// Resolve relative URLs against the site.
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

// cleanArticleString strips what PostgreSQL's text columns reject: invalid
// UTF-8 bytes and NUL characters (a NUL is valid UTF-8, so ToValidUTF8 alone
// lets it through). A feed item carrying either — a JSON Feed title with \u0000
// is one — would fail the insert on every sync, and since a failed item keeps
// the feed's ETag from advancing, the whole feed would be re-downloaded and
// the article page re-scraped forever.
func cleanArticleString(s string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(s, ""), "\x00", "")
}

func renderToValidUTF8(render func(w io.Writer) error) string {
	var sb strings.Builder
	if err := render(&sb); err != nil {
		return ""
	}
	return strings.ToValidUTF8(sb.String(), "")
}
