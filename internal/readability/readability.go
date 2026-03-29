package readability

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"golang.org/x/net/html"
)

// Readability converts HTML output from go-readability into clean Markdown.
type Readability struct{}

func NewReadability() *Readability {
	return &Readability{}
}

// HTMLToMarkdown takes readability-extracted HTML and returns clean Markdown.
func (r *Readability) HTMLToMarkdown(rawHTML string) (string, error) {
	if rawHTML == "" {
		return "", nil
	}

	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		return "", fmt.Errorf("parsing HTML: %w", err)
	}

	body := findBody(doc)
	if body == nil {
		return "", fmt.Errorf("no body element found")
	}

	preprocessDOM(body)

	var sb strings.Builder
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		html.Render(&sb, c)
	}

	conv := converter.NewConverter(
		converter.WithPlugins(
			base.NewBasePlugin(),
			commonmark.NewCommonmarkPlugin(),
			table.NewTablePlugin(
				table.WithCellPaddingBehavior(table.CellPaddingBehaviorMinimal),
			),
		),
	)

	md, err := conv.ConvertString(sb.String())
	if err != nil {
		return "", fmt.Errorf("converting to markdown: %w", err)
	}

	md = postProcessMarkdown(md)

	return strings.TrimSpace(md), nil
}

// preprocessDOM cleans the DOM before markdown conversion.
func preprocessDOM(root *html.Node) {
	var toRemove []*html.Node
	var youtubeIframes []*html.Node

	walkTree(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}

		// Remove elements with data-nosnippet (ads, promos, PWA install)
		if hasAttr(n, "data-nosnippet") {
			toRemove = append(toRemove, n)
			return false
		}

		// Remove elements with data-vendor (consent blocks)
		if hasAttr(n, "data-vendor") {
			toRemove = append(toRemove, n)
			return false
		}

		if hasAttr(n, "data-tracking") {
			toRemove = append(toRemove, n)
			return false
		}

		// Remove <picture> elements (lazy-load wrappers with duplicate imgs)
		if n.Data == "picture" {
			toRemove = append(toRemove, n)
			return false
		}

		// Collect YouTube iframes for conversion (before they'd be lost)
		if n.Data == "iframe" && isYouTubeEmbed(n) {
			youtubeIframes = append(youtubeIframes, n)
			return false
		}

		// Fix lazy-loaded images (data-src → src)
		if n.Data == "img" {
			fixLazyImage(n)
		}

		if hasAttr(n, "tabindex") && getAttr(n, "tabindex") == "-1" {
			toRemove = append(toRemove, n)
			return false
		}

		// Remove related content widgets ("Pour aller plus loin", etc.)
		if n.Data == "div" && isRelatedContentWidget(n) {
			toRemove = append(toRemove, n)
			return false
		}

		if n.Data == "div" && getAttr(n, "id") == "js-modal-gifted-url" {
			toRemove = append(toRemove, n)
			return false
		}

		if n.Data == "section" && getAttr(n, "id") == "js-capping" {
			toRemove = append(toRemove, n)
			return false
		}

		if n.Data == "section" && getAttr(n, "id") == "js-capping-old-article" {
			toRemove = append(toRemove, n)
			return false
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
	removeTrailingHR(root)
}

// tableSeparatorRegex matches table separator rows like |---|---| without spaces.
var tableSeparatorRegex = regexp.MustCompile(`\|(-+)`)

// postProcessMarkdown normalizes markdown output (e.g. table separator spacing).
func postProcessMarkdown(md string) string {
	// Add spaces around separator dashes: |---| → | --- |
	md = tableSeparatorRegex.ReplaceAllStringFunc(md, func(match string) string {
		dashes := match[1:] // strip leading |
		return "| " + dashes + " "
	})
	return md
}

// --- YouTube handling ---

var youtubeEmbedRegex = regexp.MustCompile(`youtube\.com/embed/([a-zA-Z0-9_-]+)`)

func isYouTubeEmbed(n *html.Node) bool {
	src := getAttr(n, "src")
	if src == "" {
		src = getAttr(n, "data-src")
	}
	return youtubeEmbedRegex.MatchString(src)
}

func replaceYouTubeIframe(n *html.Node) {
	src := getAttr(n, "src")
	if src == "" {
		src = getAttr(n, "data-src")
	}

	matches := youtubeEmbedRegex.FindStringSubmatch(src)
	if len(matches) < 2 || n.Parent == nil {
		return
	}

	videoID := matches[1]
	title := getAttr(n, "title")

	link := &html.Node{
		Type: html.ElementNode,
		Data: "a",
		Attr: []html.Attribute{{Key: "href", Val: "https://www.youtube.com/watch?v=" + videoID}},
	}
	img := &html.Node{
		Type: html.ElementNode,
		Data: "img",
		Attr: []html.Attribute{
			{Key: "src", Val: "https://img.youtube.com/vi/" + videoID + "/0.jpg"},
			{Key: "alt", Val: title},
		},
	}
	link.AppendChild(img)

	n.Parent.InsertBefore(link, n)
	n.Parent.RemoveChild(n)
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

// --- DOM helpers ---

func walkTree(n *html.Node, fn func(*html.Node) bool) {
	if !fn(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkTree(c, fn)
	}
}

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

func firstElementChild(n *html.Node) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			return c
		}
	}
	return nil
}

func lastElementChild(n *html.Node) *html.Node {
	var last *html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode {
			last = c
		}
	}
	return last
}

func textContent(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(textContent(c))
	}
	return sb.String()
}

// --- Attribute helpers ---

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

func removeAttr(n *html.Node, key string) {
	attrs := make([]html.Attribute, 0, len(n.Attr))
	for _, attr := range n.Attr {
		if attr.Key != key {
			attrs = append(attrs, attr)
		}
	}
	n.Attr = attrs
}
