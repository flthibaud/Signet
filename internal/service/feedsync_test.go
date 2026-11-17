package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
	"github.com/google/uuid"
)

// --- fakes ---------------------------------------------------------------

type releaseCall struct{ etag, lastModified string }

type fakeSyncFeedStore struct {
	mu sync.Mutex

	// failures is what MarkFeedFailed reports; zero means a first failure.
	failures int

	released    []releaseCall
	failedCalls int
	unclaimed   int

	claimArgs []time.Duration // dueAfter, lockWindow, deadRetry of the last claim
}

func (f *fakeSyncFeedStore) Insert(ctx context.Context, feed *data.Feed) error { return nil }

func (f *fakeSyncFeedStore) GetFeedsToSync(ctx context.Context, batchSize int, dueAfter, lockWindow, deadRetry time.Duration) ([]*data.Feed, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claimArgs = []time.Duration{dueAfter, lockWindow, deadRetry}
	return nil, nil
}

func (f *fakeSyncFeedStore) ReleaseFeed(ctx context.Context, feedID int64, etag, lastModified string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.released = append(f.released, releaseCall{etag, lastModified})
	return nil
}

func (f *fakeSyncFeedStore) MarkFeedFailed(ctx context.Context, feedID int64, threshold int) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failedCalls++
	if f.failures == 0 {
		return 1, nil
	}
	return f.failures, nil
}

func (f *fakeSyncFeedStore) Unclaim(ctx context.Context, feedID int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unclaimed++
	return nil
}

type fakeArticleStore struct {
	mu        sync.Mutex
	byHash    map[string]int64
	insertErr map[string]error // by article URL
	inserted  []*data.Article
}

func (f *fakeArticleStore) IDByHash(ctx context.Context, hash string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id, ok := f.byHash[hash]; ok {
		return id, nil
	}
	return 0, data.ErrRecordNotFound
}

func (f *fakeArticleStore) Insert(ctx context.Context, article *data.Article) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.insertErr[article.Url]; err != nil {
		return err
	}
	article.ID = int64(100 + len(f.inserted))
	f.inserted = append(f.inserted, article)
	return nil
}

type fakeLinkStore struct {
	mu       sync.Mutex
	articles []int64 // article id of every BulkInsertForArticle call
}

func (f *fakeLinkStore) BulkInsertForArticle(ctx context.Context, userIDs []uuid.UUID, articleID, feedID int64, baseSlug string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.articles = append(f.articles, articleID)
	return nil
}

type fakeSubscriberStore struct{ ids []uuid.UUID }

func (f *fakeSubscriberStore) GetSubscriberIDs(ctx context.Context, feedID int64) ([]uuid.UUID, error) {
	return f.ids, nil
}

type syncFakes struct {
	feeds    *fakeSyncFeedStore
	articles *fakeArticleStore
	links    *fakeLinkStore
	logged   *lockedBuffer
}

// newSyncTestService builds a real FeedService, private networks allowed so it
// can reach httptest servers, with every store replaced by a fake.
func newSyncTestService(t *testing.T) (*FeedService, *syncFakes) {
	t.Helper()

	f := &syncFakes{
		feeds:    &fakeSyncFeedStore{},
		articles: &fakeArticleStore{byHash: map[string]int64{}, insertErr: map[string]error{}},
		links:    &fakeLinkStore{},
		logged:   &lockedBuffer{},
	}

	s := NewFeedService(data.Models{}, jsonlog.New(f.logged, jsonlog.LevelInfo), FetchConfig{AllowPrivateNetworks: true})
	s.feeds = f.feeds
	s.articles = f.articles
	s.links = f.links
	s.subscribers = &fakeSubscriberStore{ids: []uuid.UUID{uuid.New()}}

	return s, f
}

