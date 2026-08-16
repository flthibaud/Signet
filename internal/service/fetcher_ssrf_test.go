package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/safedial"
)

// The guard is only worth anything if it is actually installed on the clients
// NewFeedService builds. These tests dial a real loopback listener and assert it
// is never reached.

func TestCreateFromURLBlocksInternalAddress(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(`<rss version="2.0"><channel><title>internal</title></channel></rss>`))
	}))
	defer internal.Close()

	s := NewFeedService(data.Models{}, nil, FetchConfig{})

	_, err := s.CreateFromURL(context.Background(), internal.URL+"/feed.xml")
	if err == nil {
		t.Fatal("CreateFromURL reached a loopback address, want blocked")
	}
	if !errors.Is(err, ErrFeedNotFound) {
		t.Errorf("err = %v, want it to wrap ErrFeedNotFound so the handler reports it on the url field", err)
	}
	if hits.Load() != 0 {
		t.Errorf("internal server received %d requests, want 0", hits.Load())
	}
}

func TestCreateFromURLReachesPrivateWhenAllowed(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Write([]byte(`<rss version="2.0"><channel><title>lan feed</title></channel></rss>`))
	}))
	defer internal.Close()

	s := NewFeedService(data.Models{}, nil, FetchConfig{AllowPrivateNetworks: true})

	// Models is a zero value, so CreateFromURL cannot get past its insert; the
	// fetch is what we are asserting on, and it happens first. Swallow whatever
	// the nil *sql.DB does so the assertion below is the one that reports.
	func() {
		defer func() { _ = recover() }()
		_, _ = s.CreateFromURL(context.Background(), internal.URL+"/feed.xml")
	}()

	if hits.Load() == 0 {
		t.Error("AllowPrivateNetworks did not let the fetch through to the LAN feed")
	}
}

// The impersonated client is a third-party transport we hand a net.Dialer to.
// If a version bump ever stops honouring it, article scraping silently becomes
// an open SSRF again — so assert on it rather than trust the option.
func TestImpersonatedFetcherBlocksInternalAddress(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte("<html><body>internal</body></html>"))
	}))
	defer internal.Close()

	guard := safedial.NewGuard(false)
	fetcher, err := newTLSFetcher(5*time.Second, guard)
	if err != nil {
		t.Skipf("impersonated transport unavailable: %v", err)
	}

	u := mustURL(t, internal.URL+"/article")
	if _, err := fetcher.fetch(context.Background(), u); err == nil {
		t.Fatal("impersonated fetch reached a loopback address, want blocked")
	}
	if hits.Load() != 0 {
		t.Errorf("internal server received %d requests, want 0 — tls-client is not honouring the dialer", hits.Load())
	}
}

// The favicon lookup takes its URL from the feed's own <link>, i.e. from remote
// content, and runs on the same client.
func TestFetchFaviconURLBlocksInternalAddress(t *testing.T) {
	var hits atomic.Int32
	internal := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Write([]byte(`<html><head><link rel="icon" href="/i.png"></head></html>`))
	}))
	defer internal.Close()

	s := NewFeedService(data.Models{}, nil, FetchConfig{})

	// A blocked fetch falls back to the conventional /favicon.ico guess, which
	// is a string the caller stores but never dereferences server-side.
	got := fetchFaviconURL(context.Background(), s.client, internal.URL)
	if hits.Load() != 0 {
		t.Errorf("internal server received %d requests, want 0", hits.Load())
	}
	if got != internal.URL+"/favicon.ico" {
		t.Errorf("got %q, want the un-fetched fallback", got)
	}
}
