package service

import (
	"net/http"
	"strings"
	"testing"
)

func TestDetectChallenge(t *testing.T) {
	// A plausible article page, big enough to clear challengeBodyMax so the
	// "small page" heuristics don't apply to it.
	article := "<html><head><title>Un article normal</title></head><body>" +
		strings.Repeat("<p>du contenu editorial</p>", 6000) +
		"</body></html>"

	tests := []struct {
		name   string
		status int
		header http.Header
		body   string
		want   challengeSignal
	}{
		{
			name:   "clean article",
			status: 200,
			header: http.Header{"Server": {"nginx"}},
			body:   article,
			want:   challengeNone,
		},
		{
			name:   "cloudflare 403",
			status: 403,
			header: http.Header{"Server": {"cloudflare"}},
			body:   "<html><head><title>Just a moment...</title></head><body></body></html>",
			want:   challengeCloudflare,
		},
		{
			name:   "cloudflare IUAM served as 503",
			status: 503,
			header: http.Header{"Cf-Ray": {"8b0c0000abcdef"}},
			body:   "<html><body>checking</body></html>",
			want:   challengeCloudflare,
		},
		{
			name:   "cf-mitigated header on a 200",
			status: 200,
			header: http.Header{"Cf-Mitigated": {"challenge"}},
			body:   article,
			want:   challengeCloudflare,
		},
		{
			name:   "cloudflare challenge script on a 200",
			status: 200,
			header: http.Header{"Server": {"cloudflare"}},
			body:   `<html><head><script src="/cdn-cgi/challenge-platform/h/b/scripts/jsd/main.js"></script></head><body></body></html>`,
			want:   challengeCloudflare,
		},
		{
			name:   "datadome block",
			status: 403,
			header: http.Header{"X-Datadome": {"protected"}},
			body:   "<html><body></body></html>",
			want:   challengeDataDome,
		},
		{
			name:   "datadome captcha script",
			status: 200,
			header: http.Header{},
			body:   `<html><body><script src="https://geo.captcha-delivery.com/captcha/?initialCid=x"></script></body></html>`,
			want:   challengeDataDome,
		},
		{
			name:   "incapsula resource",
			status: 200,
			header: http.Header{},
			body:   `<html><body><iframe src="/_Incapsula_Resource?SWUDNSAI=31"></iframe></body></html>`,
			want:   challengeIncapsula,
		},
		{
			name:   "localised interstitial title on a small page",
			status: 200,
			header: http.Header{},
			body:   "<html><head><title>Un instant…</title></head><body></body></html>",
			want:   challengeGeneric,
		},
		{
			name:   "client challenge title",
			status: 200,
			header: http.Header{},
			body:   "<html><head><title>Client Challenge</title></head><body></body></html>",
			want:   challengeGeneric,
		},
		{
			name:   "unattributed block is still a block",
			status: 403,
			header: http.Header{"Server": {"nginx"}},
			body:   "<html><body>nope</body></html>",
			want:   challengeGeneric,
		},
		{
			name:   "article quoting an interstitial phrase is not a challenge",
			status: 200,
			header: http.Header{},
			body: `<html><head><title>Just a moment, disent-ils : enquete sur Cloudflare</title></head><body>` +
				strings.Repeat("<p>analyse</p>", 10000) + "</body></html>",
			want: challengeNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectChallenge(&pageResponse{
				StatusCode: tt.status,
				Header:     tt.header,
				Body:       []byte(tt.body),
			})
			if got != tt.want {
				t.Errorf("detectChallenge() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasCookie(t *testing.T) {
	h := http.Header{"Set-Cookie": {
		"session=abc; Path=/",
		"datadome=xyz; Max-Age=31536000",
	}}

	if !hasCookie(h, "datadome") {
		t.Error("expected datadome cookie to be found")
	}
	if hasCookie(h, "incap_ses") {
		t.Error("did not expect incap_ses cookie to be found")
	}
}

func TestPageTitle(t *testing.T) {
	tests := []struct {
		body string
		want string
	}{
		{"<html><head><title>Hello</title></head></html>", "Hello"},
		{`<html><head><title lang="fr"> Bonjour </title></head></html>`, "Bonjour"},
		{"<html><head></head></html>", ""},
	}

	for _, tt := range tests {
		if got := pageTitle(tt.body); got != tt.want {
			t.Errorf("pageTitle(%q) = %q, want %q", tt.body, got, tt.want)
		}
	}
}