// rss renders a feed whose items link to the given URLs.
func rss(links ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><rss version="2.0"><channel><title>t</title>`)
	for i, l := range links {
		fmt.Fprintf(&b, `<item><title>Item %d</title><link>%s</link><description>Body %d</description></item>`, i, l, i)
	}
	b.WriteString(`</channel></rss>`)
	return b.String()
}

// feedServer serves feed at /feed.xml with the given headers, and answers 404
// for any article page while counting how many were asked for.
func feedServer(t *testing.T, headers map[string]string, feed func(base string) string) (*httptest.Server, *atomic.Int32) {
	t.Helper()

	var pageHits atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/feed.xml" {
			pageHits.Add(1)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, feed(srv.URL))
	}))
	t.Cleanup(srv.Close)
	return srv, &pageHits
}

// --- tests ---------------------------------------------------------------

func TestClaimDueFeedsAppliesTheSyncRules(t *testing.T) {
	s, f := newSyncTestService(t)

	if _, err := s.ClaimDueFeeds(context.Background(), 50, 10*time.Minute); err != nil {
		t.Fatal(err)
	}

	want := []time.Duration{9 * time.Minute, syncLockWindow, deadFeedRetryInterval}
	if fmt.Sprint(f.feeds.claimArgs) != fmt.Sprint(want) {
		t.Errorf("claim args = %v, want %v", f.feeds.claimArgs, want)
	}
	if syncLockWindow <= feedProcessTimeout {
		t.Errorf("lock window %v must outlast a sync (%v), or a live sync gets claimed twice", syncLockWindow, feedProcessTimeout)
	}
}

func TestSyncFeedNotModified(t *testing.T) {
	s, f := newSyncTestService(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != `"v1"` || r.Header.Get("If-Modified-Since") != "lm" {
			t.Errorf("conditional headers not sent: %v", r.Header)
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	res := s.SyncFeed(context.Background(), &data.Feed{ID: 1, Url: srv.URL, HttpEtag: `"v1"`, HttpLastModified: "lm"})

	if res.Status != SyncNotModified {
		t.Fatalf("status = %q, want %q", res.Status, SyncNotModified)
	}
	if len(f.feeds.released) != 1 || f.feeds.released[0] != (releaseCall{`"v1"`, "lm"}) {
		t.Errorf("released = %v, want the stored headers kept", f.feeds.released)
	}
}

func TestSyncFeedFailure(t *testing.T) {
	tests := []struct {
		name       string
		failures   int
		wantStatus SyncStatus
	}{
		{"below the threshold", 3, SyncFailed},
		{"reaching the threshold", feedFailureThreshold, SyncDeactivated},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, f := newSyncTestService(t)
			f.feeds.failures = tt.failures

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer srv.Close()

			res := s.SyncFeed(context.Background(), &data.Feed{ID: 1, Url: srv.URL})

			if res.Status != tt.wantStatus || res.Failures != tt.failures {
				t.Errorf("got %q with %d failures, want %q with %d", res.Status, res.Failures, tt.wantStatus, tt.failures)
			}
			if res.Err == nil || !strings.Contains(res.Err.Error(), "500") {
				t.Errorf("err = %v, want the status as the reason", res.Err)
			}
			if res.StoreErr != nil {
				t.Errorf("store err = %v, want none: the feed failing is not ours", res.StoreErr)
			}
			if f.feeds.failedCalls != 1 || len(f.feeds.released) != 0 {
				t.Errorf("failed %d times and released %d, want one failure and no release", f.feeds.failedCalls, len(f.feeds.released))
			}
		})
	}
}

// A server that drops ETag on a 200 must not cost the feed its conditional
// fetching: the previous value is kept, the new Last-Modified taken.
func TestSyncFeedKeepsCachingHeadersTheServerOmits(t *testing.T) {
	s, f := newSyncTestService(t)
	srv, _ := feedServer(t, map[string]string{"Last-Modified": "new-lm"}, func(base string) string {
		return rss(base + "/a")
	})

	res := s.SyncFeed(context.Background(), &data.Feed{ID: 1, Url: srv.URL + "/feed.xml", HttpEtag: `"old"`, HttpLastModified: "old-lm"})

	if res.Status != SyncSynced || res.NewArticles != 1 || res.FailedItems != 0 {
		t.Fatalf("got %+v, want synced with one new article", res)
	}
	if len(f.feeds.released) != 1 || f.feeds.released[0] != (releaseCall{`"old"`, "new-lm"}) {
		t.Errorf("released = %v, want the old ETag and the new Last-Modified", f.feeds.released)
	}
	// The page 404s, so the article falls back to the item's own text.
	if len(f.articles.inserted) != 1 || !strings.Contains(f.articles.inserted[0].TextContent, "Body 0") {
		t.Errorf("inserted = %+v, want one article built from the item description", f.articles.inserted)
	}
	if len(f.links.articles) != 1 {
		t.Errorf("link inserts = %d, want 1", len(f.links.articles))
	}
}

// One failed item holds the caching headers back, so the next sync reads the
// feed in full and retries it, while the other items still land.
func TestSyncFeedHoldsHeadersBackOnFailedItem(t *testing.T) {
	s, f := newSyncTestService(t)
	srv, _ := feedServer(t, map[string]string{"ETag": `"new"`}, func(base string) string {
		return rss(base+"/known", base+"/broken")
	})
	f.articles.byHash[hashURL(srv.URL+"/known")] = 7
	f.articles.insertErr[srv.URL+"/broken"] = errors.New("connection reset by peer")

	res := s.SyncFeed(context.Background(), &data.Feed{ID: 1, Url: srv.URL + "/feed.xml", HttpEtag: `"old"`})

	if res.Status != SyncSynced || res.FailedItems != 1 || res.UnstorableItems != 0 {
		t.Fatalf("got %+v, want synced with one failed item", res)
	}
	if len(f.feeds.released) != 1 || f.feeds.released[0].etag != `"old"` {
		t.Errorf("released = %v, want the old ETag kept", f.feeds.released)
	}
	if len(f.links.articles) != 1 || f.links.articles[0] != 7 {
		t.Errorf("link inserts = %v, want the healthy item linked", f.links.articles)
	}
	out := f.logged.String()
	if !strings.Contains(out, "feed item failed") || !strings.Contains(out, srv.URL+"/broken") {
		t.Errorf("the failed item was not logged, got: %s", out)
	}
	if !strings.Contains(out, `"permanent":"false"`) {
		t.Errorf("the failure was not logged as transient, got: %s", out)
	}
}

// An item the database refuses for its content is refused again on every
// retry, so it must not hold the headers back: that would re-download and
// re-scrape the whole feed every sync for as long as the item stays in it.
func TestSyncFeedAdvancesHeadersPastUnstorableItem(t *testing.T) {
	s, f := newSyncTestService(t)
	srv, _ := feedServer(t, map[string]string{"ETag": `"new"`}, func(base string) string {
		return rss(base+"/known", base+"/unstorable")
	})
	f.articles.byHash[hashURL(srv.URL+"/known")] = 7
	f.articles.insertErr[srv.URL+"/unstorable"] = fmt.Errorf("%w: check constraint", data.ErrUnstorable)

	res := s.SyncFeed(context.Background(), &data.Feed{ID: 1, Url: srv.URL + "/feed.xml", HttpEtag: `"old"`})

	if res.Status != SyncSynced || res.FailedItems != 0 || res.UnstorableItems != 1 {
		t.Fatalf("got %+v, want synced with one unstorable item and no failed one", res)
	}
	if len(f.feeds.released) != 1 || f.feeds.released[0].etag != `"new"` {
		t.Errorf("released = %v, want the new ETag", f.feeds.released)
	}
	out := f.logged.String()
	if !strings.Contains(out, srv.URL+"/unstorable") || !strings.Contains(out, `"permanent":"true"`) {
		t.Errorf("the unstorable item was not logged as permanent, got: %s", out)
	}
}

// An article another feed already brought in is linked, not scraped again.
func TestSyncFeedReusesKnownArticle(t *testing.T) {
	s, f := newSyncTestService(t)
	srv, pageHits := feedServer(t, nil, func(base string) string { return rss(base + "/known") })
	f.articles.byHash[hashURL(srv.URL+"/known")] = 7

	res := s.SyncFeed(context.Background(), &data.Feed{ID: 1, Url: srv.URL + "/feed.xml"})

	if res.Status != SyncSynced || res.NewArticles != 0 {
		t.Fatalf("got %+v, want synced with no new article", res)
	}
	if pageHits.Load() != 0 || len(f.articles.inserted) != 0 {
		t.Errorf("page hits = %d, inserts = %d, want neither", pageHits.Load(), len(f.articles.inserted))
	}
	if len(f.links.articles) != 1 || f.links.articles[0] != 7 {
		t.Errorf("link inserts = %v, want the known article linked", f.links.articles)
	}
}

// A shutdown is not the feed failing: no failure is counted and the claim is
// given back, so the next start syncs the feed at once.
func TestSyncFeedCancelledByShutdown(t *testing.T) {
	t.Run("before the sync starts", func(t *testing.T) {
		s, f := newSyncTestService(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		res := s.SyncFeed(ctx, &data.Feed{ID: 1, Url: "http://127.0.0.1:1/feed.xml"})

		assertCancelled(t, res, f)
	})

	t.Run("during the fetch", func(t *testing.T) {
		s, f := newSyncTestService(t)
		ctx, cancel := context.WithCancel(context.Background())

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cancel()
			<-r.Context().Done()
		}))
		defer srv.Close()

		res := s.SyncFeed(ctx, &data.Feed{ID: 1, Url: srv.URL})

		assertCancelled(t, res, f)
	})
}

func assertCancelled(t *testing.T, res SyncResult, f *syncFakes) {
	t.Helper()

	if res.Status != SyncCancelled {
		t.Fatalf("status = %q, want %q", res.Status, SyncCancelled)
	}
	if f.feeds.unclaimed != 1 || f.feeds.failedCalls != 0 || len(f.feeds.released) != 0 {
		t.Errorf("unclaimed %d, failed %d, released %d; want only the claim given back",
			f.feeds.unclaimed, f.feeds.failedCalls, len(f.feeds.released))
	}
}

// A panic below — the readability parser on a hostile page is the real-world
// case — must come back as a failure, not unwind the scheduler's worker.
func TestSyncFeedRecoversPanic(t *testing.T) {
	s, f := newSyncTestService(t)
	// A nil client makes the fetch panic: the stand-in for any panic raised
	// deeper in the parsing and scraping code.
	s.client = nil

	res := s.SyncFeed(context.Background(), &data.Feed{ID: 1, Url: "https://example.com/feed.xml"})

	if res.Status != SyncFailed || res.Err == nil || !strings.Contains(res.Err.Error(), "panic") {
		t.Fatalf("got %+v, want a failure carrying the panic", res)
	}
	if f.feeds.failedCalls != 1 {
		t.Errorf("failed calls = %d, want the feed marked failed", f.feeds.failedCalls)
	}
}
