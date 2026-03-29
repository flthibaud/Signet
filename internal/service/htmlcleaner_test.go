package service

import (
	"strings"
	"testing"
)

func TestCleanHTML(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantIn     []string // substrings that MUST be present
		wantNotIn  []string // substrings that MUST NOT be present
	}{
		{
			name:   "empty input",
			input:  "",
			wantIn: []string{""},
		},
		{
			name:      "iframe removed by bluemonday",
			input:     `<p>Before</p><iframe src="https://youtube.com/embed/abc"></iframe><p>After</p>`,
			wantIn:    []string{"Before", "After"},
			wantNotIn: []string{"iframe", "youtube"},
		},
		{
			name:      "object and embed removed",
			input:     `<p>Text</p><object data="x.swf"></object><embed src="y.swf">`,
			wantIn:    []string{"Text"},
			wantNotIn: []string{"object", "embed", ".swf"},
		},
		{
			name:      "data-nosnippet element removed",
			input:     `<p>Article content</p><div data-nosnippet=""><p>Buy Nintendo Switch!</p></div><p>More content</p>`,
			wantIn:    []string{"Article content", "More content"},
			wantNotIn: []string{"Nintendo Switch"},
		},
		{
			name:      "data-vendor element removed",
			input:     `<p>Content</p><div data-vendor="c:youtube"><p>Cookie consent text</p></div>`,
			wantIn:    []string{"Content"},
			wantNotIn: []string{"Cookie consent", "data-vendor"},
		},
		{
			name:      "lazy image fixed",
			input:     `<img src="data:image/svg+xml;utf8,placeholder" data-src="https://example.com/real.jpg" alt="photo">`,
			wantIn:    []string{"https://example.com/real.jpg", "photo"},
			wantNotIn: []string{"data:image/svg+xml"},
		},
		{
			name:      "lazy image not touched when src is real",
			input:     `<img src="https://example.com/real.jpg" alt="photo">`,
			wantIn:    []string{"https://example.com/real.jpg"},
			wantNotIn: []string{},
		},
		{
			name:      "picture/source stripped by bluemonday, img inside picture preserved",
			input:     `<picture><source data-srcset="https://example.com/img.webp 1024w" type="image/webp"><img src="data:placeholder" data-src="https://example.com/img.jpg" alt="test"></picture>`,
			wantIn:    []string{"https://example.com/img.jpg"},
			wantNotIn: []string{"data:placeholder", "source"},
		},
		{
			name:      "empty container removed after child removal",
			input:     `<p>Keep</p><div><div data-nosnippet=""><p>Ad</p></div></div>`,
			wantIn:    []string{"Keep"},
			wantNotIn: []string{"Ad"},
		},
		{
			name:      "clean content passes through",
			input:     `<p>Hello <strong>world</strong></p><h2>Title</h2><ul><li>Item</li></ul>`,
			wantIn:    []string{"Hello", "<strong>world</strong>", "<h2>Title</h2>", "<li>Item</li>"},
			wantNotIn: []string{},
		},
		{
			name:      "mixed cleanup: iframe + data-nosnippet + lazy image",
			input:     `<p>Article</p><iframe src="https://youtube.com/x"></iframe><div data-nosnippet=""><p>Ad</p></div><img src="data:placeholder" data-src="https://real.jpg" alt="img">`,
			wantIn:    []string{"Article", "https://real.jpg"},
			wantNotIn: []string{"iframe", "youtube", "Ad", "data:placeholder"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cleanHTML(tt.input)

			for _, want := range tt.wantIn {
				if !strings.Contains(got, want) {
					t.Errorf("expected output to contain %q, got:\n%s", want, got)
				}
			}
			for _, notWant := range tt.wantNotIn {
				if strings.Contains(got, notWant) {
					t.Errorf("expected output NOT to contain %q, got:\n%s", notWant, got)
				}
			}
		})
	}
}
