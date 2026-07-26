package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeFetcher is a pageFetcher returning canned responses, so the escalation
// ladder can be tested without a network or a browser.
type fakeFetcher struct {
	id    string
	calls atomic.Int32
	resp  *pageResponse
	err   error
}

func (f *fakeFetcher) name() string { return f.id }

func (f *fakeFetcher) fetch(ctx context.Context, u *url.URL) (*pageResponse, error) {
	f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	clone := *f.resp
	clone.URL = u
	clone.Via = f.id
	return &clone, nil
}

// okPage wraps body in enough real text to clear the empty-page heuristic, so
// these tests exercise the escalation ladder rather than the detector.
func okPage(body string) *pageResponse {
	full := "<html><body>" + body + strings.Repeat("<p>du contenu editorial bien reel.</p>", 20) + "</body></html>"
	return &pageResponse{StatusCode: 200, Header: http.Header{}, Body: []byte(full)}
}

func challengePage() *pageResponse {
	return &pageResponse{
		StatusCode: 403,
		Header:     http.Header{"Server": {"cloudflare"}},
		Body:       []byte("<html><head><title>Just a moment...</title></head></html>"),
	}
}

// newTestService builds a FeedService with only the scraping plumbing wired.
func newTestService(scrape, stdlib pageFetcher, cfg FetchConfig) *FeedService {
	cfg.setDefaults()
	return &FeedService{
		fetchCfg:     cfg,
		scrape:       scrape,
		scrapeStdlib: stdlib,
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parsing %q: %v", raw, err)
	}
	return u
}

func TestFetchPageCleanResponse(t *testing.T) {
	scrape := &fakeFetcher{id: "tls", resp: okPage("<html><body>article</body></html>")}
	stdlib := &fakeFetcher{id: "stdlib", resp: okPage("stdlib")}
	s := newTestService(scrape, stdlib, FetchConfig{})

	page, err := s.fetchPage(context.Background(), mustURL(t, "https://example.com/a"))
	if err != nil {
		t.Fatalf("fetchPage: %v", err)
	}
	if page.Via != "tls" {
		t.Errorf("Via = %q, want tls", page.Via)
	}
	if stdlib.calls.Load() != 0 {
		t.Error("stdlib should not be called when the primary succeeds")
	}
}

func TestFetchPageFallsBackToStdlibOnTransportError(t *testing.T) {
	scrape := &fakeFetcher{id: "tls", err: errors.New("handshake failed")}
	stdlib := &fakeFetcher{id: "stdlib", resp: okPage("<html><body>article</body></html>")}
	s := newTestService(scrape, stdlib, FetchConfig{})

	page, err := s.fetchPage(context.Background(), mustURL(t, "https://example.com/a"))
	if err != nil {
		t.Fatalf("fetchPage: %v", err)
	}
	if page.Via != "stdlib" {
		t.Errorf("Via = %q, want stdlib", page.Via)
	}
}

func TestFetchPageChallengeWithoutSolverIsAnError(t *testing.T) {
	scrape := &fakeFetcher{id: "tls", resp: challengePage()}
	s := newTestService(scrape, scrape, FetchConfig{})

	_, err := s.fetchPage(context.Background(), mustURL(t, "https://example.com/a"))
	if !errors.Is(err, errChallenge) {
		t.Fatalf("err = %v, want errChallenge", err)
	}
	// The host is remembered so its other articles can skip the doomed fetch.
	if !s.hostNeedsBrowser("example.com") {
		t.Error("expected host to be marked as challenged")
	}
}

func TestFetchPageNonChallengeErrorStatus(t *testing.T) {
	scrape := &fakeFetcher{id: "tls", resp: &pageResponse{
		StatusCode: 404,
		Header:     http.Header{},
		Body:       []byte("<html><body>not found</body></html>"),
	}}
	s := newTestService(scrape, scrape, FetchConfig{})

	_, err := s.fetchPage(context.Background(), mustURL(t, "https://example.com/a"))
	var statusErr *fetchStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("err = %v, want *fetchStatusError", err)
	}
	if statusErr.status != 404 {
		t.Errorf("status = %d, want 404", statusErr.status)
	}
}

func TestFetchPageEscalatesToSolver(t *testing.T) {
	var solverCalls atomic.Int32
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		solverCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","solution":{"url":"https://example.com/a","status":200,"response":"<html><body>rendered</body></html>"}}`))
	}))
	defer sidecar.Close()

	scrape := &fakeFetcher{id: "tls", resp: challengePage()}
	s := newTestService(scrape, scrape, FetchConfig{})
	s.solver = newSolverClient(sidecar.URL, 5*time.Second)

	page, err := s.fetchPage(context.Background(), mustURL(t, "https://example.com/a"))
	if err != nil {
		t.Fatalf("fetchPage: %v", err)
	}
	if page.Via != "browser" {
		t.Errorf("Via = %q, want browser", page.Via)
	}
	if !strings.Contains(string(page.Body), "rendered") {
		t.Errorf("body = %q, want the rendered HTML", page.Body)
	}
	if solverCalls.Load() != 1 {
		t.Errorf("solver called %d times, want 1", solverCalls.Load())
	}
}

