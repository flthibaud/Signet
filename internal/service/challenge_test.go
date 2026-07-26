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
			name:   "unknown provider interstitial on a 200",
			status: 200,
			header: http.Header{},
			body: `<html><head><title>Client Challenge</title></head><body>` +
				`<script>window.rbzns={challenge:1}</script><div id="spinner"></div></body></html>`,
			want: challengeEmptyPage,
		},
		{
			// The point of measuring text rather than matching titles: a language
			// we have no marker list for is detected all the same.
			name:   "italian interstitial",
			status: 200,
			header: http.Header{},
			body: `<html><head><title>Un momento…</title></head><body>` +
				`<p>Verifica che la connessione sia sicura.</p><script src="/verify.js"></script></body></html>`,
			want: challengeEmptyPage,
		},
		{
			name:   "script-heavy but text-free page",
			status: 200,
			header: http.Header{},
			body: `<html><head><title>…</title><script>` + strings.Repeat("var x=1;", 4000) +
				`</script></head><body></body></html>`,
			want: challengeEmptyPage,
		},
		{
			// Short like an interstitial, but a genuine failure: escalating it to
			// a browser would solve nothing.
			name:   "404 error page is not a challenge",
			status: 404,
			header: http.Header{},
			body:   "<html><head><title>404</title></head><body><p>Page introuvable.</p></body></html>",
			want:   challengeNone,
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
		{
			// A paywall teaser is a legitimately thin page: flagging it would send
			// ordinary articles to the browser for nothing.
			name:   "paywall stub is not a challenge",
			status: 200,
			header: http.Header{},
			body: `<html><head><title>Le titre de l'article</title></head><body><article>` +
				strings.Repeat("<p>Le debut de l'article, en acces libre.</p>", 12) +
				`<p>Abonnez-vous pour lire la suite.</p></article></body></html>`,
			want: challengeNone,
		},
		{
			name:   "short but real article",
			status: 200,
			header: http.Header{},
			body: `<html><head><title>Breve</title></head><body>` +
				strings.Repeat("<p>Une depeche courte mais bien reelle.</p>", 10) + `</body></html>`,
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

func TestVisibleTextLen(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"plain text", "<p>abcde</p>", 5},
		{"scripts don't count", `<p>abcde</p><script>var averyLongVariable = 1;</script>`, 5},
		{"styles don't count", `<p>abcde</p><style>body{margin:0;padding:0}</style>`, 5},
		{"whitespace is collapsed", "<p>ab</p>\n\n   \t<p>cd</p>", 5}, // "ab cd"
		{"markup alone is nothing", `<div class="wrapper"><br/><img src="x.png"></div>`, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := visibleTextLen(tt.body); got != tt.want {
				t.Errorf("visibleTextLen(%q) = %d, want %d", tt.body, got, tt.want)
			}
		})
	}

	// Runes, not bytes: a non-Latin script must not read as more text than it is.
	if got := visibleTextLen("<p>日本語のテキスト</p>"); got != 8 {
		t.Errorf("visibleTextLen(japanese) = %d, want 8", got)
	}
}
