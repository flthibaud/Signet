package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
	"github.com/flthibaud/signet/internal/opml"
	"github.com/google/uuid"
)

// --- fakes ---------------------------------------------------------------

// fakeSubscriber answers per URL, so one test can mix a success, an existing
// subscription and a dead feed.
type fakeSubscriber struct {
	mu       sync.Mutex
	byURL    map[string]error
	feedIDs  map[string]int64
	calls    []string
	lastOpts SubscribeOptions
}

func (f *fakeSubscriber) Subscribe(ctx context.Context, userID uuid.UUID, feedURL string, opts SubscribeOptions) (*data.Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls = append(f.calls, feedURL)
	f.lastOpts = opts

	if err := f.byURL[feedURL]; err != nil {
		return nil, err
	}

	return &data.Subscription{FeedID: f.feedIDs[feedURL], FolderID: opts.FolderID}, nil
}

type fakeFolderStore struct {
	mu      sync.Mutex
	created []string
	nextID  int64
	err     error
}

func (f *fakeFolderStore) GetOrCreate(ctx context.Context, userID uuid.UUID, name string) (*data.Folder, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.err != nil {
		return nil, f.err
	}

	f.created = append(f.created, name)
	f.nextID++

	return &data.Folder{ID: f.nextID, Name: name}, nil
}

type fakeImportStore struct {
	mu        sync.Mutex
	imports   []*data.OPMLImport
	results   []data.OPMLImportResult
	finished  string
	insertErr error

	// done is closed when the job closes, so a test can wait for the worker
	// rather than call Shutdown — which cancels the import instead of draining
	// it, exactly as a SIGTERM would.
	done      chan struct{}
	closeOnce sync.Once
}

func newFakeImportStore() *fakeImportStore {
	return &fakeImportStore{done: make(chan struct{})}
}

func (f *fakeImportStore) Insert(ctx context.Context, imp *data.OPMLImport) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	imp.ID = uuid.New()
	imp.Status = data.OPMLImportRunning
	f.imports = append(f.imports, imp)
	return nil
}

// wait blocks until the job closes, failing the test rather than hanging.
func (f *fakeImportStore) wait(t *testing.T) {
	t.Helper()

	select {
	case <-f.done:
	case <-time.After(2 * time.Second):
		t.Fatal("the import never finished")
	}
}

func (f *fakeImportStore) AppendResult(ctx context.Context, id uuid.UUID, result data.OPMLImportResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.results = append(f.results, result)
	return nil
}

func (f *fakeImportStore) MarkFinished(ctx context.Context, id uuid.UUID, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finished = status
	f.closeOnce.Do(func() { close(f.done) })
	return nil
}

func (f *fakeImportStore) counts() (imported, skipped, failed int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	for _, r := range f.results {
		switch r.Status {
		case data.OPMLEntryImported:
			imported++
		case data.OPMLEntrySkipped:
			skipped++
		case data.OPMLEntryFailed:
			failed++
		}
	}
	return
}

type fakeFeedMarker struct {
	mu     sync.Mutex
	marked []int64
}

