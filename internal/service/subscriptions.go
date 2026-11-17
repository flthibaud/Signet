package service

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
	"github.com/google/uuid"
)

// ErrAlreadySubscribed reports that the user is already subscribed to the feed.
// Handlers turn it into a 422 rather than silently creating a second row.
var ErrAlreadySubscribed = errors.New("already subscribed to this feed")

type feedStore interface {
	GetByURL(ctx context.Context, url string) (*data.Feed, error)
	DeleteIfOrphan(ctx context.Context, id int64) error
	MarkDueForSync(ctx context.Context, ids []int64) error
}

type subscriptionStore interface {
	Insert(ctx context.Context, sub *data.Subscription) error
}

type feedCreator interface {
	CreateFromURL(ctx context.Context, url string) (*data.Feed, error)
}

// SyncTrigger asks the scheduler to run a sync pass now instead of waiting for
// its next tick.
type SyncTrigger interface {
	TriggerSync()
}

// SubscriptionService owns the subscribe sequence: resolve or create the feed,
// insert the subscription, and hand the feed to the scheduler.
//
// It depends on the narrow store interfaces declared above rather than on
// data.Models, so it can be driven with fakes in tests; data.FeedModel and
// data.SubscriptionModel satisfy them as they are.
//
// It never imports articles itself. The scheduler is the only thing that runs
// a feed sync, so every sync holds a claim on its feed; an import started from
// here would run unclaimed, alongside any worker already syncing that feed.
type SubscriptionService struct {
	feeds   feedStore
	subs    subscriptionStore
	feedSvc feedCreator
	trigger SyncTrigger
	logger  *jsonlog.Logger
}

// NewSubscriptionService builds the service. trigger is the scheduler.
func NewSubscriptionService(models data.Models, feedSvc *FeedService, trigger SyncTrigger, logger *jsonlog.Logger) *SubscriptionService {
	return &SubscriptionService{
		feeds:   models.Feeds,
		subs:    models.Subscriptions,
		feedSvc: feedSvc,
		trigger: trigger,
		logger:  logger,
	}
}

// SubscribeOptions carries what varies between subscribing from the form and
// subscribing from an OPML import.
type SubscribeOptions struct {
	FolderID *int64
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

	subscription := &data.Subscription{
		UserID:   userID,
		FeedID:   feed.ID,
		FolderID: opts.FolderID,
	}

	if err := s.subs.Insert(ctx, subscription); err != nil {
		// The unique constraint is the only race-safe "already subscribed"
		// check: two concurrent subscribes both pass any up-front lookup, but
		// only one insert can win.
		if errors.Is(err, data.ErrDuplicateSubscription) {
			return nil, ErrAlreadySubscribed
		}
		return nil, err
	}
	subscribed = true

	subscription.Feed = *feed

	// A new subscriber's articles come from the next sync of the feed, so make
	// it due now and wake the scheduler rather than wait for its next tick.
	if err := s.feeds.MarkDueForSync(ctx, []int64{feed.ID}); err != nil {
		s.logger.PrintError(err, map[string]string{
			"context": "marking a feed due for a new subscriber",
			"feed_id": strconv.FormatInt(feed.ID, 10),
		})
	} else {
		s.trigger.TriggerSync()
	}

	return subscription, nil
}
