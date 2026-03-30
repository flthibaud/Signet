package readability

import (
	"regexp"

	"golang.org/x/net/html"
)

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
