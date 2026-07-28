package service

import "testing"

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"scheme is folded onto https", "http://example.com/article", "https://example.com/article"},
		{"scheme case", "HTTPS://Example.COM/Article", "https://example.com/Article"},
		{"www prefix", "https://www.example.com/article", "https://example.com/article"},
		{"default ports", "https://example.com:443/article", "https://example.com/article"},
		{"non default port kept", "https://example.com:8443/article", "https://example.com:8443/article"},
		{"trailing slash", "https://example.com/article/", "https://example.com/article"},
		{"root slash", "https://example.com/", "https://example.com"},
		{"fragment", "https://example.com/article#section-2", "https://example.com/article"},
		{"credentials", "https://user:pass@example.com/article", "https://example.com/article"},
		{"utm params", "https://example.com/a?utm_source=rss&utm_medium=feed", "https://example.com/a"},
		{"known tracking param", "https://example.com/a?fbclid=xyz", "https://example.com/a"},
		{"matomo params", "https://example.com/a?pk_campaign=news", "https://example.com/a"},
		{"meaningful params kept", "https://example.com/index.php?p=42", "https://example.com/index.php?p=42"},
		{"param order", "https://example.com/a?b=2&a=1", "https://example.com/a?a=1&b=2"},
		{"mixed params", "https://example.com/a?id=7&utm_source=nl", "https://example.com/a?id=7"},
		{"case of path is preserved", "https://example.com/Article", "https://example.com/Article"},
		{"whitespace", "  https://example.com/article  ", "https://example.com/article"},
		{"escaped path keeps its encoding", "https://example.com/a%20b/", "https://example.com/a%20b"},
		{"relative link untouched", "/article", "/article"},
		{"non http scheme untouched", "mailto:someone@example.com", "mailto:someone@example.com"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeURL(tt.in); got != tt.want {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// The point of normalizing is that the variants a page is linked as across feeds
// all land on one article row.
func TestHashURLDedupesVariants(t *testing.T) {
	want := hashURL("https://example.com/article")

	variants := []string{
		"http://example.com/article",
		"https://www.example.com/article/",
		"https://example.com:443/article",
		"https://example.com/article?utm_source=feedly&utm_medium=rss",
		"https://example.com/article#top",
	}

	for _, v := range variants {
		if got := hashURL(v); got != want {
			t.Errorf("hashURL(%q) does not match the canonical hash", v)
		}
	}

	if hashURL("https://example.com/other") == want {
		t.Error("distinct articles hash to the same key")
	}
}

func TestItemURL(t *testing.T) {
	tests := []struct {
		name string
		link string
		guid string
		want string
	}{
		{"link wins", "https://example.com/a", "urn:uuid:1234", "https://example.com/a"},
		{"permalink guid fallback", "", "https://example.com/a", "https://example.com/a"},
		{"blank link falls back", "   ", "https://example.com/a", "https://example.com/a"},
		{"opaque guid is not a url", "", "urn:uuid:1234", ""},
		{"relative guid is not a url", "", "/a", ""},
		{"nothing usable", "", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := itemURL(tt.link, tt.guid); got != tt.want {
				t.Errorf("itemURL(%q, %q) = %q, want %q", tt.link, tt.guid, got, tt.want)
			}
		})
	}
}
