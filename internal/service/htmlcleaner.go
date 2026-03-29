package service

import (
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/net/html"
)

// cleanHTML applies post-readability cleaning to extracted article HTML.
// Step 1: DOM cleanup (remove data-nosnippet/data-vendor elements, fix lazy images).
// Step 2: Bluemonday UGC sanitization (strips iframes, embeds, scripts, etc.).
func cleanHTML(rawHTML string) string {
	if rawHTML == "" {
		return ""
	}

	// Parse the full document (html.Parse wraps in <html><body>)
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return bluemonday.UGCPolicy().Sanitize(rawHTML)
	}

	// Find the <body> node to work with
	container := findBody(doc)
	if container == nil {
		return bluemonday.UGCPolicy().Sanitize(rawHTML)
	}

	// Step 1: DOM cleanup
	var toRemove []*html.Node
	walkTree(container, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}

		// Remove elements with data-nosnippet (ads/promos)
		if hasAttr(n, "data-nosnippet") {
			toRemove = append(toRemove, n)
			return false
		}

		// Remove elements with data-vendor (consent blocks)
		if hasAttr(n, "data-vendor") {
			toRemove = append(toRemove, n)
			return false
		}

		// Fix lazy-loaded images
		if n.Data == "img" {
			fixLazyImage(n)
		}

		// Fix lazy-loaded source elements
		if n.Data == "source" {
			fixLazySource(n)
		}

		return true
	})

	// Remove collected nodes
	for _, n := range toRemove {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}

	// Remove empty containers left after cleanup
	removeEmptyContainers(container)

	// Serialize back to HTML
	var sb strings.Builder
	for c := container.FirstChild; c != nil; c = c.NextSibling {
		html.Render(&sb, c)
	}

	// Step 2: Bluemonday UGC sanitization
	return bluemonday.UGCPolicy().Sanitize(sb.String())
}

// walkTree performs a depth-first traversal of the DOM tree.
// The callback returns false to skip children of the current node.
func walkTree(n *html.Node, fn func(*html.Node) bool) {
	if !fn(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkTree(c, fn)
	}
}

// fixLazyImage swaps data-src into src when src is a placeholder.
func fixLazyImage(n *html.Node) {
	src := getAttr(n, "src")
	dataSrc := getAttr(n, "data-src")

	if dataSrc != "" && (src == "" || strings.HasPrefix(src, "data:")) {
		setAttr(n, "src", dataSrc)
		removeAttr(n, "data-src")
	}
}

// fixLazySource swaps data-srcset into srcset.
func fixLazySource(n *html.Node) {
	srcset := getAttr(n, "srcset")
	dataSrcset := getAttr(n, "data-srcset")

	if dataSrcset != "" && srcset == "" {
		setAttr(n, "srcset", dataSrcset)
		removeAttr(n, "data-srcset")
	}
}

// removeEmptyContainers removes div/section/aside elements that contain only whitespace.
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

// Attribute helpers

func hasAttr(n *html.Node, key string) bool {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return true
		}
	}
	return false
}

func getAttr(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

func setAttr(n *html.Node, key, val string) {
	for i, attr := range n.Attr {
		if attr.Key == key {
			n.Attr[i].Val = val
			return
		}
	}
	n.Attr = append(n.Attr, html.Attribute{Key: key, Val: val})
}

// findBody finds the <body> element in a parsed HTML document.
func findBody(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "body" {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if result := findBody(c); result != nil {
			return result
		}
	}
	return nil
}

func removeAttr(n *html.Node, key string) {
	attrs := make([]html.Attribute, 0, len(n.Attr))
	for _, attr := range n.Attr {
		if attr.Key != key {
			attrs = append(attrs, attr)
		}
	}
	n.Attr = attrs
}
