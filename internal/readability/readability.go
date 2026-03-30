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
