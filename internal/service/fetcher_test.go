package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flthibaud/signet/internal/data"
	"github.com/flthibaud/signet/internal/jsonlog"
	"github.com/mmcdole/gofeed"
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

// --- panic recovery -------------------------------------------------------

// recordingConnector is a database/sql driver that records every query it is
// asked to run, so a test can see what the service wrote without a database.
type recordingConnector struct {
	queries []string
}

func (c *recordingConnector) Connect(context.Context) (driver.Conn, error) {
	return &recordingConn{connector: c}, nil
}

func (c *recordingConnector) Driver() driver.Driver { return recordingDriver{} }

func (c *recordingConnector) ran(queryPart string) bool {
	for _, q := range c.queries {
		if strings.Contains(q, queryPart) {
			return true
		}
	}
	return false
}

type recordingDriver struct{}

func (recordingDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("Open is not used; the connector builds the conns")
}

type recordingConn struct{ connector *recordingConnector }

func (c *recordingConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("queries go through QueryContext")
}
func (c *recordingConn) Close() error              { return nil }
func (c *recordingConn) Begin() (driver.Tx, error) { return nil, errors.New("no transactions") }

func (c *recordingConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.connector.queries = append(c.connector.queries, query)
	return &recordingRows{}, nil
}

// recordingRows serves the single column MarkFeedFailed scans back.
type recordingRows struct{ done bool }

func (r *recordingRows) Columns() []string { return []string{"consecutive_failures"} }
func (r *recordingRows) Close() error      { return nil }
func (r *recordingRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	r.done = true
	dest[0] = int64(1)
	return nil
}

// A panic inside the import — the readability parser on a hostile page is the
// real-world case — must come back as an error with the feed marked failed,
// not unwind the goroutine and take the process down. The post-subscribe
// import runs exactly this method on a bare goroutine, where the middleware's
// recoverPanic is not watching.
func TestImportArticlesForSubscribersRecoversPanic(t *testing.T) {
	connector := &recordingConnector{}
	out := &lockedBuffer{}

	fetchCfg := FetchConfig{AllowPrivateNetworks: true}
	fetchCfg.setDefaults()

	s := &FeedService{
		models:   data.NewModels(sql.OpenDB(connector)),
		logger:   jsonlog.New(out, jsonlog.LevelInfo),
		fetchCfg: fetchCfg,
		// client is left nil, so the fetch below panics: the stand-in for any
		// panic raised deeper in the parsing and scraping code.
	}

	feed := &data.Feed{ID: 9, Url: "https://example.com/feed.xml"}
	err := s.ImportArticlesForSubscribers(context.Background(), feed)
	if err == nil {
		t.Fatal("a panic during the import must return as an error, not unwind the goroutine")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("err = %v, want it to carry the panic", err)
	}
	if !connector.ran("consecutive_failures") {
		t.Errorf("the feed was not marked failed; queries run: %v", connector.queries)
	}
	if logged := out.String(); !strings.Contains(logged, "panic") {
		t.Errorf("the panic was not logged, got: %s", logged)
	}
}

// A feed item that carries only <description> (Content stays empty in plain
// RSS 2.0) must still get a body when the article page cannot be scraped —
// otherwise the article is stored empty and never fetched again.
func TestCreateArticleFromItemFallsBackToDescription(t *testing.T) {
	// The article page is gone, so fetchWithReadability fails and the
	// fallback to the feed item's own text runs.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	// recordingRows hands back one integer, which is all Articles.Insert scans.
	db := sql.OpenDB(&recordingConnector{})
	defer db.Close()

	s := NewFeedService(data.NewModels(db), nil, FetchConfig{AllowPrivateNetworks: true})

	item := &gofeed.Item{
		Title:       "Protected post",
		Link:        srv.URL + "/post",
		Description: "<p>Full text from the feed</p>",
		// Content deliberately left empty: only <content:encoded> fills it.
	}

	article, err := s.createArticleFromItem(context.Background(), item, srv.URL+"/post", "deadbeef", "simple_ua")
	if err != nil {
		t.Fatalf("createArticleFromItem: %v", err)
	}
	if article.OriginalHTML != "<p>Full text from the feed</p>" {
		t.Errorf("OriginalHTML = %q, want the item description", article.OriginalHTML)
	}
	if !strings.Contains(article.TextContent, "Full text from the feed") {
		t.Errorf("TextContent = %q, want the description converted to markdown", article.TextContent)
	}
}
