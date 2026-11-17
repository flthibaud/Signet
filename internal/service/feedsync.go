package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/google/uuid"
	"github.com/mmcdole/gofeed"
)

// The feed sync's rules. They live here, next to the code that relies on them,
// and reach the queries as parameters, so no rule is written twice.
const (
	// feedFailureThreshold is how many consecutive failures turn a feed off.
	feedFailureThreshold = 10

	// feedProcessTimeout bounds how long a single feed sync may run.
	feedProcessTimeout = 8 * time.Minute

	// syncLockWindow is how long a claim holds before another worker may take
	// the feed over. Deriving it from feedProcessTimeout keeps a slow but live
	// sync from ever being picked up twice.
	syncLockWindow = feedProcessTimeout + 2*time.Minute

	// syncIntervalSlack shrinks the refresh interval when testing whether a feed
	// is due. last_fetched_at is stamped when a sync *finishes*, slightly after
	// the tick that started it, so an exact comparison would push every feed to
	// the tick after the next one and halve the effective refresh rate.
	syncIntervalSlack = 0.9

	// deadFeedRetryInterval is how often a deactivated feed is tried again.
	// Deactivation slows a failing feed down rather than ending it: a site down
	// for an afternoon would otherwise be dropped for every subscriber.
	deadFeedRetryInterval = 24 * time.Hour

	// releaseTimeout bounds the write that closes a sync. It runs on a context
	// detached from cancellation, so it still happens after a deadline or a
	// shutdown has fired.
	releaseTimeout = 5 * time.Second
)

// SyncStatus is how a feed sync ended.
type SyncStatus string

const (
	// SyncNotModified: the feed answered 304, nothing to import.
	SyncNotModified SyncStatus = "not_modified"
	// SyncSynced: the feed was read and its items imported; FailedItems counts
	// the ones that will be retried next time.
	SyncSynced SyncStatus = "synced"
	// SyncFailed: the feed could not be fetched or parsed.
	SyncFailed SyncStatus = "failed"
	// SyncDeactivated: a failure that took the feed to the threshold.
	SyncDeactivated SyncStatus = "deactivated"
	// SyncCancelled: the shutdown cut the sync short; nothing is held against
	// the feed.
	SyncCancelled SyncStatus = "cancelled"
)

// SyncResult reports one feed sync.
type SyncResult struct {
	Status      SyncStatus
	NewArticles int
	FailedItems int
	// UnstorableItems are items the database refused for their content
	// (data.ErrUnstorable). Retrying would get the same refusal, so unlike
	// FailedItems they do not hold the caching headers back.
	UnstorableItems int
	// Failures is the feed's consecutive failure count after a SyncFailed or
	// SyncDeactivated.
	Failures int
	// Err is why the feed failed: the far end's doing, not ours.
	Err error
	// StoreErr is a write that closes the sync and did not happen. That one is
	// our own infrastructure failing.
	StoreErr error
}

// The stores a feed sync writes through. data.FeedModel, data.ArticleModel,
// data.LinkModel and data.SubscriptionModel satisfy them as they are.

type syncFeedStore interface {
	Insert(ctx context.Context, feed *data.Feed) error
	GetFeedsToSync(ctx context.Context, batchSize int, dueAfter, lockWindow, deadRetry time.Duration) ([]*data.Feed, error)
	ReleaseFeed(ctx context.Context, feedID int64, etag, lastModified string) error
	MarkFeedFailed(ctx context.Context, feedID int64, threshold int) (int, error)
	Unclaim(ctx context.Context, feedID int64) error
}

type articleStore interface {
	GetByHash(ctx context.Context, hash string) (*data.Article, error)
	Insert(ctx context.Context, article *data.Article) error
}

type linkStore interface {
	BulkInsertForArticle(ctx context.Context, userIDs []uuid.UUID, articleID int64, feedID int64, baseSlug string) error
}

type subscriberStore interface {
	GetSubscriberIDs(ctx context.Context, feedID int64) ([]uuid.UUID, error)
}

