package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// maxPageBytes bounds how much of a scraped page we pull into memory. Articles
// are HTML documents; anything past a few megabytes is a download, not a page.
const maxPageBytes = 8 << 20

// The impersonated TLS handshake and the announced User-Agent must describe the
// *same* browser: a Chrome 146 fingerprint claiming to be Chrome 120 is itself
// an anti-bot signal. Bump these three together.
var browserProfile = profiles.Chrome_146

const (
	browserUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36"
	browserSecChUA   = `"Not?A_Brand";v="8", "Chromium";v="146", "Google Chrome";v="146"`
)

// pageResponse is a fetched page, normalised across the three transports
// (stdlib, impersonated TLS, browser sidecar) so the caller never has to care
// which one produced it.
type pageResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	// URL is the final URL after redirects; it is the base readability resolves
	// relative links against.
	URL *url.URL
	// Via names the transport that produced the response, for logging.
	Via string
}

// pageFetcher fetches an article page. Implementations must be safe for
// concurrent use: the scheduler runs a pool of workers.
type pageFetcher interface {
	fetch(ctx context.Context, u *url.URL) (*pageResponse, error)
	name() string
}

// browserHeader is one request header. We keep an ordered slice rather than a
// map because header *order* is part of the browser fingerprint: Cloudflare
// looks at it alongside JA3/JA4, and a request carrying only a User-Agent is
// flagged however good its TLS handshake is.
type browserHeader struct {
	key   string
	value string
}

// browserHeaders returns the header set Chrome sends on a top-level navigation,
// in Chrome's order.
func browserHeaders() []browserHeader {
	return []browserHeader{
		{"sec-ch-ua", browserSecChUA},
		{"sec-ch-ua-mobile", "?0"},
		{"sec-ch-ua-platform", `"Windows"`},
		{"upgrade-insecure-requests", "1"},
		{"user-agent", browserUserAgent},
		{"accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
		{"sec-fetch-site", "none"},
		{"sec-fetch-mode", "navigate"},
		{"sec-fetch-user", "?1"},
		{"sec-fetch-dest", "document"},
		// Chrome advertises zstd too; omitting it contradicts the User-Agent we
		// send, which is exactly the kind of inconsistency anti-bots score on.
		{"accept-encoding", "gzip, deflate, br, zstd"},
		{"accept-language", "fr-FR,fr;q=0.9,en-US;q=0.8,en;q=0.7"},
		{"priority", "u=0, i"},
	}
}

// readPageBody reads at most maxPageBytes from r.
func readPageBody(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, maxPageBytes))
}

// stdlibFetcher fetches with the standard net/http client. It is the transport
// the scraper used before anti-bot handling existed, kept as the fallback when
// impersonation is disabled or its transport fails.
type stdlibFetcher struct {
	client *http.Client
}

func (f *stdlibFetcher) name() string { return "stdlib" }

func (f *stdlibFetcher) fetch(ctx context.Context, u *url.URL) (*pageResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	for _, h := range browserHeaders() {
		// Setting Accept-Encoding by hand switches off net/http's transparent
		// gzip handling, and the stdlib transport can't decode brotli at all.
		// Let it negotiate compression itself.
		if h.key == "accept-encoding" {
			continue
		}
		req.Header.Set(h.key, h.value)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := readPageBody(resp.Body)
	if err != nil {
		return nil, err
	}

	final := u
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL
	}

	return &pageResponse{
		StatusCode: resp.StatusCode,
		Header:     resp.Header,
		Body:       body,
		URL:        final,
		Via:        f.name(),
	}, nil
}

// tlsFetcher fetches with a client that reproduces a real browser's TLS
// (JA3/JA4) and HTTP/2 fingerprint. This is what gets past Cloudflare's
// *passive* filtering, which rejects Go's stdlib handshake outright — no
// JavaScript execution involved, so no browser needed.
type tlsFetcher struct {
	client tls_client.HttpClient
}

func newTLSFetcher(timeout time.Duration) (*tlsFetcher, error) {
	client, err := tls_client.NewHttpClient(
		tls_client.NewNoopLogger(),
		tls_client.WithClientProfile(browserProfile),
		tls_client.WithTimeoutSeconds(int(timeout.Seconds())),
		// A jar lets clearance cookies handed out mid-redirect survive to the
		// next hop, like a browser's would.
		tls_client.WithCookieJar(tls_client.NewCookieJar()),
	)
	if err != nil {
		return nil, err
	}
	return &tlsFetcher{client: client}, nil
}

func (f *tlsFetcher) name() string { return "tls" }

func (f *tlsFetcher) fetch(ctx context.Context, u *url.URL) (*pageResponse, error) {
	req, err := fhttp.NewRequestWithContext(ctx, fhttp.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}

	header := make(fhttp.Header, len(browserHeaders())+1)
	order := make([]string, 0, len(browserHeaders()))
	for _, h := range browserHeaders() {
		header[h.key] = []string{h.value}
		order = append(order, h.key)
	}
	header[fhttp.HeaderOrderKey] = order
	req.Header = header

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Because we set Accept-Encoding by hand, decompression depends on the
	// protocol: fhttp's HTTP/2 path decompresses but leaves Content-Encoding in
	// place, so decompressing unconditionally double-decodes and fails with
	// "gzip: invalid header". Uncompressed is the reliable signal.
	reader := io.Reader(resp.Body)
	if !resp.Uncompressed {
		reader = fhttp.DecompressBody(resp)
	}

	body, err := readPageBody(reader)
	if err != nil {
		return nil, err
	}

	final := u
	if resp.Request != nil && resp.Request.URL != nil {
		final = resp.Request.URL
	}

	return &pageResponse{
		StatusCode: resp.StatusCode,
		Header:     toStdHeader(resp.Header),
		Body:       body,
		URL:        final,
		Via:        f.name(),
	}, nil
}

// toStdHeader converts fhttp headers to net/http ones, dropping the magic
// ordering keys tls-client uses internally.
func toStdHeader(h fhttp.Header) http.Header {
	out := make(http.Header, len(h))
	for k, v := range h {
		if k == fhttp.HeaderOrderKey || k == fhttp.PHeaderOrderKey {
			continue
		}
		out[http.CanonicalHeaderKey(k)] = v
	}
	return out
}

// newScrapeFetcher builds the article-scraping transport: impersonated when
// enabled, stdlib otherwise. It never fails the caller — if the impersonated
// client can't be built we report the error and let the caller fall back.
func newScrapeFetcher(impersonate bool, timeout time.Duration, fallback pageFetcher) (pageFetcher, error) {
	if !impersonate {
		return fallback, nil
	}
	f, err := newTLSFetcher(timeout)
	if err != nil {
		return fallback, fmt.Errorf("tls impersonation unavailable, falling back to stdlib: %w", err)
	}
	return f, nil
}
