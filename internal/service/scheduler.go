package service

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/flthibaud/origami/internal/data"
	"github.com/flthibaud/origami/internal/jsonlog"
	"golang.org/x/time/rate"
)

type Scheduler struct {
	services       *Services
	logger         *jsonlog.Logger
	interval       time.Duration
	workers        int
	batchSize      int
	quit           chan struct{}
	ctx            context.Context    // cancelled on Stop to abort in-flight syncs
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	domainLimiters sync.Map // map[string]*rate.Limiter
}

func NewScheduler(services *Services, logger *jsonlog.Logger, interval time.Duration, workers, batchSize int) *Scheduler {
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

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
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
	}()
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

	feeds, err := s.services.FeedService.models.Feeds.GetFeedsToSync(ctx, s.batchSize)
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
		workerWg.Add(1)
		go func() {
			defer workerWg.Done()
			for job := range feedsChan {
				s.processFeed(ctx, job)
			}
		}()
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
			s.services.FeedService.models.Feeds.MarkFeedFailed(ctx, job.feed.ID)
		}
	}()

	// Rate limit per domain
	domain := extractDomain(job.feed.Url)
	limiter := s.getOrCreateLimiter(domain)
	_ = limiter.Wait(ctx)

	err := s.services.FeedService.ImportArticlesForSubscribers(ctx, job.feed)
	if err != nil {
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
