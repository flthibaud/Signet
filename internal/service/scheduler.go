package service

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
	"golang.org/x/time/rate"
)

// Scheduler is the background worker pool that keeps feeds up to date. It runs
// three periodic jobs on their own goroutines: the feed sync itself, an hourly
// sweep of expired tokens, and a sweep of OPML imports that stopped reporting
// progress.
//
// Create it with NewScheduler, run it with Start, and always pair Start with
// Stop — the workers hold a context the scheduler owns.
type Scheduler struct {
	syncer         feedSyncer
	models         data.Models
	logger         *jsonlog.Logger
	interval       time.Duration
	workers        int
	batchSize      int
	quit           chan struct{}
	wake           chan struct{}
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	domainLimiters sync.Map
}

// feedSyncer runs feed syncs; *FeedService is the real one. The scheduler only
// decides when and how many at once: what makes a feed due and what a sync
// records belong to the feed sync.
type feedSyncer interface {
	ClaimDueFeeds(ctx context.Context, n int, interval time.Duration) ([]*data.Feed, error)
	SyncFeed(ctx context.Context, feed *data.Feed) SyncResult
}

// DefaultSyncInterval is used when no valid interval is configured. A
// non-positive one would panic time.NewTicker and make every feed permanently
// due in GetFeedsToSync.
const DefaultSyncInterval = 15 * time.Minute

// tokenCleanupInterval is how often expired tokens are swept.
const tokenCleanupInterval = time.Hour

// staleImportAge is how long an OPML import may go without writing progress
// before it is presumed dead.
const staleImportAge = 30 * time.Minute

// SchedulerConfig sizes the scheduler: how often feeds are refreshed, how many
// sync at once, and how many are claimed per batch.
type SchedulerConfig struct {
	Interval  time.Duration
	Workers   int
	BatchSize int
}

// cancelledByShutdown reports whether err is the cancellation of ctx, i.e. the
// work was cut short on purpose rather than failing.
func cancelledByShutdown(ctx context.Context, err error) bool {
	return ctx.Err() != nil && errors.Is(err, context.Canceled)
}