func (f *fakeFeedMarker) MarkDueForSync(ctx context.Context, ids []int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.marked = append(f.marked, ids...)
	return nil
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

func newTestOPMLService(subs subscriber, folders folderStore, imports importStore, feeds feedMarker) *OPMLService {
	ctx, cancel := context.WithCancel(context.Background())

	return &OPMLService{
		subs:    subs,
		folders: folders,
		imports: imports,
		feeds:   feeds,
		logger:  jsonlog.New(&lockedBuffer{}, jsonlog.LevelInfo),
		ctx:     ctx,
		cancel:  cancel,
	}
}

// --- tests ---------------------------------------------------------------

// TestImportMixedOutcomes is the case that matters: one bad line in a file of
// forty must not cost the other thirty-nine.
func TestImportMixedOutcomes(t *testing.T) {
	entries := []opml.Entry{
		{Title: "Korben", XMLURL: "https://korben.info/feed", Folder: "Tech"},
		{Title: "Already there", XMLURL: "https://jvns.ca/atom.xml", Folder: "Tech"},
		{Title: "Dead", XMLURL: "https://dead.test/rss"},
		{Title: "Not a feed", XMLURL: "https://example.test/index.html"},
		{Title: "Boom", XMLURL: "https://broken.test/rss"},
	}

	subs := &fakeSubscriber{
		byURL: map[string]error{
			"https://jvns.ca/atom.xml":        ErrAlreadySubscribed,
			"https://dead.test/rss":           ErrFeedNotFound,
			"https://example.test/index.html": ErrInvalidFeed,
			"https://broken.test/rss":         errors.New("database exploded"),
		},
		feedIDs: map[string]int64{"https://korben.info/feed": 42},
	}
	folders := &fakeFolderStore{}
	imports := newFakeImportStore()
	feeds := &fakeFeedMarker{}
	trigger := &fakeSyncTrigger{}

	svc := newTestOPMLService(subs, folders, imports, feeds)
	svc.SetSyncTrigger(trigger)

	imp, err := svc.StartImport(context.Background(), uuid.New(), entries)
	if err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	if imp.Total != len(entries) {
		t.Errorf("total = %d, want %d", imp.Total, len(entries))
	}

	imports.wait(t)
	svc.Shutdown()

	imported, skipped, failed := imports.counts()
	if imported != 1 || skipped != 1 || failed != 3 {
		t.Errorf("counts = imported %d / skipped %d / failed %d, want 1/1/3", imported, skipped, failed)
	}
	if len(imports.results) != len(entries) {
		t.Errorf("got %d report lines, want one per entry (%d)", len(imports.results), len(entries))
	}
	if imports.finished != data.OPMLImportCompleted {
		t.Errorf("finished with %q, want %q", imports.finished, data.OPMLImportCompleted)
	}

	// Every line was attempted despite the failures.
	if len(subs.calls) != len(entries) {
		t.Errorf("subscribed to %d feeds, want %d", len(subs.calls), len(entries))
	}

	// The folder was created once for the two entries that share it.
	if len(folders.created) != 1 || folders.created[0] != "Tech" {
		t.Errorf("folders created: %v, want one \"Tech\"", folders.created)
	}

	// Only feeds that actually got subscribed are handed to the scheduler.
	if len(feeds.marked) != 1 || feeds.marked[0] != 42 {
		t.Errorf("marked %v due for sync, want [42]", feeds.marked)
	}
	if trigger.calls != 1 {
		t.Errorf("TriggerSync calls = %d, want 1", trigger.calls)
	}
}

// The per-feed import must stay off: it is the whole reason the scheduler is
// nudged once at the end instead.
func TestImportDefersArticleImports(t *testing.T) {
	subs := &fakeSubscriber{feedIDs: map[string]int64{"https://korben.info/feed": 1}}

	imports := newFakeImportStore()

	svc := newTestOPMLService(subs, &fakeFolderStore{}, imports, &fakeFeedMarker{})

	_, err := svc.StartImport(context.Background(), uuid.New(), []opml.Entry{
		{XMLURL: "https://korben.info/feed"},
	})
	if err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	imports.wait(t)
	svc.Shutdown()

	if !subs.lastOpts.DeferImport {
		t.Error("entries must be subscribed with DeferImport set")
	}
}

// A folder that cannot be created costs the feed its filing, not its import.
func TestImportSurvivesFolderFailure(t *testing.T) {
	subs := &fakeSubscriber{feedIDs: map[string]int64{"https://korben.info/feed": 1}}
	folders := &fakeFolderStore{err: errors.New("folders table is on fire")}
	imports := newFakeImportStore()

	svc := newTestOPMLService(subs, folders, imports, &fakeFeedMarker{})

	_, err := svc.StartImport(context.Background(), uuid.New(), []opml.Entry{
		{XMLURL: "https://korben.info/feed", Folder: "Tech"},
	})
	if err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	imports.wait(t)
	svc.Shutdown()

	imported, _, _ := imports.counts()
	if imported != 1 {
		t.Errorf("imported = %d, want the feed to land unfiled rather than not at all", imported)
	}
	if subs.lastOpts.FolderID != nil {
		t.Errorf("folder id = %v, want nil", subs.lastOpts.FolderID)
	}
}

// Nothing to sync means nothing to wake the scheduler for.
func TestImportWithoutSuccessDoesNotTrigger(t *testing.T) {
	subs := &fakeSubscriber{byURL: map[string]error{"https://dead.test/rss": ErrFeedNotFound}}
	trigger := &fakeSyncTrigger{}

	imports := newFakeImportStore()

	svc := newTestOPMLService(subs, &fakeFolderStore{}, imports, &fakeFeedMarker{})
	svc.SetSyncTrigger(trigger)

	if _, err := svc.StartImport(context.Background(), uuid.New(), []opml.Entry{
		{XMLURL: "https://dead.test/rss"},
	}); err != nil {
		t.Fatalf("StartImport: %v", err)
	}
	imports.wait(t)
	svc.Shutdown()

	if trigger.calls != 0 {
		t.Errorf("TriggerSync calls = %d, want none", trigger.calls)
	}
}

// Shutdown in the middle of an import must close the job as interrupted and
// stay quiet about it: cancelling on purpose is not a failure, and logging it
// at ERROR would put one stack trace per worker in the logs on every restart.
func TestImportInterruptedByShutdown(t *testing.T) {
	release := make(chan struct{})
	subs := &blockingSubscriber{started: make(chan struct{}, 1), release: release}
	imports := newFakeImportStore()

	svc := newTestOPMLService(subs, &fakeFolderStore{}, imports, &fakeFeedMarker{})
	logged := &lockedBuffer{}
	svc.logger = jsonlog.New(logged, jsonlog.LevelInfo)

	entries := make([]opml.Entry, 10)
	for i := range entries {
		entries[i] = opml.Entry{XMLURL: fmt.Sprintf("https://feed-%d.test/rss", i)}
	}

	if _, err := svc.StartImport(context.Background(), uuid.New(), entries); err != nil {
		t.Fatalf("StartImport: %v", err)
	}

	// Wait until a worker is actually inside Subscribe before pulling the rug.
	select {
	case <-subs.started:
	case <-time.After(2 * time.Second):
		t.Fatal("no worker ever started")
	}

	svc.Shutdown()
	close(release)
	imports.wait(t)

	if imports.finished != data.OPMLImportInterrupted {
		t.Errorf("finished with %q, want %q", imports.finished, data.OPMLImportInterrupted)
	}
	// Entries the shutdown cut short are not recorded as failures.
	if _, _, failed := imports.counts(); failed != 0 {
		t.Errorf("%d entries recorded as failed, want none", failed)
	}
	if out := logged.String(); strings.Contains(out, `"level":"ERROR"`) {
		t.Errorf("a cancelled import must not log at ERROR, got: %s", out)
	}
}

// blockingSubscriber holds each call until released, so a test can be sure the
// import is genuinely in flight.
type blockingSubscriber struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingSubscriber) Subscribe(ctx context.Context, userID uuid.UUID, feedURL string, opts SubscribeOptions) (*data.Subscription, error) {
	b.once.Do(func() { b.started <- struct{}{} })

	select {
	case <-b.release:
		return &data.Subscription{FeedID: 1}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestStartImportPropagatesInsertFailure(t *testing.T) {
	insertErr := errors.New("database down")
	imports := &fakeImportStore{insertErr: insertErr}

	svc := newTestOPMLService(&fakeSubscriber{}, &fakeFolderStore{}, imports, &fakeFeedMarker{})

	if _, err := svc.StartImport(context.Background(), uuid.New(), []opml.Entry{{XMLURL: "https://a.test/feed"}}); !errors.Is(err, insertErr) {
		t.Fatalf("got %v, want %v", err, insertErr)
	}
	svc.Shutdown()
}
