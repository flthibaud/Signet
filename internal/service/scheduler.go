package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
	"golang.org/x/time/rate"
)

type Scheduler struct {
	services       *Services
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

// DefaultSyncInterval is used when no valid interval is configured. A
// non-positive one would panic time.NewTicker and make every feed permanently
// due in GetFeedsToSync.
const DefaultSyncInterval = 15 * time.Minute

// tokenCleanupInterval is how often expired tokens are swept.
const tokenCleanupInterval = time.Hour

// staleImportAge is how long an OPML import may go without writing progress
// before it is presumed dead.
const staleImportAge = 30 * time.Minute

func NewScheduler(services *Services, logger *jsonlog.Logger, interval time.Duration, workers, batchSize int) *Scheduler {
	if interval <= 0 {
		interval = DefaultSyncInterval
	}

	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		services:  services,
		logger:    logger,
		interval:  interval,
		workers:   workers,
		batchSize: batchSize,
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
	interrupted, err := s.services.models.OPMLImports.FailStale(s.ctx, staleImportAge)
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
	deleted, err := s.services.models.Tokens.DeleteExpired(s.ctx)
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

func (s *Scheduler) Stop() {
	s.logger.PrintInfo("scheduler stopping, waiting for workers...", nil)
	close(s.quit)
	s.cancel()
	s.wg.Wait()
	s.logger.PrintInfo("scheduler stopped", nil)
}

func (s *Scheduler) syncFeeds() {
	ctx := s.ctx

	feeds, err := s.services.FeedService.models.Feeds.GetFeedsToSync(ctx, s.batchSize, s.interval)
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

	feedsChan := make(chan *feedSyncJob, len(feeds))
	for _, f := range feeds {
		feedsChan <- &feedSyncJob{feed: f}
	}
	close(feedsChan)

	var workerWg sync.WaitGroup
	for range s.workers {
		workerWg.Go(func() {
			for job := range feedsChan {
				s.processFeed(ctx, job)
			}
		})
	}
	workerWg.Wait()
}

type feedSyncJob struct {
	feed *data.Feed
}

func (s *Scheduler) processFeed(ctx context.Context, job *feedSyncJob) {
	defer func() {
		if r := recover(); r != nil {
			s.logger.PrintError(fmt.Errorf("panic processing feed %d: %v", job.feed.ID, r), map[string]string{
				"feed_id": strconv.FormatInt(job.feed.ID, 10),
				"url":     job.feed.Url,
			})

			markCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if _, err := s.services.models.Feeds.MarkFeedFailed(markCtx, job.feed.ID); err != nil {
				s.logger.PrintError(err, map[string]string{
					"context": "marking feed failed after panic",
					"feed_id": strconv.FormatInt(job.feed.ID, 10),
				})
			}
		}
	}()

	domain := extractDomain(job.feed.Url)
	limiter := s.getOrCreateLimiter(domain)
	if err := limiter.Wait(ctx); err != nil {
		return
	}

	err := s.services.FeedService.ImportArticlesForSubscribers(ctx, job.feed)
	if err != nil {
		if cancelledByShutdown(ctx, err) {
			s.logger.PrintInfo("feed sync cancelled at shutdown", map[string]string{
				"feed_id": strconv.FormatInt(job.feed.ID, 10),
			})
			return
		}

		var fetchErr *FeedFetchError
		if errors.As(err, &fetchErr) {
			s.logger.PrintInfo("feed sync failed", map[string]string{
				"feed_id":  strconv.FormatInt(job.feed.ID, 10),
				"url":      job.feed.Url,
				"reason":   fetchErr.Error(),
				"failures": strconv.Itoa(fetchErr.Failures),
			})
			return
		}

		s.logger.PrintError(err, map[string]string{
			"feed_id": strconv.FormatInt(job.feed.ID, 10),
			"url":     job.feed.Url,
		})
	}
}

func (s *Scheduler) getOrCreateLimiter(domain string) *rate.Limiter {
	if v, ok := s.domainLimiters.Load(domain); ok {
		return v.(*rate.Limiter)
	}
	limiter := rate.NewLimiter(rate.Every(time.Second), 1)
	actual, _ := s.domainLimiters.LoadOrStore(domain, limiter)
	return actual.(*rate.Limiter)
}

func extractDomain(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	return u.Host
}
