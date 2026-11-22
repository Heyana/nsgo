package updates

import (
	"strings"

	"golang.org/x/net/html"
)

// NotesText is an appcast item's description, HTML as nsgo build writes it from -release-notes' Markdown, as the plain text the updater's prompt shows: a line for each paragraph, heading and list item, list items behind "• ", runs of spaces made one and blank lines between blocks dropped.
func NotesText(description string) string {
	z := html.NewTokenizer(strings.NewReader(description))
	var lines []string
	var line strings.Builder
	end := func() {
		s := strings.Join(strings.Fields(line.String()), " ")
		if s == "•" {
			// A list item's bullet waits for its text, which a paragraph inside the item starts.
			return
		}
		if s != "" {
			lines = append(lines, s)
		}
		line.Reset()
	}
	// A feed another tool wrote may describe the release with a whole page, whose script, style and title are no text of the notes.
	skip := 0
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			end()
			return strings.Join(lines, "\n")
		case html.TextToken:
			if skip == 0 {
				line.Write(z.Text())
			}
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			switch string(name) {
			case "script", "style", "title", "head":
				if tt == html.StartTagToken {
					skip++
				} else if tt == html.EndTagToken && skip > 0 {
					skip--
				}
				continue
			}
			switch string(name) {
			case "p", "div", "br", "h1", "h2", "h3", "h4", "h5", "h6", "ul", "ol", "pre", "blockquote", "hr", "tr":
				end()
			case "li":
				end()
				if tt == html.StartTagToken {
					line.WriteString("• ")
				}
			}
		}
	}
}
