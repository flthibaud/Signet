package service

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"

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

	// markedDue records every MarkDueForSync call, which is how the tests see
	// whether a new subscriber will be served a full fetch rather than a 304.
	markedDue [][]int64
	markErr   error
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

func (f *fakeFeedStore) MarkDueForSync(ctx context.Context, ids []int64) error {
	f.markedDue = append(f.markedDue, ids)
	return f.markErr
}

type fakeSubscriptionStore struct {
	insertErr error

	inserted []*data.Subscription
}

func (f *fakeSubscriptionStore) Insert(ctx context.Context, sub *data.Subscription) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	sub.ID = int64(len(f.inserted) + 1)
	f.inserted = append(f.inserted, sub)
	return nil
}

type fakeFeedCreator struct {
	created    *data.Feed
	createErr  error
	createCall int
}

func (f *fakeFeedCreator) CreateFromURL(ctx context.Context, url string) (*data.Feed, error) {
	f.createCall++
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.created, nil
}

type fakeSyncTrigger struct {
	mu    sync.Mutex
	calls int
}

func (f *fakeSyncTrigger) TriggerSync() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
}

// newTestSubscriptionService builds the service directly rather than through
// NewSubscriptionService, which insists on the concrete data.Models.
func newTestSubscriptionService(feeds feedStore, subs subscriptionStore, creator feedCreator) (*SubscriptionService, *fakeSyncTrigger) {
	trigger := &fakeSyncTrigger{}

	return &SubscriptionService{
		feeds:   feeds,
		subs:    subs,
		feedSvc: creator,
		trigger: trigger,
		logger:  jsonlog.New(&lockedBuffer{}, jsonlog.LevelInfo),
	}, trigger
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

// assertHandedToScheduler checks the one thing a successful subscribe owes the
// new subscriber: the feed is due and the scheduler is awake, since the next
// sync is what brings their articles in.
func assertHandedToScheduler(t *testing.T, feeds *fakeFeedStore, trigger *fakeSyncTrigger, feedID int64) {
	t.Helper()

	if len(feeds.markedDue) != 1 || len(feeds.markedDue[0]) != 1 || feeds.markedDue[0][0] != feedID {
		t.Errorf("marked due: got %v, want just feed %d", feeds.markedDue, feedID)
	}
	if trigger.calls != 1 {
		t.Errorf("TriggerSync calls: got %d, want 1", trigger.calls)
	}
}

func TestSubscribeCreatesMissingFeed(t *testing.T) {
	newFeed := &data.Feed{ID: 42, Url: "https://example.com/feed.xml"}

	feeds := &fakeFeedStore{} // nothing in the database
	subs := &fakeSubscriptionStore{}
	creator := &fakeFeedCreator{created: newFeed}

	svc, trigger := newTestSubscriptionService(feeds, subs, creator)
	userID := uuid.New()

	sub, err := svc.Subscribe(context.Background(), userID, newFeed.Url, SubscribeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creator.createCall != 1 {
		t.Errorf("CreateFromURL calls: got %d, want 1", creator.createCall)
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

	// A feed just created carries a fresh last_fetched_at, so without the mark
	// it would wait a whole interval for its first sync.
	assertHandedToScheduler(t, feeds, trigger, newFeed.ID)
}

func TestSubscribeReusesExistingFeed(t *testing.T) {
	existing := &data.Feed{ID: 7, Url: "https://example.com/feed.xml"}

	feeds := &fakeFeedStore{feed: existing}
	subs := &fakeSubscriptionStore{}
	creator := &fakeFeedCreator{}

	svc, trigger := newTestSubscriptionService(feeds, subs, creator)

	sub, err := svc.Subscribe(context.Background(), uuid.New(), existing.Url, SubscribeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if creator.createCall != 0 {
		t.Errorf("a feed already on file must not be re-created, got %d calls", creator.createCall)
	}
	if sub.FeedID != existing.ID {
		t.Errorf("feed id: got %d, want %d", sub.FeedID, existing.ID)
	}

	assertHandedToScheduler(t, feeds, trigger, existing.ID)
}

func TestSubscribeFilesIntoFolder(t *testing.T) {
	folderID := int64(3)

	feeds := &fakeFeedStore{}
	subs := &fakeSubscriptionStore{}
	creator := &fakeFeedCreator{created: &data.Feed{ID: 42}}

	svc, _ := newTestSubscriptionService(feeds, subs, creator)

	sub, err := svc.Subscribe(context.Background(), uuid.New(), "https://example.com/feed.xml", SubscribeOptions{
		FolderID: &folderID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if sub.FolderID == nil || *sub.FolderID != folderID {
		t.Errorf("folder: got %v, want %d", sub.FolderID, folderID)
	}
	if len(subs.inserted) != 1 {
		t.Fatalf("inserted subscriptions: got %d, want 1", len(subs.inserted))
	}
	if subs.inserted[0].FolderID == nil || *subs.inserted[0].FolderID != folderID {
		t.Errorf("the folder was not persisted: %+v", subs.inserted[0])
	}
}

// The subscription is what the user asked for and it exists, so failing to
// mark the feed due is logged rather than reported: the feed still syncs, a
// tick later than it could have.
func TestSubscribeSurvivesMarkFailure(t *testing.T) {
	feeds := &fakeFeedStore{feed: &data.Feed{ID: 7}, markErr: errors.New("database blip")}
	subs := &fakeSubscriptionStore{}

	svc, trigger := newTestSubscriptionService(feeds, subs, &fakeFeedCreator{})

	if _, err := svc.Subscribe(context.Background(), uuid.New(), "https://example.com/feed.xml", SubscribeOptions{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(subs.inserted) != 1 {
		t.Fatalf("inserted subscriptions: got %d, want 1", len(subs.inserted))
	}
	if trigger.calls != 0 {
		t.Errorf("a feed that is not due gives the scheduler nothing to do, got %d triggers", trigger.calls)
	}
}

// The loser of two concurrent subscribes is decided by the unique constraint,
// not by a prior lookup: Insert fails with data.ErrDuplicateSubscription and
// Subscribe reports it as ErrAlreadySubscribed.
func TestSubscribeWhenAlreadySubscribed(t *testing.T) {
	existing := &data.Feed{ID: 7}

	feeds := &fakeFeedStore{feed: existing}
	subs := &fakeSubscriptionStore{insertErr: data.ErrDuplicateSubscription}

	svc, trigger := newTestSubscriptionService(feeds, subs, &fakeFeedCreator{})

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
	// Marking would clear the ETag and force a full re-read of the feed for a
	// subscriber who already has its articles.
	if len(feeds.markedDue) != 0 || trigger.calls != 0 {
		t.Errorf("a refused subscribe must leave the feed alone, got marks %v and %d triggers", feeds.markedDue, trigger.calls)
	}
}

// A feed created for a subscription that then fails to insert has no subscriber
// and nothing else referencing it, so it must be cleaned up.
func TestSubscribeCleansUpFeedItCreated(t *testing.T) {
	newFeed := &data.Feed{ID: 42}

	feeds := &fakeFeedStore{}
	subs := &fakeSubscriptionStore{insertErr: errors.New("insert failed")}

	svc, _ := newTestSubscriptionService(feeds, subs, &fakeFeedCreator{created: newFeed})

	_, err := svc.Subscribe(context.Background(), uuid.New(), "https://example.com/feed.xml", SubscribeOptions{})
	if err == nil {
		t.Fatal("expected an error")
	}

	if len(feeds.deletedIDs) != 1 || feeds.deletedIDs[0] != newFeed.ID {
		t.Errorf("expected cleanup of feed %d, got %v", newFeed.ID, feeds.deletedIDs)
	}
}

// The mirror image, and the one that matters: a feed that was already on file
// belongs to its other subscribers. A failed insert must not touch it.
func TestSubscribeLeavesPreexistingFeedAloneOnFailure(t *testing.T) {
	existing := &data.Feed{ID: 7}

	feeds := &fakeFeedStore{feed: existing}
	subs := &fakeSubscriptionStore{insertErr: errors.New("insert failed")}

	svc, _ := newTestSubscriptionService(feeds, subs, &fakeFeedCreator{})

	_, err := svc.Subscribe(context.Background(), uuid.New(), "https://example.com/feed.xml", SubscribeOptions{})
	if err == nil {
		t.Fatal("expected an error")
	}

	if len(feeds.deletedIDs) != 0 {
		t.Errorf("a pre-existing feed must never be deleted, got %v", feeds.deletedIDs)
	}
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

			svc, _ := newTestSubscriptionService(feeds, subs, &fakeFeedCreator{createErr: tt.err})

			_, err := svc.Subscribe(context.Background(), uuid.New(), "https://example.com/feed.xml", SubscribeOptions{})
			if !errors.Is(err, tt.err) {
				t.Fatalf("got %v, want %v", err, tt.err)
			}
			// Feed creation failed, so there is no feed to compensate for.
			if len(feeds.deletedIDs) != 0 {
				t.Errorf("no feed was created, nothing to clean up, got %v", feeds.deletedIDs)
			}
		})
	}
}

func TestSubscribeReportsLookupFailure(t *testing.T) {
	lookupErr := errors.New("database down")

	feeds := &fakeFeedStore{getErr: lookupErr}
	creator := &fakeFeedCreator{}

	svc, _ := newTestSubscriptionService(feeds, &fakeSubscriptionStore{}, creator)

	_, err := svc.Subscribe(context.Background(), uuid.New(), "https://example.com/feed.xml", SubscribeOptions{})
	if !errors.Is(err, lookupErr) {
		t.Fatalf("got %v, want %v", err, lookupErr)
	}
	if creator.createCall != 0 {
		t.Error("a failed lookup must not lead to a feed being created")
	}
}
