package service

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
	"github.com/google/uuid"
)

var ErrAlreadySubscribed = errors.New("already subscribed to this feed")

type feedStore interface {
	GetByURL(ctx context.Context, url string) (*data.Feed, error)
	DeleteIfOrphan(ctx context.Context, id int64) error
	MarkDueForSync(ctx context.Context, ids []int64) error
}

type subscriptionStore interface {
	Exists(ctx context.Context, userID uuid.UUID, feedID int64) (bool, error)
	Insert(ctx context.Context, sub *data.Subscription) error
}

type feedImporter interface {
	CreateFromURL(ctx context.Context, url string) (*data.Feed, error)
	ImportArticlesForSubscribers(ctx context.Context, feed *data.Feed) error
}

// shutdownGrace bounds how long Shutdown waits for cancelled imports to return.
// It only has to cover the unwinding after cancellation, not the import itself.
const shutdownGrace = 5 * time.Second

func cancelledByShutdown(ctx context.Context, err error) bool {
	return ctx.Err() != nil && errors.Is(err, context.Canceled)
}

type SubscriptionService struct {
	feeds   feedStore
	subs    subscriptionStore
	feedSvc feedImporter
	logger  *jsonlog.Logger
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

func NewSubscriptionService(models data.Models, feedSvc *FeedService, logger *jsonlog.Logger) *SubscriptionService {
	ctx, cancel := context.WithCancel(context.Background())

	return &SubscriptionService{
		feeds:   models.Feeds,
		subs:    models.Subscriptions,
		feedSvc: feedSvc,
		logger:  logger,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// SubscribeOptions carries what varies between subscribing from the form and
// subscribing from an OPML import.
type SubscribeOptions struct {
	FolderID    *int64
	DeferImport bool
}

// Subscribe signs userID up to the feed at feedURL, creating the feed itself if
// nobody follows it yet
func (s *SubscriptionService) Subscribe(ctx context.Context, userID uuid.UUID, feedURL string, opts SubscribeOptions) (*data.Subscription, error) {
	feed, err := s.feeds.GetByURL(ctx, feedURL)
	if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
		return nil, err
	}

	feedCreated := feed == nil
	subscribed := false

	defer func() {
		if !feedCreated || subscribed || feed == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := s.feeds.DeleteIfOrphan(cleanupCtx, feed.ID); err != nil {
			s.logger.PrintError(err, map[string]string{
				"context": "cleaning up orphan feed",
				"feed_id": strconv.FormatInt(feed.ID, 10),
			})
		}
	}()

	if feed == nil {
		feed, err = s.feedSvc.CreateFromURL(ctx, feedURL)
		if err != nil {
			return nil, err
		}
	}

	exists, err := s.subs.Exists(ctx, userID, feed.ID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrAlreadySubscribed
	}

	subscription := &data.Subscription{
		UserID:   userID,
		FeedID:   feed.ID,
		FolderID: opts.FolderID,
	}

	if err := s.subs.Insert(ctx, subscription); err != nil {
		return nil, err
	}
	subscribed = true

	subscription.Feed = *feed

	if !feedCreated {
		if err := s.feeds.MarkDueForSync(ctx, []int64{feed.ID}); err != nil {
			s.logger.PrintError(err, map[string]string{
				"context": "marking an existing feed due for a new subscriber",
				"feed_id": strconv.FormatInt(feed.ID, 10),
			})
		}
	}

	if !opts.DeferImport {
		s.importAsync(feed)
	}

	return subscription, nil
}

// importAsync pulls the feed's articles in the background, so the caller isn't
// held while a whole feed is fetched and scraped.
func (s *SubscriptionService) importAsync(feed *data.Feed) {
	s.wg.Go(func() {
		err := s.feedSvc.ImportArticlesForSubscribers(s.ctx, feed)
		if err == nil {
			return
		}

		// An import cut short by Shutdown is the shutdown working, not a failure.
		// The scheduler's next tick picks the feed back up.
		if cancelledByShutdown(s.ctx, err) {
			s.logger.PrintInfo("article import cancelled at shutdown", map[string]string{
				"feed_id": strconv.FormatInt(feed.ID, 10),
			})
			return
		}

		// Same rule as the scheduler: a feed the far end would not serve is not
		// this program failing.
		var fetchErr *FeedFetchError
		if errors.As(err, &fetchErr) {
			s.logger.PrintInfo("article import failed", map[string]string{
				"feed_id":  strconv.FormatInt(feed.ID, 10),
				"reason":   fetchErr.Error(),
				"failures": strconv.Itoa(fetchErr.Failures),
			})
			return
		}

		s.logger.PrintError(err, map[string]string{
			"context": "importing articles for new subscription",
			"feed_id": strconv.FormatInt(feed.ID, 10),
		})
	})
}

// Shutdown cancels the imports still in flight and gives them a moment to
// unwind.
func (s *SubscriptionService) Shutdown() {
	s.cancel()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(shutdownGrace):
		s.logger.PrintError(errors.New("timed out waiting for article imports to stop"), nil)
	}
}