// ClaimDueFeeds claims up to n due feeds for syncing, interval being how often
// a feed is meant to be refreshed. Every feed it returns must go to SyncFeed,
// which gives the claim back.
func (s *FeedService) ClaimDueFeeds(ctx context.Context, n int, interval time.Duration) ([]*data.Feed, error) {
	dueAfter := time.Duration(float64(interval) * syncIntervalSlack)
	return s.feeds.GetFeedsToSync(ctx, n, dueAfter, syncLockWindow, deadFeedRetryInterval)
}

// SyncFeed runs one feed sync on a feed claimed through ClaimDueFeeds: fetch it
// conditionally, turn its new items into articles, give every subscriber a link
// to each, and record the outcome on the feed, which also gives the claim back.
//
// It runs on the scheduler's workers, which the HTTP middleware's recoverPanic
// cannot see, so a panic anywhere below — the readability parser chewing on
// arbitrary third-party HTML is the known case — is recovered here and recorded
// as a failure instead of taking the whole process down.
func (s *FeedService) SyncFeed(ctx context.Context, feed *data.Feed) (res SyncResult) {
	shutdown := ctx

	defer func() {
		if r := recover(); r != nil {
			res = s.fail(shutdown, feed, fmt.Errorf("panic syncing feed: %v", r))
		}
	}()

	if ctx.Err() != nil {
		return s.unclaim(feed)
	}

	// Bound the whole sync below the lock window, so the claim is always given
	// back before another worker would consider it stale.
	ctx, cancel := context.WithTimeout(ctx, feedProcessTimeout)
	defer cancel()

	// Cap browser solves for this run: each one can take a minute, and letting
	// them eat the whole timeout would starve the remaining items and stop the
	// ETag from advancing, making the next sync redo everything.
	ctx = withSolveBudget(ctx, s.fetchCfg.SolverMaxPerFeed)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feed.Url, nil)
	if err != nil {
		return s.fail(shutdown, feed, err)
	}
	req.Header.Set("User-Agent", UserAgent)
	if feed.HttpEtag != "" {
		req.Header.Set("If-None-Match", feed.HttpEtag)
	}
	if feed.HttpLastModified != "" {
		req.Header.Set("If-Modified-Since", feed.HttpLastModified)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return s.fail(shutdown, feed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		return s.release(feed, SyncResult{Status: SyncNotModified}, feed.HttpEtag, feed.HttpLastModified)
	}
	if resp.StatusCode != http.StatusOK {
		return s.fail(shutdown, feed, fmt.Errorf("feed returned status %d", resp.StatusCode))
	}

	raw, err := readFeedBody(resp.Body)
	if err != nil {
		return s.fail(shutdown, feed, err)
	}
	parsed, err := s.parser.Parse(bytes.NewReader(raw))
	if err != nil {
		return s.fail(shutdown, feed, err)
	}

	// A server that omits a caching header keeps the previous value rather than
	// clearing it, so conditional fetching survives the next sync.
	etag := resp.Header.Get("ETag")
	if etag == "" {
		etag = feed.HttpEtag
	}
	lastModified := resp.Header.Get("Last-Modified")
	if lastModified == "" {
		lastModified = feed.HttpLastModified
	}

	subscriberIDs, err := s.subscribers.GetSubscriberIDs(ctx, feed.ID)
	if err != nil {
		return s.fail(shutdown, feed, err)
	}

	// The language the feed declares (<language>, xml:lang) is the most reliable
	// signal available: the overwhelming majority of feeds carry it, and
	// guessing from the text would be less certain. Absent, this resolves to the
	// neutral configuration.
	language := data.ResolveTextSearchConfig(parsed.Language)

	res = SyncResult{Status: SyncSynced}
	for i, item := range parsed.Items {
		if ctx.Err() != nil {
			if shutdown.Err() != nil {
				return s.unclaim(feed)
			}
			// Our own deadline: the items not reached are retried next sync.
			res.FailedItems += len(parsed.Items) - i
			break
		}

		link := itemURL(item.Link, item.GUID)
		if link == "" {
			continue
		}

		created, err := s.syncItem(ctx, feed, item, link, language, subscriberIDs)
		if err != nil {
			if shutdown.Err() != nil {
				return s.unclaim(feed)
			}
			unstorable := errors.Is(err, data.ErrUnstorable)
			if unstorable {
				res.UnstorableItems++
			} else {
				res.FailedItems++
			}
			s.logItemFailure(feed, link, err, unstorable)
			continue
		}
		if created {
			res.NewArticles++
		}
	}

	// Advancing the caching headers past a failed item would make the next
	// fetch a 304, and the item would never be retried.
	if res.FailedItems > 0 {
		etag, lastModified = feed.HttpEtag, feed.HttpLastModified
	}
	return s.release(feed, res, etag, lastModified)
}

