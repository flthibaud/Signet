package service

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

// errChallenge means the page we got back is an anti-bot interstitial, not the
// article. Callers treat it like any other fetch failure and fall back to the
// RSS item content.
var errChallenge = errors.New("anti-bot challenge could not be solved")

// challengeSignal names the protection that answered instead of the site.
type challengeSignal string

const (
	challengeNone       challengeSignal = ""
	challengeCloudflare challengeSignal = "cloudflare"
	challengeDataDome   challengeSignal = "datadome"
	challengeIncapsula  challengeSignal = "incapsula"
	challengeGeneric    challengeSignal = "generic"
	// challengeEmptyPage is a 200 carrying no readable text. It is the weakest
	// signal of the lot — a paywall stub looks much the same — so callers
	// escalate on it without condemning the whole host.
	challengeEmptyPage challengeSignal = "empty-page"
)

// challengeScanBytes caps how much of the body we scan for markers. Interstitial
// pages are small and their markers sit in the <head>, so there is nothing to
// gain from scanning a multi-megabyte article.
const challengeScanBytes = 200 << 10

// challengeBodyMax is a cheap pre-filter for the emptiness heuristic: past this
// size a document cannot be the near-empty page a challenge serves, so we skip
// measuring it.
const challengeBodyMax = 100 << 10

// challengeMinText is the amount of readable text below which a 200 response is
// treated as an interstitial rather than an article.
//
// Deliberately conservative: legitimate thin pages exist — a paywall stub is a
// teaser of a few hundred characters — and flagging those would send perfectly
// ordinary articles to the browser. The named providers are already covered by
// scriptMarkers, so this only has to be a net for the ones we don't know.
const challengeMinText = 300

// scriptMarkers are challenge-runtime asset paths and cookie names. They cannot
// plausibly appear in editorial prose, so they are conclusive on their own —
// including on a 200, which is how Cloudflare serves some interstitials.
var scriptMarkers = map[challengeSignal][]string{
	challengeCloudflare: {
		"/cdn-cgi/challenge-platform/",
		"__cf_chl",
		"cf-browser-verification",
		"challenges.cloudflare.com/turnstile",
	},
	challengeDataDome: {
		"geo.captcha-delivery.com",
		"captcha-delivery.com/captcha",
	},
	challengeIncapsula: {
		"_incapsula_resource",
		"incap_ses_",
	},
}

var (
	scriptStyleRE = regexp.MustCompile(`(?is)<(script|style|noscript)\b[^>]*>.*?</(script|style|noscript)>`)
	tagRE         = regexp.MustCompile(`(?s)<[^>]*>`)
	whitespaceRE  = regexp.MustCompile(`\s+`)
)

// detectChallenge reports whether a response is an anti-bot interstitial rather
// than the page we asked for.
//
// Checking the status code is the part the scraper used to be missing: a 403
// body was handed straight to readability, which is why a challenge only ever
// surfaced later as an article titled "Just a moment...".
func detectChallenge(resp *pageResponse) challengeSignal {
	blocked := resp.StatusCode == http.StatusForbidden ||
		resp.StatusCode == http.StatusServiceUnavailable ||
		resp.StatusCode == http.StatusTooManyRequests

	// Headers first: they identify the protection with no ambiguity.
	if resp.Header.Get("Cf-Mitigated") != "" {
		return challengeCloudflare
	}
	if blocked {
		server := strings.ToLower(resp.Header.Get("Server"))
		if strings.Contains(server, "cloudflare") || resp.Header.Get("Cf-Ray") != "" {
			return challengeCloudflare
		}
		if resp.Header.Get("X-Datadome") != "" || hasCookie(resp.Header, "datadome") {
			return challengeDataDome
		}
		if hasCookie(resp.Header, "incap_ses") || hasCookie(resp.Header, "visid_incap") {
			return challengeIncapsula
		}
	}

	scan := resp.Body
	if len(scan) > challengeScanBytes {
		scan = scan[:challengeScanBytes]
	}
	lower := strings.ToLower(string(scan))

	for signal, markers := range scriptMarkers {
		for _, marker := range markers {
			if strings.Contains(lower, marker) {
				return signal
			}
		}
	}

	// Last net, for protections we don't have markers for. What sets an
	// interstitial apart is not its size but that it carries nothing to read: a
	// title, a script, a spinner. Measuring text instead of matching known page
	// titles is what makes this work in any language — an Italian interstitial
	// is no wordier than an English one, and a title list never would be.
	//
	// Restricted to 2xx: a blocked status is handled just below, and any other
	// error status is a genuine failure whose short error page must not be
	// mistaken for an interstitial — sending a 404 to a browser solves nothing.
	success := resp.StatusCode >= 200 && resp.StatusCode < 300
	if success && len(resp.Body) < challengeBodyMax && visibleTextLen(lower) < challengeMinText {
		return challengeEmptyPage
	}

	// A blocked status with no identifiable marker is still a block; escalating
	// to the browser is the only thing left to try.
	if blocked {
		return challengeGeneric
	}

	return challengeNone
}

// visibleTextLen approximates how many characters of readable text an HTML
// document carries, ignoring markup, scripts and styles. Counting runes rather
// than bytes keeps the measure comparable across alphabets.
func visibleTextLen(body string) int {
	text := scriptStyleRE.ReplaceAllString(body, " ")
	text = tagRE.ReplaceAllString(text, " ")
	text = whitespaceRE.ReplaceAllString(text, " ")
	return utf8.RuneCountInString(strings.TrimSpace(text))
}

// hasCookie reports whether the response sets a cookie whose name starts with
// prefix.
func hasCookie(h http.Header, prefix string) bool {
	for _, c := range h.Values("Set-Cookie") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(c)), prefix) {
			return true
		}
	}
	return false
}
