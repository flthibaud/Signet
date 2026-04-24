package readability

import (
	"strings"

	"golang.org/x/net/html"
)

// removalRules defines which elements to strip from the DOM.
// Each rule returns true if the node should be removed (subtree skipped).
var removalRules = []func(*html.Node) bool{
	hasAttrRule("data-nosnippet"), // ads, promos, PWA install
	hasAttrRule("data-vendor"),    // consent blocks
	hasAttrRule("data-tracking"),
	tagRule("picture"),                               // lazy-load wrappers with duplicate imgs
	hasIDRule("div", "download"),                      // Numerama app download widget
	hasIDRule("div", "js-modal-gifted-url"),          // Le Monde gifted modal
	hasIDRule("section", "js-capping"),               // Le Monde paywall cap
	hasIDRule("section", "js-capping-old-article"),   // Le Monde paywall cap (old)
	func(n *html.Node) bool {
		return hasAttr(n, "tabindex") && getAttr(n, "tabindex") == "-1"
	},
	func(n *html.Node) bool {
		return n.Data == "div" && isRelatedContentWidget(n)
	},
}

// hasAttrRule removes any element that has the given attribute.
func hasAttrRule(attr string) func(*html.Node) bool {
	return func(n *html.Node) bool { return hasAttr(n, attr) }
}

// tagRule removes any element with the given tag name.
func tagRule(tag string) func(*html.Node) bool {
	return func(n *html.Node) bool { return n.Data == tag }
}

// hasIDRule removes elements matching <tag id="id">.
func hasIDRule(tag, id string) func(*html.Node) bool {
	return func(n *html.Node) bool { return n.Data == tag && getAttr(n, "id") == id }
}

// preprocessDOM cleans the DOM before markdown conversion.
func preprocessDOM(root *html.Node) {
	var toRemove []*html.Node
	var youtubeIframes []*html.Node

	walkTree(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}

		for _, rule := range removalRules {
			if rule(n) {
				toRemove = append(toRemove, n)
				return false
			}
		}

		if n.Data == "iframe" && isYouTubeEmbed(n) {
			youtubeIframes = append(youtubeIframes, n)
			return false
		}

		if n.Data == "img" {
			fixLazyImage(n)
		}

		return true
	})

	for _, n := range toRemove {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}

	for _, iframe := range youtubeIframes {
		replaceYouTubeIframe(iframe)
	}

	removeEmptyContainers(root)
	removeHRBeforeEmbeddedTag(root)
	removeTrailingHR(root)
}

// --- Lazy image fix ---

func fixLazyImage(n *html.Node) {
	src := getAttr(n, "src")
	dataSrc := getAttr(n, "data-src")

	if dataSrc != "" && (src == "" || strings.HasPrefix(src, "data:")) {
		setAttr(n, "src", dataSrc)
		removeAttr(n, "data-src")
	}
}

// --- Related content widget detection ---

var relatedContentPatterns = []string{
	"pour aller plus loin",
	"à lire aussi",
	"sur le même sujet",
}

func isRelatedContentWidget(n *html.Node) bool {
	first := firstElementChild(n)
	if first == nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(textContent(first)))
	for _, pattern := range relatedContentPatterns {
		if text == pattern {
			return true
		}
	}
	return false
}

// --- Empty container cleanup ---

func removeEmptyContainers(n *html.Node) {
	var toRemove []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			removeEmptyContainers(c)
			if isContainer(c) && isEffectivelyEmpty(c) {
				toRemove = append(toRemove, c)
			}
		}
	}
	for _, c := range toRemove {
		n.RemoveChild(c)
	}
}

func isContainer(n *html.Node) bool {
	switch n.Data {
	case "div", "section", "aside", "span", "figure":
		return true
	}
	return false
}

func isEffectivelyEmpty(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			return false
		}
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) != "" {
			return false
		}
	}
	return true
}

// --- HR + embedded tag removal ---

// removeHRBeforeEmbeddedTag removes any <hr> that is immediately followed by a
// <div id="embedded-tag-*">, along with all subsequent siblings (the tag cloud
// section at the bottom of Numerama articles).
func removeHRBeforeEmbeddedTag(root *html.Node) {
	type cutPoint struct{ parent, from *html.Node }
	var cuts []cutPoint

	walkTree(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "hr" {
			next := nextElementSibling(n)
			if next != nil && next.Data == "div" && strings.HasPrefix(getAttr(next, "id"), "embedded-tag-") {
				cuts = append(cuts, cutPoint{n.Parent, n})
			}
		}
		return true
	})

	for _, cp := range cuts {
		var toRemove []*html.Node
		for s := cp.from; s != nil; s = s.NextSibling {
			toRemove = append(toRemove, s)
		}
		for _, s := range toRemove {
			cp.parent.RemoveChild(s)
		}
	}
}

// --- Trailing HR removal ---

func removeTrailingHR(root *html.Node) {
	n := root
	for {
		last := lastElementChild(n)
		if last == nil {
			return
		}
		if last.Data == "hr" {
			last.Parent.RemoveChild(last)
			return
		}
		if isContainer(last) {
			n = last
		} else {
			return
		}
	}
}