// syncItem makes sure the article behind one feed item exists and that every
// subscriber has a link to it. It reports whether it created the article.
func (s *FeedService) syncItem(ctx context.Context, feed *data.Feed, item *gofeed.Item, link, language string, subscriberIDs []uuid.UUID) (bool, error) {
	hash := hashURL(link)

	article, err := s.articles.GetByHash(ctx, hash)
	if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
		return false, err
	}

	created := false
	if article == nil {
		article, err = s.createArticleFromItem(ctx, item, link, hash, language)
		if err != nil {
			return false, err
		}
		created = true
	}

	if len(subscriberIDs) > 0 {
		if err := s.links.BulkInsertForArticle(ctx, subscriberIDs, article.ID, feed.ID, slugFromHash(hash)); err != nil {
			return created, err
		}
	}
	return created, nil
}

// release records a sync that reached the feed and gives the claim back.
func (s *FeedService) release(feed *data.Feed, res SyncResult, etag, lastModified string) SyncResult {
	ctx, cancel := context.WithTimeout(context.Background(), releaseTimeout)
	defer cancel()

	res.StoreErr = s.feeds.ReleaseFeed(ctx, feed.ID, etag, lastModified)
	return res
}

// fail records a failed sync, deactivating the feed at the threshold. A failure
// the shutdown caused is not the feed's fault and goes to unclaim instead.
func (s *FeedService) fail(shutdown context.Context, feed *data.Feed, cause error) SyncResult {
	if shutdown.Err() != nil {
		return s.unclaim(feed)
	}

	ctx, cancel := context.WithTimeout(context.Background(), releaseTimeout)
	defer cancel()

	res := SyncResult{Status: SyncFailed, Err: cause}
	res.Failures, res.StoreErr = s.feeds.MarkFeedFailed(ctx, feed.ID, feedFailureThreshold)
	if res.StoreErr == nil && res.Failures >= feedFailureThreshold {
		res.Status = SyncDeactivated
	}
	return res
}

// unclaim gives the claim back untouched, so the feed keeps its failure count
// and the next start syncs it at once.
func (s *FeedService) unclaim(feed *data.Feed) SyncResult {
	ctx, cancel := context.WithTimeout(context.Background(), releaseTimeout)
	defer cancel()

	return SyncResult{Status: SyncCancelled, StoreErr: s.feeds.Unclaim(ctx, feed.ID)}
}

// logItemFailure makes a failed item visible. A transient failure holds the
// feed's ETag back, so one that keeps recurring costs a full re-download each
// sync; an unstorable item is skipped for good. This line is the only trace of
// either.
func (s *FeedService) logItemFailure(feed *data.Feed, link string, err error, unstorable bool) {
	if s.logger == nil {
		return
	}
	s.logger.PrintInfo("feed item failed", map[string]string{
		"feed_id":   strconv.FormatInt(feed.ID, 10),
		"item":      link,
		"reason":    err.Error(),
		"permanent": strconv.FormatBool(unstorable),
	})
}