func TestFetchPageRespectsSolveBudget(t *testing.T) {
	var solverCalls atomic.Int32
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		solverCalls.Add(1)
		w.Write([]byte(`{"status":"ok","solution":{"status":200,"response":"<html><body>rendered</body></html>"}}`))
	}))
	defer sidecar.Close()

	scrape := &fakeFetcher{id: "tls", resp: challengePage()}
	s := newTestService(scrape, scrape, FetchConfig{SolverMaxPerFeed: 1})
	s.solver = newSolverClient(sidecar.URL, 5*time.Second)

	ctx := withSolveBudget(context.Background(), s.fetchCfg.SolverMaxPerFeed)

	// First article of the run gets its solve.
	if _, err := s.fetchPage(ctx, mustURL(t, "https://example.com/a")); err != nil {
		t.Fatalf("first fetchPage: %v", err)
	}
	// The budget is spent, so the second falls through to the RSS excerpt
	// instead of eating the feed's remaining time.
	if _, err := s.fetchPage(ctx, mustURL(t, "https://example.com/b")); !errors.Is(err, errChallenge) {
		t.Fatalf("second fetchPage err = %v, want errChallenge", err)
	}
	if solverCalls.Load() != 1 {
		t.Errorf("solver called %d times, want 1", solverCalls.Load())
	}
}

func TestSolverBreakerOpensAfterRepeatedFailures(t *testing.T) {
	var calls atomic.Int32
	sidecar := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer sidecar.Close()

	solver := newSolverClient(sidecar.URL, time.Second)
	u := mustURL(t, "https://example.com/a")

	for range solverFailureThreshold {
		if _, err := solver.fetch(context.Background(), u); err == nil {
			t.Fatal("expected the solve to fail")
		}
	}

	if solver.available() {
		t.Fatal("expected the breaker to be open")
	}
	if _, err := solver.fetch(context.Background(), u); !errors.Is(err, errSolverUnavailable) {
		t.Fatalf("err = %v, want errSolverUnavailable", err)
	}
	if calls.Load() != int32(solverFailureThreshold) {
		t.Errorf("sidecar hit %d times, want %d", calls.Load(), solverFailureThreshold)
	}
}

func TestHostChallengedTTL(t *testing.T) {
	s := newTestService(nil, nil, FetchConfig{})

	if s.hostNeedsBrowser("example.com") {
		t.Error("unknown host should not need the browser")
	}

	s.markHostChallenged("example.com")
	if !s.hostNeedsBrowser("example.com") {
		t.Error("marked host should need the browser")
	}

	// An expired entry is forgotten, so a site dropping its protection recovers.
	s.challengedHosts.Store("example.com", time.Now().Add(-time.Minute))
	if s.hostNeedsBrowser("example.com") {
		t.Error("expired mark should have been dropped")
	}

	s.markHostChallenged("example.com")
	s.clearHostChallenged("example.com")
	if s.hostNeedsBrowser("example.com") {
		t.Error("cleared mark should not need the browser")
	}
}

func TestFetchPageBlockedImpersonationFallsBackToStdlib(t *testing.T) {
	// Some WAFs block the exact Chrome fingerprint but serve a plain client.
	scrape := &fakeFetcher{id: "tls", resp: challengePage()}
	stdlib := &fakeFetcher{id: "stdlib", resp: okPage("<html><body>article</body></html>")}
	s := newTestService(scrape, stdlib, FetchConfig{})

	page, err := s.fetchPage(context.Background(), mustURL(t, "https://example.com/a"))
	if err != nil {
		t.Fatalf("fetchPage: %v", err)
	}
	if page.Via != "stdlib" {
		t.Errorf("Via = %q, want stdlib", page.Via)
	}
	// The host recovered, so it must not be routed to the browser next time.
	if s.hostNeedsBrowser("example.com") {
		t.Error("host should have been un-marked after the stdlib recovery")
	}
}

func TestFetchPageStdlibRetryDoesNotConsumeSolveBudget(t *testing.T) {
	// Both transports blocked: the stdlib retry must not eat the solve budget
	// that the sidecar escalation needs.
	scrape := &fakeFetcher{id: "tls", resp: challengePage()}
	stdlib := &fakeFetcher{id: "stdlib", resp: challengePage()}
	s := newTestService(scrape, stdlib, FetchConfig{SolverMaxPerFeed: 1})

	ctx := withSolveBudget(context.Background(), 1)
	if _, err := s.fetchPage(ctx, mustURL(t, "https://example.com/a")); !errors.Is(err, errChallenge) {
		t.Fatalf("err = %v, want errChallenge", err)
	}
	if stdlib.calls.Load() != 1 {
		t.Errorf("stdlib called %d times, want 1", stdlib.calls.Load())
	}
	if !s.hostNeedsBrowser("example.com") {
		t.Error("host should stay marked when neither transport gets through")
	}
}