// NewScheduler builds a scheduler without starting it. A non-positive interval
// is replaced by DefaultSyncInterval rather than rejected, since the alternative
// is a ticker that panics at Start.
//
// models serves only the housekeeping sweeps; feed syncs go through syncer.
func NewScheduler(syncer feedSyncer, models data.Models, logger *jsonlog.Logger, cfg SchedulerConfig) *Scheduler {
	if cfg.Interval <= 0 {
		cfg.Interval = DefaultSyncInterval
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		syncer:    syncer,
		models:    models,
		logger:    logger,
		interval:  cfg.Interval,
		workers:   cfg.Workers,
		batchSize: cfg.BatchSize,
		quit:      make(chan struct{}),
		wake:      make(chan struct{}, 1),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// TriggerSync asks for a sync pass without waiting for the next tick.
func (s *Scheduler) TriggerSync() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Start launches the scheduler's goroutines and returns immediately.
//
// The first sync runs at once rather than on the first tick, so a freshly
// started instance does not leave feeds stale for a whole interval. Call Stop
// to shut it down.
func (s *Scheduler) Start() {
	s.logger.PrintInfo("scheduler started", map[string]string{
		"interval":   s.interval.String(),
		"workers":    strconv.Itoa(s.workers),
		"batch_size": strconv.Itoa(s.batchSize),
	})

	s.wg.Go(func() {
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.syncFeeds()

		for {
			select {
			case <-ticker.C:
				s.syncFeeds()
			case <-s.wake:
				s.syncFeeds()
			case <-s.quit:
				return
			}
		}
	})

	s.wg.Go(func() {
		ticker := time.NewTicker(tokenCleanupInterval)
		defer ticker.Stop()

		s.housekeeping()

		for {
			select {
			case <-ticker.C:
				s.housekeeping()
			case <-s.quit:
				return
			}
		}
	})
}

func (s *Scheduler) housekeeping() {
	s.purgeExpiredTokens()
	s.failStaleImports()
}

func (s *Scheduler) logBackgroundError(err error, component string) {
	if cancelledByShutdown(s.ctx, err) {
		return
	}

	s.logger.PrintError(err, map[string]string{"component": component})
}

func (s *Scheduler) failStaleImports() {
	interrupted, err := s.models.OPMLImports.FailStale(s.ctx, staleImportAge)
	if err != nil {
		s.logBackgroundError(err, "opml_import_cleanup")
		return
	}

	if interrupted > 0 {
		s.logger.PrintInfo("stale OPML imports marked interrupted", map[string]string{
			"count": strconv.FormatInt(interrupted, 10),
		})
	}
}

func (s *Scheduler) purgeExpiredTokens() {
	deleted, err := s.models.Tokens.DeleteExpired(s.ctx)
	if err != nil {
		s.logBackgroundError(err, "token_cleanup")
		return
	}

	if deleted > 0 {
		s.logger.PrintInfo("expired tokens deleted", map[string]string{
			"count": strconv.FormatInt(deleted, 10),
		})
	}
}

// Stop cancels the workers' context and blocks until every one of them has
// returned, so a shutdown does not cut a sync off mid-write. It is not safe to
// call twice: the second close of the quit channel panics.
func (s *Scheduler) Stop() {
	s.logger.PrintInfo("scheduler stopping, waiting for workers...", nil)
	close(s.quit)
	s.cancel()
	s.wg.Wait()
	s.logger.PrintInfo("scheduler stopped", nil)
}

// syncFeeds claims due feeds batch after batch until none is left. One batch per
// tick capped the instance at SCHEDULER_BATCH_SIZE feeds per interval, so past
// that many feeds each one was refreshed less often than configured. The batch
// still bounds how many feeds sit claimed at once, which is what keeps their
// locks from going stale before a worker reaches them.
//
// The loop ends because every processed feed leaves the due set: success and
// failure both stamp last_fetched_at. The exceptions are a feed a new
// subscriber marked due mid-sync, which should be synced again, and a shutdown,
// which ends the loop anyway.
func (s *Scheduler) syncFeeds() {
	ctx := s.ctx

	for ctx.Err() == nil {
		feeds, err := s.syncer.ClaimDueFeeds(ctx, s.batchSize, s.interval)
		if err != nil {
			s.logBackgroundError(err, "scheduler")
			return
		}

		if len(feeds) == 0 {
			return
		}

		s.logger.PrintInfo("syncing feeds", map[string]string{
			"count": strconv.Itoa(len(feeds)),
		})

		feedsChan := make(chan *data.Feed, len(feeds))
		for _, f := range feeds {
			feedsChan <- f
		}
		close(feedsChan)

		// Every claimed feed reaches processFeed, even after a shutdown: SyncFeed
		// is what gives the claim back.
		var workerWg sync.WaitGroup
		for range s.workers {
			workerWg.Go(func() {
				for feed := range feedsChan {
					s.processFeed(ctx, feed)
				}
			})
		}
		workerWg.Wait()

		// A short batch was everything that was due.
		if len(feeds) < s.batchSize {
			return
		}
	}
}

func (s *Scheduler) processFeed(ctx context.Context, feed *data.Feed) {
	// A wait cut short by the shutdown still goes on to SyncFeed, which sees the
	// cancelled context and gives the claim back.
	_ = domainLimiter(&s.domainLimiters, extractDomain(feed.Url)).Wait(ctx)

	res := s.syncer.SyncFeed(ctx, feed)

	props := map[string]string{
		"feed_id": strconv.FormatInt(feed.ID, 10),
		"url":     feed.Url,
		"status":  string(res.Status),
	}
	if res.NewArticles > 0 {
		props["new_articles"] = strconv.Itoa(res.NewArticles)
	}
	if res.FailedItems > 0 {
		props["failed_items"] = strconv.Itoa(res.FailedItems)
	}
	if res.UnstorableItems > 0 {
		props["unstorable_items"] = strconv.Itoa(res.UnstorableItems)
	}
	if res.Status == SyncFailed || res.Status == SyncDeactivated {
		props["failures"] = strconv.Itoa(res.Failures)
	}
	if res.Err != nil {
		props["reason"] = res.Err.Error()
	}

	// A feed the far end would not serve is not this program failing; only a
	// write of ours that did not happen is.
	if res.StoreErr != nil {
		s.logger.PrintError(res.StoreErr, props)
		return
	}
	s.logger.PrintInfo("feed sync finished", props)
}

// domainLimiter returns the 1 req/s limiter m holds for host, creating it on
// first use. Feed polling and article scraping each pass their own map, so a
// sync's poll of a site does not eat into the budget for scraping its articles.
func domainLimiter(m *sync.Map, host string) *rate.Limiter {
	if v, ok := m.Load(host); ok {
		return v.(*rate.Limiter)
	}
	actual, _ := m.LoadOrStore(host, rate.NewLimiter(rate.Every(time.Second), 1))
	return actual.(*rate.Limiter)
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}
