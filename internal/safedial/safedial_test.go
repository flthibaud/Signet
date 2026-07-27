package safedial

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCheckAddrBlocksByDefault(t *testing.T) {
	g := NewGuard(false)

	blocked := []string{
		"169.254.169.254", // AWS/GCP/Azure instance metadata
		"127.0.0.1",
		"127.0.0.53", // systemd-resolved
		"0.0.0.0",
		"10.0.0.5",
		"172.16.0.1",
		"172.31.255.255",
		"192.168.1.10",
		"100.64.0.1", // CGNAT
		"198.18.0.1", // benchmarking
		"224.0.0.1",  // multicast
		"255.255.255.255",
		"::1",
		"::",
		"fe80::1",                // IPv6 link-local
		"fd00::1",                // unique local
		"ff02::1",                // IPv6 multicast
		"::ffff:169.254.169.254", // IPv4-mapped metadata
		"::ffff:10.0.0.5",        // IPv4-mapped private
		"2002:a9fe:a9fe::",       // 6to4-wrapped metadata
		"64:ff9b::a9fe:a9fe",     // NAT64-wrapped metadata
		"2001:0:1:2:3:4:5:6",     // Teredo
	}

	for _, s := range blocked {
		t.Run(s, func(t *testing.T) {
			err := g.CheckAddr(netip.MustParseAddr(s))
			if err == nil {
				t.Fatalf("CheckAddr(%s) = nil, want blocked", s)
			}
			if !errors.Is(err, ErrBlocked) {
				t.Fatalf("CheckAddr(%s) error %v does not wrap ErrBlocked", s, err)
			}
		})
	}
}

func TestCheckAddrAllowsPublic(t *testing.T) {
	g := NewGuard(false)

	allowed := []string{
		"1.1.1.1",
		"140.82.121.4", // github.com
		"93.184.216.34",
		"172.32.0.1",  // just outside 172.16/12
		"100.128.0.1", // just outside the CGNAT block
		"2606:4700:4700::1111",
		"2a00:1450:4007:80f::200e",
	}

	for _, s := range allowed {
		t.Run(s, func(t *testing.T) {
			if err := g.CheckAddr(netip.MustParseAddr(s)); err != nil {
				t.Fatalf("CheckAddr(%s) = %v, want allowed", s, err)
			}
		})
	}
}

// AllowPrivate is the self-hoster's knob: it opens the LAN, and nothing else.
func TestAllowPrivateOpensLANButNotMetadata(t *testing.T) {
	g := NewGuard(true)

	nowAllowed := []string{"10.0.0.5", "192.168.1.10", "172.16.0.1", "127.0.0.1", "fd00::1", "100.64.0.1"}
	for _, s := range nowAllowed {
		if err := g.CheckAddr(netip.MustParseAddr(s)); err != nil {
			t.Errorf("AllowPrivate: CheckAddr(%s) = %v, want allowed", s, err)
		}
	}

	stillBlocked := []string{
		"169.254.169.254",
		"fe80::1",
		"0.0.0.0",
		"224.0.0.1",
		"64:ff9b::a9fe:a9fe",
		"2002:a9fe:a9fe::",
	}
	for _, s := range stillBlocked {
		if err := g.CheckAddr(netip.MustParseAddr(s)); err == nil {
			t.Errorf("AllowPrivate: CheckAddr(%s) = nil, want still blocked", s)
		}
	}
}

func TestControlRejectsUnparseableAddress(t *testing.T) {
	g := NewGuard(false)
	if err := g.Control("tcp", "not-an-address", nil); err == nil {
		t.Fatal("Control on a garbage address = nil, want blocked")
	}
}

