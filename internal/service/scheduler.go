package service

import (
	"context"
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
	ctx            context.Context // cancelled on Stop to abort in-flight syncs
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	domainLimiters sync.Map // map[string]*rate.Limiter
}

// DefaultSyncInterval is used when no valid interval is configured. A
// non-positive one would panic time.NewTicker and make every feed permanently
// due in GetFeedsToSync.
const DefaultSyncInterval = 15 * time.Minute

// tokenCleanupInterval is how often expired tokens are swept. Nothing depends
// on the timing — expired tokens already authenticate no one — so this only has
// to be often enough that the table doesn't grow without bound.
const tokenCleanupInterval = time.Hour

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
		ctx:       ctx,
		cancel:    cancel,
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

		// Run immediately on start
		s.syncFeeds()

		for {
			select {
			case <-ticker.C:
				s.syncFeeds()
			case <-s.quit:
				return
			}
		}
	})

	// Housekeeping runs on its own goroutine rather than sharing the feed tick:
	// the two have nothing to do with each other, and a long sync shouldn't
	// delay the sweep (or the other way round).
	s.wg.Go(func() {
		ticker := time.NewTicker(tokenCleanupInterval)
		defer ticker.Stop()

		s.purgeExpiredTokens()

		for {
			select {
			case <-ticker.C:
				s.purgeExpiredTokens()
			case <-s.quit:
				return
			}
		}
	})
}

// purgeExpiredTokens drops lapsed authentication and activation tokens. They
// are already inert — GetForToken filters on expiry — so this is about keeping
// the table from growing for the life of the install.
func (s *Scheduler) purgeExpiredTokens() {
	deleted, err := s.services.models.Tokens.DeleteExpired(s.ctx)
	if err != nil {
		s.logger.PrintError(err, map[string]string{"component": "token_cleanup"})
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
	close(s.quit) // stop scheduling new ticks
	s.cancel()    // abort any in-flight feed syncs
	s.wg.Wait()
	s.logger.PrintInfo("scheduler stopped", nil)
}

func (s *Scheduler) syncFeeds() {
	ctx := s.ctx

	feeds, err := s.services.FeedService.models.Feeds.GetFeedsToSync(ctx, s.batchSize, s.interval)
	if err != nil {
		s.logger.PrintError(err, map[string]string{"component": "scheduler"})
		return
	}

	if len(feeds) == 0 {
		return
	}

	s.logger.PrintInfo("syncing feeds", map[string]string{
		"count": strconv.Itoa(len(feeds)),
	})

	// Fan out to worker pool via buffered channel
	feedsChan := make(chan *feedSyncJob, len(feeds))
	for _, f := range feeds {
		feedsChan <- &feedSyncJob{feed: f}
	}
	close(feedsChan)

	var workerWg sync.WaitGroup
	for i := 0; i < s.workers; i++ {
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

			// Detached context: a panic during shutdown arrives with ctx already
			// cancelled, and that is exactly when the failure needs recording.
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

	// Rate limit per domain. Wait only errors when the context is done, which on
	// this path means Stop was called — carrying on would fetch with a dead
	// context and report the failure as if the feed were at fault.
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
