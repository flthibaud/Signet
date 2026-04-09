package readability

import "golang.org/x/net/html"

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
