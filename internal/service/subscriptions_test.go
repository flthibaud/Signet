package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
	"github.com/google/uuid"
)

// --- fakes ---------------------------------------------------------------

type fakeFeedStore struct {
	// feed is what GetByURL returns; nil means "not in the database yet".
	feed   *data.Feed
	getErr error

	deleteErr error

	// deletedIDs records every DeleteIfOrphan call, which is how the tests tell
	// a compensated feed from one that was left alone.
	deletedIDs []int64
}

func (f *fakeFeedStore) GetByURL(ctx context.Context, url string) (*data.Feed, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.feed == nil {
		return nil, data.ErrRecordNotFound
	}
	return f.feed, nil
}

func (f *fakeFeedStore) DeleteIfOrphan(ctx context.Context, id int64) error {
	f.deletedIDs = append(f.deletedIDs, id)
	return f.deleteErr
}

type fakeSubscriptionStore struct {
	exists    bool
	existsErr error
	insertErr error

	inserted []*data.Subscription
}

func (f *fakeSubscriptionStore) Exists(ctx context.Context, userID uuid.UUID, feedID int64) (bool, error) {
	return f.exists, f.existsErr
}

func (f *fakeSubscriptionStore) Insert(ctx context.Context, sub *data.Subscription) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	sub.ID = int64(len(f.inserted) + 1)
	f.inserted = append(f.inserted, sub)
	return nil
}

type fakeFeedImporter struct {
	created    *data.Feed
	createErr  error
	createCall int

	// imported is closed by ImportArticlesForSubscribers so a test can wait for
	// the background goroutine instead of sleeping.
	imported   chan struct{}
	importHold bool // block until the context is cancelled
}

func (f *fakeFeedImporter) CreateFromURL(ctx context.Context, url string) (*data.Feed, error) {
	f.createCall++
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.created, nil
}

