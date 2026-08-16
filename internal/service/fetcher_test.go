package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flthibaud/signet/internal/data"
)

func TestReadFeedBody(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		wantErr error
	}{
		{"under the cap", 1024, nil},
		{"exactly at the cap", maxFeedBytes, nil},
		{"one byte over", maxFeedBytes + 1, errFeedTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := readFeedBody(bytes.NewReader(bytes.Repeat([]byte("x"), tt.size)))
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if tt.wantErr == nil && len(body) != tt.size {
				t.Errorf("read %d bytes, want %d", len(body), tt.size)
			}
		})
	}
}

// A feed server that never stops sending must not be able to grow the process
// until it dies — the read has to give up and report a bad feed.
func TestCreateFromURLRejectsOversizedFeed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		if _, err := w.Write([]byte(`<rss version="2.0"><channel><title>flood</title>`)); err != nil {
			return
		}
		chunk := bytes.Repeat([]byte("<item><title>x</title></item>"), 1024)
		// Bounded so a bug in the cap fails the test instead of hanging it.
		for written := 0; written < 4*maxFeedBytes; written += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	s := NewFeedService(data.Models{}, nil, FetchConfig{AllowPrivateNetworks: true})

	_, err := s.CreateFromURL(context.Background(), srv.URL+"/feed.xml")
	if !errors.Is(err, ErrInvalidFeed) {
		t.Fatalf("err = %v, want it to wrap ErrInvalidFeed", err)
	}
	if !strings.Contains(err.Error(), errFeedTooLarge.Error()) {
		t.Errorf("err = %v, want the size limit to be what rejected it", err)
	}
}

// The cap only means anything if it applies after decompression: a few hundred
// kilobytes of gzip expand to gigabytes.
func TestCreateFromURLRejectsGzipBomb(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	zeros := make([]byte, 1<<20)
	for i := 0; i < 32; i++ { // 32 MiB in, a few KiB out
		if _, err := zw.Write(zeros); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("Content-Encoding", "gzip")
		w.Write(compressed.Bytes())
	}))
	defer srv.Close()

	s := NewFeedService(data.Models{}, nil, FetchConfig{AllowPrivateNetworks: true})

	_, err := s.CreateFromURL(context.Background(), srv.URL+"/feed.xml")
	if !errors.Is(err, ErrInvalidFeed) {
		t.Fatalf("err = %v, want it to wrap ErrInvalidFeed", err)
	}
	if !strings.Contains(err.Error(), errFeedTooLarge.Error()) {
		t.Errorf("err = %v, want the decompressed body to hit the size limit", err)
	}
}

// A feed nobody's server will serve is not this program failing, and the
// classification the loggers rely on has to survive being wrapped.
func TestFeedFetchErrorClassification(t *testing.T) {
	cause := fmt.Errorf("Get %q: %w", "https://slow.test/atom.xml", context.DeadlineExceeded)

	tests := []struct {
		name         string
		err          error
		wantFetchErr bool
		wantFailures int
	}{
		{
			name:         "routine failure",
			err:          &FeedFetchError{Failures: 3, Err: cause},
			wantFetchErr: true,
			wantFailures: 3,
		},
		{
			name:         "wrapped by a caller",
			err:          fmt.Errorf("importing feed 75: %w", &FeedFetchError{Failures: 1, Err: cause}),
			wantFetchErr: true,
			wantFailures: 1,
		},
		{
			// Deactivation is the one outcome an operator may want to act on,
			// so it must not be swallowed as routine.
			name:         "deactivation",
			err:          fmt.Errorf("feed 75 deactivated after 10 consecutive failures: %w", cause),
			wantFetchErr: false,
		},
		{
			// An unrecognised failure stays loud rather than being downgraded.
			name:         "store failure",
			err:          errors.Join(cause, errors.New("marking feed 75 failed: connection refused")),
			wantFetchErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fetchErr *FeedFetchError
			got := errors.As(tt.err, &fetchErr)

			if got != tt.wantFetchErr {
				t.Fatalf("errors.As(*FeedFetchError) = %v, want %v", got, tt.wantFetchErr)
			}
			if got && fetchErr.Failures != tt.wantFailures {
				t.Errorf("failures = %d, want %d", fetchErr.Failures, tt.wantFailures)
			}
		})
	}
}

// The wrapper must stay transparent to errors.Is, or a shutdown would start
// being reported as a feed failure.
func TestFeedFetchErrorUnwraps(t *testing.T) {
	err := &FeedFetchError{Failures: 1, Err: fmt.Errorf("fetching: %w", context.Canceled)}

	if !errors.Is(err, context.Canceled) {
		t.Error("errors.Is(err, context.Canceled) = false, want true")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !cancelledByShutdown(ctx, err) {
		t.Error("a cancelled import wrapped as a fetch failure must still read as a shutdown")
	}
}
