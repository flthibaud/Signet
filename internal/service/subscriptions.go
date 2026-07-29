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

// ErrAlreadySubscribed reports that the user already follows the feed behind the
// requested URL. The service only names the situation; whether that reads as a
// validation error on a form field is the HTTP layer's call.
var ErrAlreadySubscribed = errors.New("already subscribed to this feed")

// The three interfaces below are sized to exactly what Subscribe calls, and are
// declared here rather than in internal/data so the data layer stays free of
// abstractions written for one consumer. data.FeedModel and
// data.SubscriptionModel satisfy them as they are.

type feedStore interface {
	GetByURL(ctx context.Context, url string) (*data.Feed, error)
	DeleteIfOrphan(ctx context.Context, id int64) error
}

type subscriptionStore interface {
	Exists(ctx context.Context, userID uuid.UUID, feedID int64) (bool, error)
	Insert(ctx context.Context, sub *data.Subscription) error
}

// feedImporter is what SubscriptionService needs from FeedService.
type feedImporter interface {
	CreateFromURL(ctx context.Context, url string) (*data.Feed, error)
	ImportArticlesForSubscribers(ctx context.Context, feed *data.Feed) error
}

// shutdownGrace bounds how long Shutdown waits for cancelled imports to return.
// It only has to cover the unwinding after cancellation, not the import itself.
const shutdownGrace = 5 * time.Second

// cancelledByShutdown reports whether err is nothing more than our own
// cancellation, so background work stopped on purpose isn't reported as a
// failure — it would otherwise log at ERROR, with a stack trace, on every
// restart.
func cancelledByShutdown(ctx context.Context, err error) bool {
	return ctx.Err() != nil && errors.Is(err, context.Canceled)
}

type SubscriptionService struct {
	feeds   feedStore
	subs    subscriptionStore
	feedSvc feedImporter
	logger  *jsonlog.Logger

	// Article imports run detached from the request that started them. ctx is
	// cancelled by Shutdown and wg tracks the goroutines, so a SIGTERM stops
	// them deliberately instead of cutting them off mid-write.
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
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
	// FolderID files the subscription, nil leaving it unfiled.
	FolderID *int64

	// DeferImport skips the per-feed background import. An OPML file holds
	// hundreds of feeds, and firing one import per entry would have hundreds of
	// goroutines fetching and scraping whole feeds at once; the importer marks
	// the feeds due instead and wakes the scheduler once, so the existing worker
	// pool and per-domain rate limiting do the work.
	DeferImport bool
}

// Subscribe signs userID up to the feed at feedURL, creating the feed itself if
// nobody follows it yet, and kicks off the article import in the background.
//
// It returns ErrAlreadySubscribed when the user already follows the feed, and
// propagates ErrInvalidFeed / ErrFeedNotFound from feed creation.
func (s *SubscriptionService) Subscribe(ctx context.Context, userID uuid.UUID, feedURL string, opts SubscribeOptions) (*data.Subscription, error) {
	feed, err := s.feeds.GetByURL(ctx, feedURL)
	if err != nil && !errors.Is(err, data.ErrRecordNotFound) {
		return nil, err
	}

	// Créer le feed demande un fetch HTTP, il ne peut donc pas partager une
	// transaction avec l'insert de la souscription ci-dessous ; on retient qu'on
	// l'a créé pour pouvoir le reprendre si la suite échoue.
	feedCreated := feed == nil
	subscribed := false

	// Toute sortie avant l'insert de la souscription laisserait derrière elle un
	// feed sans abonné, que plus rien ne référence ni ne nettoie. Le garde
	// NOT EXISTS de DeleteIfOrphan rend l'appel sûr même si un autre utilisateur
	// vient de s'abonner à la même URL. Contexte détaché : le cas le plus
	// probable est justement un client parti en cours de route.
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

	// The caller renders the subscription it just created, so hand back the feed
	// rather than make it fetch the list again. UnreadCount stays at zero, which
	// is accurate: the import below hasn't produced anything yet.
	subscription.Feed = *feed

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
//
// It cancels rather than waits: an import is bounded by feedProcessTimeout
// (eight minutes) while the server's own shutdown is bounded at twenty seconds,
// so waiting one out would simply hold the process open until it is killed. A
// feed left half-imported is picked up by the scheduler's next tick.
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