func (f *fakeFeedImporter) ImportArticlesForSubscribers(ctx context.Context, feed *data.Feed) error {
	if f.imported != nil {
		close(f.imported)
	}
	if f.importHold {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

// newTestSubscriptionService builds the service directly rather than through
// NewSubscriptionService, which insists on the concrete data.Models.
func newTestSubscriptionService(feeds feedStore, subs subscriptionStore, importer feedImporter) *SubscriptionService {
	svc, _ := newTestSubscriptionServiceWithLog(feeds, subs, importer)
	return svc
}

// newTestSubscriptionServiceWithLog also hands back the buffer the service logs
// into, for the tests that care about what was reported.
func newTestSubscriptionServiceWithLog(feeds feedStore, subs subscriptionStore, importer feedImporter) (*SubscriptionService, *lockedBuffer) {
	ctx, cancel := context.WithCancel(context.Background())
	out := &lockedBuffer{}

	return &SubscriptionService{
		feeds:   feeds,
		subs:    subs,
		feedSvc: importer,
		logger:  jsonlog.New(out, jsonlog.LevelInfo),
		ctx:     ctx,
		cancel:  cancel,
	}, out
}

// lockedBuffer is a bytes.Buffer safe to read while a background import is
// still writing to it.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// --- tests ---------------------------------------------------------------

func TestSubscribeCreatesMissingFeed(t *testing.T) {
	newFeed := &data.Feed{ID: 42, Url: "https://example.com/feed.xml"}

	feeds := &fakeFeedStore{} // nothing in the database
	subs := &fakeSubscriptionStore{}
	importer := &fakeFeedImporter{created: newFeed, imported: make(chan struct{})}

	svc := newTestSubscriptionService(feeds, subs, importer)
	userID := uuid.New()

	sub, err := svc.Subscribe(context.Background(), userID, newFeed.Url, SubscribeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if importer.createCall != 1 {
		t.Errorf("CreateFromURL calls: got %d, want 1", importer.createCall)
	}
	if len(subs.inserted) != 1 {
		t.Fatalf("inserted subscriptions: got %d, want 1", len(subs.inserted))
	}
	if sub.FeedID != newFeed.ID || sub.UserID != userID {
		t.Errorf("subscription: got feed %d / user %v, want %d / %v", sub.FeedID, sub.UserID, newFeed.ID, userID)
	}
	// The caller renders what it just created, so the feed travels back with it
	// rather than costing a second round trip.
	if sub.Feed.ID != newFeed.ID || sub.Feed.Url != newFeed.Url {
		t.Errorf("embedded feed: got %+v, want id %d / url %q", sub.Feed, newFeed.ID, newFeed.Url)
	}
	if len(feeds.deletedIDs) != 0 {
		t.Errorf("nothing failed, so no feed should have been cleaned up: %v", feeds.deletedIDs)
	}

	// The import is scheduled, not awaited.
	select {
	case <-importer.imported:
	case <-time.After(2 * time.Second):
		t.Error("the article import was never started")
	}
	svc.Shutdown()
}

func TestSubscribeReusesExistingFeed(t *testing.T) {
	existing := &data.Feed{ID: 7, Url: "https://example.com/feed.xml"}

	feeds := &fakeFeedStore{feed: existing}
	subs := &fakeSubscriptionStore{}
	importer := &fakeFeedImporter{}

	svc := newTestSubscriptionService(feeds, subs, importer)

	sub, err := svc.Subscribe(context.Background(), uuid.New(), existing.Url, SubscribeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if importer.createCall != 0 {
		t.Errorf("a feed already on file must not be re-created, got %d calls", importer.createCall)
	}
	if sub.FeedID != existing.ID {
		t.Errorf("feed id: got %d, want %d", sub.FeedID, existing.ID)
	}

	svc.Shutdown()
}

// DeferImport is what keeps an OPML file of 200 feeds from starting 200
// concurrent imports.
func TestSubscribeDeferredImport(t *testing.T) {
	folderID := int64(3)

	feeds := &fakeFeedStore{}
	subs := &fakeSubscriptionStore{}
	importer := &fakeFeedImporter{created: &data.Feed{ID: 42}, imported: make(chan struct{})}

	svc := newTestSubscriptionService(feeds, subs, importer)

	sub, err := svc.Subscribe(context.Background(), uuid.New(), "https://example.com/feed.xml", SubscribeOptions{
		FolderID:    &folderID,
		DeferImport: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sub.FolderID == nil || *sub.FolderID != folderID {
		t.Errorf("folder: got %v, want %d", sub.FolderID, folderID)
	}

	// Shutdown waits for anything that was started, so once it returns the
	// channel is a reliable witness.
	svc.Shutdown()

	select {
	case <-importer.imported:
		t.Error("DeferImport must not start the per-feed article import")
	default:
	}

	if len(subs.inserted) != 1 {
		t.Fatalf("inserted subscriptions: got %d, want 1", len(subs.inserted))
	}
	if subs.inserted[0].FolderID == nil || *subs.inserted[0].FolderID != folderID {
		t.Errorf("the folder was not persisted: %+v", subs.inserted[0])
	}
}

func TestSubscribeWhenAlreadySubscribed(t *testing.T) {
	existing := &data.Feed{ID: 7}

	feeds := &fakeFeedStore{feed: existing}
	subs := &fakeSubscriptionStore{exists: true}
	importer := &fakeFeedImporter{}

	svc := newTestSubscriptionService(feeds, subs, importer)

	_, err := svc.Subscribe(context.Background(), uuid.New(), "https://example.com/feed.xml", SubscribeOptions{})
	if !errors.Is(err, ErrAlreadySubscribed) {
		t.Fatalf("got %v, want ErrAlreadySubscribed", err)
	}
	if len(subs.inserted) != 0 {
		t.Errorf("nothing should have been inserted, got %d", len(subs.inserted))
	}
	if len(feeds.deletedIDs) != 0 {
		t.Errorf("the feed pre-existed and must survive, got deletes for %v", feeds.deletedIDs)
	}
	svc.Shutdown()
}

// A feed created for a subscription that then fails to insert has no subscriber
// and nothing else referencing it, so it must be cleaned up.
func TestSubscribeCleansUpFeedItCreated(t *testing.T) {
	newFeed := &data.Feed{ID: 42}

	feeds := &fakeFeedStore{}
	subs := &fakeSubscriptionStore{insertErr: errors.New("insert failed")}
	importer := &fakeFeedImporter{created: newFeed}

	svc := newTestSubscriptionService(feeds, subs, importer)

	_, err := svc.Subscribe(context.Background(), uuid.New(), "https://example.com/feed.xml", SubscribeOptions{})
	if err == nil {
		t.Fatal("expected an error")
	}

	if len(feeds.deletedIDs) != 1 || feeds.deletedIDs[0] != newFeed.ID {
		t.Errorf("expected cleanup of feed %d, got %v", newFeed.ID, feeds.deletedIDs)
	}
	svc.Shutdown()
}

// The mirror image, and the one that matters: a feed that was already on file
// belongs to its other subscribers. A failed insert must not touch it.
func TestSubscribeLeavesPreexistingFeedAloneOnFailure(t *testing.T) {
	existing := &data.Feed{ID: 7}

	feeds := &fakeFeedStore{feed: existing}
	subs := &fakeSubscriptionStore{insertErr: errors.New("insert failed")}
	importer := &fakeFeedImporter{}

	svc := newTestSubscriptionService(feeds, subs, importer)

	_, err := svc.Subscribe(context.Background(), uuid.New(), "https://example.com/feed.xml", SubscribeOptions{})
	if err == nil {
		t.Fatal("expected an error")
	}

	if len(feeds.deletedIDs) != 0 {
		t.Errorf("a pre-existing feed must never be deleted, got %v", feeds.deletedIDs)
	}
	svc.Shutdown()
}

func TestSubscribePropagatesFeedErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"invalid feed", ErrInvalidFeed},
		{"unreachable feed", ErrFeedNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			feeds := &fakeFeedStore{}
			subs := &fakeSubscriptionStore{}
			importer := &fakeFeedImporter{createErr: tt.err}

			svc := newTestSubscriptionService(feeds, subs, importer)

			_, err := svc.Subscribe(context.Background(), uuid.New(), "https://example.com/feed.xml", SubscribeOptions{})
			if !errors.Is(err, tt.err) {
				t.Fatalf("got %v, want %v", err, tt.err)
			}
			// Feed creation failed, so there is no feed to compensate for.
			if len(feeds.deletedIDs) != 0 {
				t.Errorf("no feed was created, nothing to clean up, got %v", feeds.deletedIDs)
			}
			svc.Shutdown()
		})
	}
}

func TestSubscribeReportsLookupFailure(t *testing.T) {
	lookupErr := errors.New("database down")

	feeds := &fakeFeedStore{getErr: lookupErr}
	subs := &fakeSubscriptionStore{}
	importer := &fakeFeedImporter{}

	svc := newTestSubscriptionService(feeds, subs, importer)

	_, err := svc.Subscribe(context.Background(), uuid.New(), "https://example.com/feed.xml", SubscribeOptions{})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("got %v, want %v", err, lookupErr)
	}
	if importer.createCall != 0 {
		t.Error("a failed lookup must not lead to a feed being created")
	}
	svc.Shutdown()
}

// Shutdown has to cancel an import that is still running and come back, rather
// than let the goroutine be cut off at process exit.
func TestShutdownCancelsInFlightImport(t *testing.T) {
	feeds := &fakeFeedStore{}
	subs := &fakeSubscriptionStore{}
	importer := &fakeFeedImporter{
		created:    &data.Feed{ID: 42},
		imported:   make(chan struct{}),
		importHold: true,
	}

	svc, logged := newTestSubscriptionServiceWithLog(feeds, subs, importer)

	if _, err := svc.Subscribe(context.Background(), uuid.New(), "https://example.com/feed.xml", SubscribeOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait until the import is actually running before shutting down.
	select {
	case <-importer.imported:
	case <-time.After(2 * time.Second):
		t.Fatal("the article import was never started")
	}

	done := make(chan struct{})
	go func() {
		svc.Shutdown()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(shutdownGrace + 2*time.Second):
		t.Fatal("Shutdown did not return")
	}

	// Cancelling on purpose is not a failure: reporting it at ERROR would put a
	// stack trace in the logs on every restart.
	if out := logged.String(); strings.Contains(out, `"level":"ERROR"`) {
		t.Errorf("a cancelled import must not be logged as an error, got: %s", out)
	}
}
