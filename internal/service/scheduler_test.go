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
)

// fakeSyncer hands out the queued batches one claim at a time, then nothing.
type fakeSyncer struct {
	mu       sync.Mutex
	batches  [][]*data.Feed
	claimErr error
	claims   int
	synced   []int64
	results  map[int64]SyncResult
	onSync   func()
}

func (f *fakeSyncer) ClaimDueFeeds(ctx context.Context, n int, interval time.Duration) ([]*data.Feed, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.claims++
	if f.claimErr != nil {
		return nil, f.claimErr
	}
	if len(f.batches) == 0 {
		return nil, nil
	}
	batch := f.batches[0]
	f.batches = f.batches[1:]
	return batch, nil
}

func (f *fakeSyncer) SyncFeed(ctx context.Context, feed *data.Feed) SyncResult {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.synced = append(f.synced, feed.ID)
	if f.onSync != nil {
		f.onSync()
	}
	if res, ok := f.results[feed.ID]; ok {
		return res
	}
	return SyncResult{Status: SyncSynced}
}

// feeds builds feeds on distinct hosts, so the per-domain limiter never makes a
// test wait.
func feeds(ids ...int64) []*data.Feed {
	out := make([]*data.Feed, len(ids))
	for i, id := range ids {
		out[i] = &data.Feed{ID: id, Url: fmt.Sprintf("https://feed-%d.test/rss", id)}
	}
	return out
}

func newTestScheduler(syncer feedSyncer) (*Scheduler, *lockedBuffer) {
	logged := &lockedBuffer{}
	s := NewScheduler(syncer, data.Models{}, jsonlog.New(logged, jsonlog.LevelInfo),
		SchedulerConfig{Interval: time.Minute, Workers: 2, BatchSize: 2})
	return s, logged
}

func TestSyncFeedsDrainsEveryDueBatch(t *testing.T) {
	syncer := &fakeSyncer{batches: [][]*data.Feed{feeds(1, 2), feeds(3, 4), feeds(5)}}
	s, _ := newTestScheduler(syncer)

	s.syncFeeds()

	// The short third batch was everything left, so no fourth claim.
	if syncer.claims != 3 || len(syncer.synced) != 5 {
		t.Errorf("claims = %d, synced = %v; want 3 claims and all 5 feeds", syncer.claims, syncer.synced)
	}
}

func TestSyncFeedsStopsWhenNothingIsDue(t *testing.T) {
	syncer := &fakeSyncer{batches: [][]*data.Feed{feeds(1, 2)}}
	s, _ := newTestScheduler(syncer)

	s.syncFeeds()

	if syncer.claims != 2 || len(syncer.synced) != 2 {
		t.Errorf("claims = %d, synced = %v; want a second, empty claim and 2 feeds", syncer.claims, syncer.synced)
	}
}

func TestSyncFeedsStopsOnClaimError(t *testing.T) {
	syncer := &fakeSyncer{claimErr: errors.New("database down")}
	s, logged := newTestScheduler(syncer)

	s.syncFeeds()

	if syncer.claims != 1 {
		t.Errorf("claims = %d, want 1", syncer.claims)
	}
	if !strings.Contains(logged.String(), `"level":"ERROR"`) {
		t.Errorf("a failed claim must be logged as an error, got: %s", logged.String())
	}
}

// A shutdown stops the claiming, but every feed already claimed still goes to
// SyncFeed: that call is what gives the claim back.
func TestSyncFeedsHandsBackClaimedFeedsAtShutdown(t *testing.T) {
	syncer := &fakeSyncer{batches: [][]*data.Feed{feeds(1, 2), feeds(3, 4)}}
	s, _ := newTestScheduler(syncer)
	syncer.onSync = s.cancel

	s.syncFeeds()

	if syncer.claims != 1 || len(syncer.synced) != 2 {
		t.Errorf("claims = %d, synced = %v; want one claim, both its feeds handed to SyncFeed", syncer.claims, syncer.synced)
	}
}

// Only a write of ours that did not happen is an error. A dead feed, even one
// just deactivated, is the far end's doing.
func TestProcessFeedLogLevels(t *testing.T) {
	tests := []struct {
		name      string
		res       SyncResult
		wantLevel string
	}{
		{"synced", SyncResult{Status: SyncSynced, NewArticles: 3}, "INFO"},
		{"failed", SyncResult{Status: SyncFailed, Failures: 2, Err: errors.New("status 500")}, "INFO"},
		{"deactivated", SyncResult{Status: SyncDeactivated, Failures: 10, Err: errors.New("status 404")}, "INFO"},
		{"cancelled", SyncResult{Status: SyncCancelled}, "INFO"},
		{"release not written", SyncResult{Status: SyncSynced, StoreErr: errors.New("connection refused")}, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			syncer := &fakeSyncer{results: map[int64]SyncResult{1: tt.res}}
			s, logged := newTestScheduler(syncer)

			s.processFeed(context.Background(), feeds(1)[0])

			out := logged.String()
			if !strings.Contains(out, `"level":"`+tt.wantLevel+`"`) {
				t.Errorf("want one %s line, got: %s", tt.wantLevel, out)
			}
			if !strings.Contains(out, `"status":"`+string(tt.res.Status)+`"`) {
				t.Errorf("the status is missing from the line: %s", out)
			}
		})
	}
}

func TestDomainLimiterIsPerHostAndPerMap(t *testing.T) {
	var polls, scrapes sync.Map

	a := domainLimiter(&polls, "a.test")
	if domainLimiter(&polls, "a.test") != a {
		t.Error("the same host got a second limiter, so it would get twice the budget")
	}
	if domainLimiter(&polls, "b.test") == a {
		t.Error("two hosts share one limiter")
	}
	if domainLimiter(&scrapes, "a.test") == a {
		t.Error("polling and scraping share one limiter for the same host")
	}

	// 1 req/s with a burst of 1: the first request goes through, the next waits.
	if !a.Allow() || a.Allow() {
		t.Error("the limiter is not 1 request per second")
	}
}
