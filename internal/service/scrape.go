package service

import (
	"context"
	"net/url"
	"strconv"
	"sync/atomic"
	"time"
)

// FetchConfig configures the article-scraping transports. Zero value = previous
// behaviour: stdlib fetch, no browser sidecar, RSS excerpt as fallback.
type FetchConfig struct {
	// TLSImpersonate makes article scraping use a browser TLS fingerprint. On by
	// default; the flag exists to fall back to the stdlib if the third-party
	// transport misbehaves. RSS polling is never affected.
	TLSImpersonate bool
	// SolverURL is the FlareSolverr-compatible sidecar endpoint (Byparr,
	// FlareBypasser, FlareSolverr). Empty disables browser escalation.
	SolverURL string
	// SolverTimeout is the per-solve budget handed to the sidecar.
	SolverTimeout time.Duration
	// SolverMaxPerFeed caps browser solves per feed run. A solve can take a
	// minute and feedProcessTimeout bounds the whole feed at 8, so without a cap
	// a handful of protected articles starves every remaining item — which also
	// blocks the ETag from advancing and makes the next tick redo everything.
	SolverMaxPerFeed int
	// AllowPrivateNetworks lets fetches reach RFC1918/loopback addresses. Off by
	// default: feed and article URLs come from users, so an unguarded fetch is an
	// SSRF into whatever the deployment can reach. On for the self-hoster whose
	// feed lives on the LAN. Cloud metadata stays blocked either way — see
	// internal/safedial.
	AllowPrivateNetworks bool
}

const (
	defaultSolverTimeout    = 60 * time.Second
	defaultSolverMaxPerFeed = 5
	// challengedHostTTL is how long we remember that a host only answers through
	// the browser, so we skip the doomed impersonated fetch for its other
	// articles. Short enough that a site dropping its protection recovers.
	challengedHostTTL = time.Hour
)

func (c *FetchConfig) setDefaults() {
	if c.SolverTimeout <= 0 {
		c.SolverTimeout = defaultSolverTimeout
	}
	if c.SolverMaxPerFeed <= 0 {
		c.SolverMaxPerFeed = defaultSolverMaxPerFeed
	}
}

// solveBudget caps browser solves for one feed run. It rides on the context so
// the per-article scrape path can consume it without threading a counter
// through every signature.
type solveBudget struct {
	remaining atomic.Int32
}

type solveBudgetKey struct{}

func withSolveBudget(ctx context.Context, n int) context.Context {
	b := &solveBudget{}
	b.remaining.Store(int32(n))
	return context.WithValue(ctx, solveBudgetKey{}, b)
}

// takeSolve consumes one solve from the budget. Without a budget in the context
// (a one-off scrape outside a feed run) it always allows.
func takeSolve(ctx context.Context) bool {
	b, ok := ctx.Value(solveBudgetKey{}).(*solveBudget)
	if !ok {
		return true
	}
	return b.remaining.Add(-1) >= 0
}

// hostNeedsBrowser reports whether host was recently seen serving a challenge.
func (s *FeedService) hostNeedsBrowser(host string) bool {
	v, ok := s.challengedHosts.Load(host)
	if !ok {
		return false
	}
	expiry, ok := v.(time.Time)
	if !ok || time.Now().After(expiry) {
		s.challengedHosts.Delete(host)
		return false
	}
	return true
}

func (s *FeedService) markHostChallenged(host string) {
	s.challengedHosts.Store(host, time.Now().Add(challengedHostTTL))
}

func (s *FeedService) clearHostChallenged(host string) {
	s.challengedHosts.Delete(host)
}

// fetchPage runs the scraping ladder for one article URL:
//
//	impersonated TLS (or stdlib) → browser sidecar on challenge → error
//
// Returning an error is what makes the caller fall back to the RSS item
// content, so a challenge we cannot solve is an error, not a page.
func (s *FeedService) fetchPage(ctx context.Context, u *url.URL) (*pageResponse, error) {
	// Hosts known to gate everything behind a JS challenge go straight to the
	// sidecar; the impersonated fetch would only burn a round-trip. If the
	// sidecar can't deliver we still try the normal path below.
	if s.solver != nil && s.hostNeedsBrowser(u.Host) && s.solver.available() && takeSolve(ctx) {
		if page, err := s.solver.fetch(ctx, u); err == nil {
			return page, nil
		} else if ctx.Err() != nil {
			return nil, err
		}
	}

	page, err := s.scrape.fetch(ctx, u)
	if err != nil {
		// Transport error on the impersonated client: it is a third-party
		// library on a critical path, so give the stdlib one chance rather than
		// lose the article. HTTP error *statuses* don't come through here.
		if ctx.Err() != nil || s.scrape == s.scrapeStdlib {
			return nil, err
		}
		s.logFetch("scrape transport failed, retrying with stdlib", u, s.scrape.name(), err)
		page, err = s.scrapeStdlib.fetch(ctx, u)
		if err != nil {
			return nil, err
		}
	}

	signal := detectChallenge(page)
	if signal == challengeNone {
		if page.StatusCode < 200 || page.StatusCode >= 300 {
			return nil, &fetchStatusError{status: page.StatusCode, url: u.String()}
		}
		s.clearHostChallenged(u.Host)
		return page, nil
	}

	// An empty page is our weakest signal — a paywall stub reads the same — so it
	// escalates for this article only. Marking the host would route a whole site
	// of legitimately thin pages to the browser for an hour.
	if signal != challengeEmptyPage {
		s.markHostChallenged(u.Host)
	}

	// Impersonation can make things *worse*: some WAFs match the exact Chrome
	// header order and block it as "a bot pretending to be a browser", while
	// serving a plainer client without complaint (zonebourse.com does exactly
	// this). One stdlib retry is far cheaper than a browser solve and costs no
	// solve budget, so it goes first.
	if s.scrape != s.scrapeStdlib {
		if plain, err := s.scrapeStdlib.fetch(ctx, u); err == nil &&
			plain.StatusCode >= 200 && plain.StatusCode < 300 &&
			detectChallenge(plain) == challengeNone {
			s.clearHostChallenged(u.Host)
			s.logFetch("blocked while impersonating, stdlib client got through", u, string(signal), nil)
			return plain, nil
		}
	}

	if s.solver == nil {
		return nil, errChallenge
	}
	if !takeSolve(ctx) {
		s.logFetch("challenge detected but solve budget exhausted for this feed run", u, string(signal), nil)
		return nil, errChallenge
	}

	solved, err := s.solver.fetch(ctx, u)
	if err != nil {
		s.logFetch("browser sidecar could not solve challenge", u, string(signal), err)
		return nil, errChallenge
	}

	s.logFetch("challenge solved via browser sidecar", u, string(signal), nil)
	return solved, nil
}

// logFetch records the interesting scrape events — escalations, fallbacks,
// failures — so the value of the sidecar is measurable. The nominal path stays
// silent to keep the log readable.
func (s *FeedService) logFetch(message string, u *url.URL, detail string, err error) {
	if s.logger == nil {
		return
	}
	props := map[string]string{
		"url":    u.String(),
		"host":   u.Host,
		"detail": detail,
	}
	if err != nil {
		props["error"] = err.Error()
	}
	s.logger.PrintInfo(message, props)
}

// fetchStatusError reports a non-2xx page that is not an anti-bot challenge.
type fetchStatusError struct {
	status int
	url    string
}

func (e *fetchStatusError) Error() string {
	return "fetch " + e.url + ": status " + strconv.Itoa(e.status)
}
