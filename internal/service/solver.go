package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/flthibaud/signet/internal/safedial"
)

// solverClient drives a browser sidecar over the FlareSolverr REST contract
// (POST /v1 with a `cmd`), which several implementations expose interchangeably:
// Byparr (Camoufox-backed, the one to prefer today), FlareBypasser, or
// FlareSolverr itself. We target the contract, not the implementation, so
// swapping sidecars is a URL change.
//
// This is the last resort before falling back to the RSS excerpt: it runs a real
// browser, so it is slow and heavy, and only sites demanding JavaScript
// execution justify it.
type solverClient struct {
	url     string
	timeout time.Duration
	client  *http.Client

	// sem serialises solves. The sidecar drives one browser; firing the whole
	// scheduler worker pool at it just makes every solve time out.
	sem chan struct{}

	// guard vets the target URL before it is handed to the sidecar. Best-effort
	// only: the browser resolves DNS itself, in another process, so a rebinding
	// attack beats this check. Network isolation is the real control — see
	// docs/ANTIBOT_FETCHING.md.
	guard *safedial.Guard

	mu        sync.Mutex
	failures  int
	downUntil time.Time
}

// solverFailureThreshold is how many consecutive failures trip the breaker, and
// solverCooldown how long we stop calling the sidecar for once it does. Without
// this, a sidecar that is down turns every article into a timeout.
const (
	solverFailureThreshold = 3
	solverCooldown         = 5 * time.Minute
)

var errSolverUnavailable = errors.New("browser sidecar unavailable")

func newSolverClient(rawURL string, timeout time.Duration, guard *safedial.Guard) *solverClient {
	return &solverClient{
		url:     rawURL,
		timeout: timeout,
		// The sidecar needs its full maxTimeout to answer; leave it headroom
		// before we hang up on it.
		//
		// Deliberately unguarded: this client dials the sidecar, whose URL comes
		// from configuration and normally *is* a private address
		// (http://solver:8191). The user-supplied URL is the one inside the
		// payload, and it is vetted in fetch below.
		client: &http.Client{Timeout: timeout + 30*time.Second},
		sem:    make(chan struct{}, 1),
		guard:  guard,
	}
}

type solverRequest struct {
	Cmd string `json:"cmd"`
	URL string `json:"url"`
	// The two implementations disagree on this field: FlareSolverr reads
	// `maxTimeout` in milliseconds, Byparr reads `max_timeout` in seconds and
	// has no camelCase alias for it. Each ignores the key it doesn't know —
	// FlareSolverr swallows unknown keys, Byparr's pydantic model drops extras —
	// so sending both beats guessing which sidecar is on the other end. Send
	// only one and the other silently falls back to its own 60s default.
	MaxTimeoutMs      int64 `json:"maxTimeout"`
	MaxTimeoutSeconds int64 `json:"max_timeout"`
}

type solverResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution struct {
		URL       string `json:"url"`
		Status    int    `json:"status"`
		Response  string `json:"response"`
		UserAgent string `json:"userAgent"`
		// ContentType is a Byparr extension. It matters because a PDF comes back
		// base64-encoded rather than as HTML, and feeding that to readability
		// would produce an article of gibberish.
		ContentType string `json:"contentType"`
	} `json:"solution"`
}

func (s *solverClient) name() string { return "browser" }

// available reports whether the breaker currently allows a call.
func (s *solverClient) available() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return time.Now().After(s.downUntil)
}

func (s *solverClient) recordSuccess() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = 0
	s.downUntil = time.Time{}
}

func (s *solverClient) recordFailure() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures++
	if s.failures >= solverFailureThreshold {
		s.downUntil = time.Now().Add(solverCooldown)
		s.failures = 0
	}
}

// fetch asks the sidecar to load u in a real browser and returns the rendered
// HTML, which rejoins the normal readability pipeline.
func (s *solverClient) fetch(ctx context.Context, u *url.URL) (*pageResponse, error) {
	if !s.available() {
		return nil, errSolverUnavailable
	}

	// The sidecar is a browser that will fetch whatever we name, from inside the
	// deployment's network. It gets no dialer of ours, so this pre-flight is the
	// only check we can apply — and a rejection here is not a sidecar fault, so
	// it must not touch the breaker.
	if s.guard != nil {
		if err := s.guard.CheckURL(ctx, u); err != nil {
			return nil, err
		}
	}

	// One solve at a time.
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	page, err := s.solve(ctx, u)
	if err != nil {
		// A cancelled context is our own deadline, not the sidecar failing;
		// counting it would trip the breaker on a busy feed.
		if ctx.Err() == nil {
			s.recordFailure()
		}
		return nil, err
	}
	s.recordSuccess()
	return page, nil
}

func (s *solverClient) solve(ctx context.Context, u *url.URL) (*pageResponse, error) {
	payload, err := json.Marshal(solverRequest{
		Cmd:               "request.get",
		URL:               u.String(),
		MaxTimeoutMs:      s.timeout.Milliseconds(),
		MaxTimeoutSeconds: int64(s.timeout.Seconds()),
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("solver returned status %d", resp.StatusCode)
	}

	var solved solverResponse
	if err := json.NewDecoder(resp.Body).Decode(&solved); err != nil {
		return nil, fmt.Errorf("decoding solver response: %w", err)
	}
	if solved.Status != "ok" {
		return nil, fmt.Errorf("solver failed: %s", solved.Message)
	}
	if ct := solved.Solution.ContentType; ct != "" && !strings.HasPrefix(ct, "text/html") {
		return nil, fmt.Errorf("solver returned %s, not HTML", ct)
	}

	body := []byte(solved.Solution.Response)
	if len(body) > maxPageBytes {
		body = body[:maxPageBytes]
	}

	final := u
	if solved.Solution.URL != "" {
		if parsed, err := url.Parse(solved.Solution.URL); err == nil {
			final = parsed
		}
	}

	status := solved.Solution.Status
	if status == 0 {
		status = http.StatusOK
	}

	return &pageResponse{
		StatusCode: status,
		Header:     http.Header{},
		Body:       body,
		URL:        final,
		Via:        s.name(),
	}, nil
}
