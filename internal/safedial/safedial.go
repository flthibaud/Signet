// Package safedial guards the outbound HTTP the app performs on behalf of a
// user, so a feed or article URL cannot be turned into a request against the
// deployment's own network (SSRF).
//
// The check lives in net.Dialer.Control rather than in a URL validator, because
// a validator can only inspect what the user submitted. It is defeated by both
// of the standard bypasses:
//
//   - redirects — a public URL answering 302 to http://169.254.169.254/ is
//     followed by the http.Client, which never re-runs the validator;
//   - DNS rebinding — a hostname the attacker controls resolves to a public IP
//     when validated and to 127.0.0.1 when dialled.
//
// Control runs after DNS resolution, on the address actually being connected
// to, and on every connection a redirect chain opens. That closes both.
package safedial

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"
)

// ErrBlocked is the sentinel wrapped by every refusal, so callers can tell a
// policy rejection from a genuine network failure.
var ErrBlocked = errors.New("blocked address")

// alwaysBlocked are the ranges no configuration reopens.
//
// Link-local is the reason this package exists: 169.254.169.254 is the cloud
// instance metadata endpoint on AWS, GCP, Azure, DigitalOcean and Hetzner, and
// on an IAM-attached instance it hands out role credentials to anything that
// asks. An operator allowing their LAN must not be silently reopening that, so
// it stays blocked even when AllowPrivate is set.
//
// The IPv6 transition ranges are here because each one embeds an IPv4 address
// that the host stack unwraps at send time: 2002::a9fe:a9fe (6to4) and
// 64:ff9b::a9fe:a9fe (NAT64) both reach the metadata endpoint while looking
// like ordinary global unicast to a naive check.
var alwaysBlocked = mustPrefixes(
	"0.0.0.0/8",      // "this network"; 0.0.0.0 itself reaches localhost on Linux
	"169.254.0.0/16", // IPv4 link-local — cloud metadata
	"192.0.0.0/24",   // IETF protocol assignments
	"198.18.0.0/15",  // benchmarking
	"224.0.0.0/4",    // IPv4 multicast
	"240.0.0.0/4",    // reserved, incl. 255.255.255.255
	"::/128",         // unspecified
	"fe80::/10",      // IPv6 link-local
	"ff00::/8",       // IPv6 multicast
	"2002::/16",      // 6to4
	"2001::/32",      // Teredo
	"64:ff9b::/96",   // NAT64 well-known prefix
	"64:ff9b:1::/48", // NAT64 local-use prefix
	"100::/64",       // discard-only
)

// privateRanges are the operator's own networks: blocked by default, reopened
// by AllowPrivate for the self-hoster whose feed lives on the LAN or in a
// neighbouring container.
var privateRanges = mustPrefixes(
	"10.0.0.0/8",
	"172.16.0.0/12",
	"192.168.0.0/16",
	"100.64.0.0/10", // CGNAT, also Tailscale's range
	"127.0.0.0/8",   // loopback
	"::1/128",       // loopback
	"fc00::/7",      // unique local
)

// Guard decides which addresses outbound fetches may connect to.
//
// The zero value is the safe policy (private networks blocked) and is usable.
type Guard struct {
	// AllowPrivate permits connections to RFC1918, CGNAT, unique-local and
	// loopback addresses. It never permits link-local, so cloud metadata stays
	// out of reach whatever the setting.
	AllowPrivate bool
}

// NewGuard returns a Guard. allowPrivate should come from configuration; see
// the AllowPrivate field for exactly what it opens.
func NewGuard(allowPrivate bool) *Guard {
	return &Guard{AllowPrivate: allowPrivate}
}

// CheckAddr reports whether ip may be connected to.
func (g *Guard) CheckAddr(ip netip.Addr) error {
	// An IPv4-mapped IPv6 address (::ffff:127.0.0.1) is an IPv4 address wearing
	// a disguise; unmap so it is matched against the IPv4 ranges.
	ip = ip.Unmap()

	if !ip.IsValid() {
		return fmt.Errorf("%w: invalid IP", ErrBlocked)
	}

	for _, p := range alwaysBlocked {
		if p.Contains(ip) {
			return fmt.Errorf("%w: %s is in reserved range %s", ErrBlocked, ip, p)
		}
	}

	if g.AllowPrivate {
		return nil
	}

	for _, p := range privateRanges {
		if p.Contains(ip) {
			return fmt.Errorf("%w: %s is in private range %s", ErrBlocked, ip, p)
		}
	}

	// Backstop for anything the explicit prefixes miss (interface-local
	// multicast, future stdlib additions).
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return fmt.Errorf("%w: %s is not a public address", ErrBlocked, ip)
	}

	return nil
}

// Control is the net.Dialer.Control hook. address is the post-resolution
// "ip:port" the dialer is about to connect to.
func (g *Guard) Control(_, address string, _ syscall.RawConn) error {
	addrPort, err := netip.ParseAddrPort(address)
	if err != nil {
		// A dialer that hands us something unparseable is not a dialer we can
		// vet, so refuse rather than wave it through.
		return fmt.Errorf("%w: unparseable address %q", ErrBlocked, address)
	}
	return g.CheckAddr(addrPort.Addr())
}

// Dialer returns a net.Dialer that refuses blocked addresses.
func (g *Guard) Dialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
		Control:   g.Control,
	}
}

// Transport returns an http.Transport with the stdlib defaults and the guard
// installed on its dialer.
func (g *Guard) Transport(timeout time.Duration) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = g.Dialer(timeout).DialContext
	return t
}

// CheckURL resolves u's host and rejects it if any address it resolves to is
// blocked. It is the best-effort pre-flight for fetchers we cannot install a
// dialer on — the browser sidecar, which does its own DNS in another process.
// Unlike Control it is racy against DNS rebinding, so it is a supplement to
// network isolation, not a replacement for it.
func (g *Guard) CheckURL(ctx context.Context, u *url.URL) error {
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: no host in %q", ErrBlocked, u.Redacted())
	}

	if ip, err := netip.ParseAddr(host); err == nil {
		return g.CheckAddr(ip)
	}

	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return err
	}
	if len(ips) == 0 {
		return fmt.Errorf("%w: %s resolved to no addresses", ErrBlocked, host)
	}
	// Every answer must pass: a host resolving to one public and one private
	// address would otherwise let the caller pick the private one.
	for _, ip := range ips {
		if err := g.CheckAddr(ip); err != nil {
			return err
		}
	}
	return nil
}

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		p, err := netip.ParsePrefix(c)
		if err != nil {
			panic("safedial: bad CIDR " + c + ": " + err.Error())
		}
		out = append(out, p)
	}
	return out
}
