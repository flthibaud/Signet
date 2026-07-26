package service

import (
	"errors"
	"net/http"
	"regexp"
	"strings"
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
)

// challengeScanBytes caps how much of the body we scan for markers. Interstitial
// pages are small and their markers sit in the <head>, so there is nothing to
// gain from scanning a multi-megabyte article.
const challengeScanBytes = 200 << 10

// challengeBodyMax is the size above which a title match alone is not enough to
// call a page a challenge: a real interstitial is a near-empty document, while a
// full article merely *mentioning* "just a moment" is not blocked.
const challengeBodyMax = 100 << 10

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

// titleMarkers are interstitial page titles, lowercased. They are localised and
// change often, so they only count as a signal on a small page (see
// challengeBodyMax) — this list will never be exhaustive.
var titleMarkers = []string{
	"just a moment",
	"un instant",
	"client challenge",
	"attention required!",
	"checking your browser",
	"verifying you are human",
	"vérification",
	"access denied",
	"accès refusé",
	"security check",
}

var titleRE = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

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

	// Title match: only trusted on a blocked status or a suspiciously small
	// document, to avoid flagging an article that merely quotes the phrase.
	if blocked || len(resp.Body) < challengeBodyMax {
		title := strings.ToLower(pageTitle(lower))
		for _, marker := range titleMarkers {
			if strings.Contains(title, marker) {
				return challengeGeneric
			}
		}
	}

	// A blocked status with no identifiable marker is still a block; escalating
	// to the browser is the only thing left to try.
	if blocked {
		return challengeGeneric
	}

	return challengeNone
}

// pageTitle extracts the <title> text, or "" when there is none.
func pageTitle(body string) string {
	m := titleRE.FindStringSubmatch(body)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
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