// The dialer must refuse a direct connection to a loopback listener even though
// the listener is real and reachable.
func TestTransportBlocksDirectLoopbackDial(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("secret"))
	}))
	defer srv.Close()

	client := &http.Client{Transport: NewGuard(false).Transport(5 * time.Second)}

	resp, err := client.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("request to loopback succeeded, want blocked")
	}
	if !strings.Contains(err.Error(), "blocked address") {
		t.Fatalf("error %v is not a guard rejection", err)
	}
}

// The bypass a URL validator cannot catch: an allowed host answering 302 to a
// blocked one. The first hop must succeed and the second must be refused, which
// is only true because Control runs per-connection rather than on the submitted
// URL.
//
// AllowPrivate makes the loopback test server stand in for the "allowed public
// site"; the metadata endpoint stays blocked either way, so it is the one hop
// the guard has to stop.
func TestTransportBlocksRedirectToMetadata(t *testing.T) {
	var hops int

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/iam/security-credentials/", http.StatusFound)
	}))
	defer redirector.Close()

	client := &http.Client{Transport: NewGuard(true).Transport(5 * time.Second)}

	resp, err := client.Get(redirector.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("redirect to the metadata endpoint completed, want blocked")
	}
	if hops != 1 {
		t.Fatalf("first hop ran %d times, want 1 — the test is not exercising the redirect", hops)
	}
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("error %v does not wrap ErrBlocked", err)
	}
	if !strings.Contains(err.Error(), "169.254.169.254") {
		t.Fatalf("error %v does not name the blocked hop", err)
	}
}

// The property DNS rebinding attacks: a *hostname* must be judged on what it
// resolves to at dial time, not on how it looks. Nothing here pre-validates the
// name, so there is no window between the check and the connection for an
// attacker's second DNS answer to slip through.
func TestDialerJudgesHostnamesOnResolvedAddress(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("splitting %q: %v", ln.Addr(), err)
	}
	// A name, not an IP literal — the guard only ever sees the resolved address.
	target := net.JoinHostPort("localhost", port)

	conn, err := NewGuard(false).Dialer(5*time.Second).DialContext(context.Background(), "tcp", target)
	if err == nil {
		conn.Close()
		t.Fatal("dialled a hostname resolving to loopback, want blocked")
	}
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("error %v does not wrap ErrBlocked", err)
	}

	conn, err = NewGuard(true).Dialer(5*time.Second).DialContext(context.Background(), "tcp", target)
	if err != nil {
		t.Fatalf("AllowPrivate: dialling %s = %v, want success", target, err)
	}
	conn.Close()
}

// With AllowPrivate the same loopback listener must become reachable, otherwise
// the knob does not do what the self-hoster expects.
func TestAllowPrivateReachesLoopbackServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<rss/>"))
	}))
	defer srv.Close()

	client := &http.Client{Transport: NewGuard(true).Transport(5 * time.Second)}

	resp, err := client.Get(srv.URL)
	if err != nil {
		t.Fatalf("AllowPrivate: request to loopback = %v, want success", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestCheckURL(t *testing.T) {
	g := NewGuard(false)
	ctx := context.Background()

	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"metadata by IP", "http://169.254.169.254/latest/meta-data/", true},
		{"private by IP", "http://10.0.0.5:6379/", true},
		{"loopback by name", "http://localhost:8000/", true},
		{"bracketed IPv6 loopback", "http://[::1]:8080/", true},
		{"no host", "http:///path", true},
		{"public IP", "http://1.1.1.1/feed.xml", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.raw)
			if err != nil {
				t.Fatalf("parsing %q: %v", tt.raw, err)
			}
			err = g.CheckURL(ctx, u)

			// A resolver failure in a sandboxed CI is not a policy answer.
			var dnsErr *net.DNSError
			if errors.As(err, &dnsErr) {
				t.Skipf("DNS unavailable: %v", err)
			}

			if tt.wantErr && err == nil {
				t.Fatalf("CheckURL(%s) = nil, want blocked", tt.raw)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("CheckURL(%s) = %v, want allowed", tt.raw, err)
			}
		})
	}
}
