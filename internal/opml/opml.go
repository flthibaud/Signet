package opml

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"golang.org/x/net/html/charset"
)

// Entry is one feed found in a subscription list.
type Entry struct {
	Title   string
	XMLURL  string
	HTMLURL string
	Folder  string
}

// FolderSeparator joins the levels of a nested outline into one folder name.
const FolderSeparator = " / "

// maxFolderDepth bounds how many levels are folded into a name.
const maxFolderDepth = 5

type document struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    head     `xml:"head"`
	Body    body     `xml:"body"`
}

type head struct {
	Title       string `xml:"title,omitempty"`
	DateCreated string `xml:"dateCreated,omitempty"`
}

type body struct {
	Outlines []outline `xml:"outline"`
}

type outline struct {
	Text     string    `xml:"text,attr,omitempty"`
	Type     string    `xml:"type,attr,omitempty"`
	XMLURL   string    `xml:"xmlUrl,attr,omitempty"`
	HTMLURL  string    `xml:"htmlUrl,attr,omitempty"`
	Children []outline `xml:"outline,omitempty"`
}

// UnmarshalXML matches attributes case-insensitively.
func (o *outline) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	for _, attr := range start.Attr {
		value := strings.TrimSpace(attr.Value)
		switch {
		case strings.EqualFold(attr.Name.Local, "text"):
			o.Text = value
		case strings.EqualFold(attr.Name.Local, "title"):
			if o.Text == "" {
				o.Text = value
			}
		case strings.EqualFold(attr.Name.Local, "type"):
			o.Type = value
		case strings.EqualFold(attr.Name.Local, "xmlurl"):
			o.XMLURL = value
		case strings.EqualFold(attr.Name.Local, "htmlurl"):
			o.HTMLURL = value
		}
	}

	for {
		token, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		switch t := token.(type) {
		case xml.StartElement:
			if !strings.EqualFold(t.Name.Local, "outline") {
				if err := d.Skip(); err != nil {
					return err
				}
				continue
			}
			var child outline
			if err := child.UnmarshalXML(d, t); err != nil {
				return err
			}
			o.Children = append(o.Children, child)
		case xml.EndElement:
			return nil
		}
	}
}

// Parse reads a subscription list and returns its feeds, flattened.
func Parse(r io.Reader) ([]Entry, error) {
	dec := xml.NewDecoder(r)
	dec.Strict = false
	dec.CharsetReader = charset.NewReaderLabel

	var doc document
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("invalid OPML: %w", err)
	}

	entries := []Entry{}
	collect(doc.Body.Outlines, nil, &entries)

	return entries, nil
}

// collect walks the outline tree depth-first, carrying the folder path down.
func collect(outlines []outline, path []string, entries *[]Entry) {
	for _, o := range outlines {
		if o.XMLURL != "" {
			title := o.Text
			if title == "" {
				title = o.XMLURL
			}

			*entries = append(*entries, Entry{
				Title:   title,
				XMLURL:  o.XMLURL,
				HTMLURL: o.HTMLURL,
				Folder:  strings.Join(path, FolderSeparator),
			})
		}

		if len(o.Children) == 0 {
			continue
		}

		childPath := path
		if o.XMLURL == "" && o.Text != "" && len(path) < maxFolderDepth {
			childPath = append(append([]string{}, path...), o.Text)
		}

		collect(o.Children, childPath, entries)
	}
}

// Write renders entries as an OPML 2.0 subscription list, grouping them by
// folder.
func Write(w io.Writer, title string, entries []Entry) error {
	byFolder := map[string][]outline{}
	folders := []string{}

	for _, e := range entries {
		if _, seen := byFolder[e.Folder]; !seen && e.Folder != "" {
			folders = append(folders, e.Folder)
		}

		text := e.Title
		if text == "" {
			text = e.XMLURL
		}

		byFolder[e.Folder] = append(byFolder[e.Folder], outline{
			Text:    text,
			Type:    "rss",
			XMLURL:  e.XMLURL,
			HTMLURL: e.HTMLURL,
		})
	}

	slices.Sort(folders)

	outlines := make([]outline, 0, len(folders)+len(byFolder[""]))
	for _, name := range folders {
		outlines = append(outlines, outline{Text: name, Children: byFolder[name]})
	}

	outlines = append(outlines, byFolder[""]...)

	doc := document{
		Version: "2.0",
		Head: head{
			Title:       title,
			DateCreated: time.Now().UTC().Format(time.RFC1123Z),
		},
		Body: body{Outlines: outlines},
	}

	if _, err := io.WriteString(w, xml.Header); err != nil {
		return err
	}

	enc := xml.NewEncoder(w)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return err
	}

	_, err := io.WriteString(w, "\n")
	return err
}
