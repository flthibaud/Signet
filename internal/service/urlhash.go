package service

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/url"
	"strings"
)

// trackingParams are query parameters that describe how a reader arrived at a
// page rather than which page it is. The same article syndicated through a
// newsletter, a social share and the site's own feed differs only by these, so
// they must not reach the dedup hash.
//
// Anything not listed here is kept: plenty of sites still address content with
// a query string (?p=123, ?storyId=…), and dropping those would merge unrelated
// articles — a far worse failure than missing a dedup.
var trackingParams = map[string]struct{}{
	"fbclid":      {},
	"gclid":       {},
	"dclid":       {},
	"msclkid":     {},
	"twclid":      {},
	"igshid":      {},
	"mc_cid":      {},
	"mc_eid":      {},
	"_hsenc":      {},
	"_hsmi":       {},
	"vero_id":     {},
	"vero_conv":   {},
	"yclid":       {},
	"wt_zmc":      {},
	"ref_src":     {},
	"ref_url":     {},
	"spm":         {},
	"cmpid":       {},
	"campaign_id": {},
}

// isTrackingParam reports whether a query parameter is campaign/referrer noise.
// The utm_* and pk_* (Matomo) families are matched by prefix because their
// members are open-ended.
func isTrackingParam(key string) bool {
	k := strings.ToLower(key)
	if strings.HasPrefix(k, "utm_") || strings.HasPrefix(k, "pk_") {
		return true
	}
	_, ok := trackingParams[k]
	return ok
}

// normalizeURL returns the canonical form of an article URL, used as the dedup
// key. Articles are stored once for all users, so two feeds pointing at the same
// page must produce the same string here even though they rarely spell the URL
// the same way: http vs https, a trailing slash, www., a default port, a
// fragment, a campaign parameter or a different parameter order.
//
// The result is a hash input, not a URL to fetch — the original link is what
// gets stored and scraped. That is why http is folded onto https: the pair
// practically always serves the same document, and treating them as one page is
// worth more than the rare site that disagrees.
//
// A relative, malformed or non-HTTP link (mailto:, magnet:, …) has no canonical
// form we can trust, so it is returned trimmed and otherwise untouched.
func normalizeURL(raw string) string {
	trimmed := strings.TrimSpace(raw)

	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return trimmed
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
	default:
		return trimmed
	}

	u.Scheme = "https"
	u.User = nil
	u.Fragment = ""
	u.RawFragment = ""

	host := strings.ToLower(u.Host)
	if h, port, err := net.SplitHostPort(host); err == nil && (port == "80" || port == "443") {
		host = h
	}
	u.Host = strings.TrimPrefix(host, "www.")

	// Trim the trailing slash off both spellings of the path so Path and RawPath
	// stay consistent (a slash is never percent-encoded, so the same trim applies
	// to each). "/" collapses to "" as well, which is what makes
	// https://example.com and https://example.com/ the same key.
	u.Path = strings.TrimRight(u.Path, "/")
	if u.RawPath != "" {
		u.RawPath = strings.TrimRight(u.RawPath, "/")
	}

	if u.RawQuery != "" {
		q := u.Query()
		for key := range q {
			if isTrackingParam(key) {
				q.Del(key)
			}
		}
		// Encode sorts by key, so parameter order stops mattering.
		u.RawQuery = q.Encode()
	}

	return u.String()
}

// hashURL computes the dedup key of an article URL: the SHA256 of its
// normalized form (see normalizeURL).
func hashURL(rawURL string) string {
	h := sha256.New()
	h.Write([]byte(normalizeURL(rawURL)))
	return hex.EncodeToString(h.Sum(nil))
}

// itemURL returns the link identifying an RSS item, or "" when it has none.
func itemURL(link, guid string) string {
	if l := strings.TrimSpace(link); l != "" {
		return l
	}

	g := strings.TrimSpace(guid)
	if g == "" {
		return ""
	}
	u, err := url.Parse(g)
	if err != nil || u.Host == "" {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return g
	}
	return ""
}
