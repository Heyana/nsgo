package core

import (
	"strings"
	"unicode/utf8"
)

// Text is laid out once: the engine prepares it with the platform's
// TextEngine, measures it at the widths layout asks for and lays it out
// at its final size. The main thread only draws the result.

// AttributedText is styled text for a TextEngine.
type AttributedText struct {
	Text  string
	Style TextStyle
	// Spans style byte ranges of Text over Style, in order, without gaps
	// or overlaps where they cover it.
	Spans []TextSpan
	// MaxLines limits the lines shown, ending the last with an ellipsis;
	// 0 shows every line.
	MaxLines int
}

// TextSpan styles the bytes from Start to End of an AttributedText.
type TextSpan struct {
	Start, End int
	Style      TextStyle
	// Link marks a span with a press handler: a press on it reports
	// EventPress with Index set to a byte offset inside it.
	Link bool
}

func (a *AttributedText) equal(b *AttributedText) bool {
	if a.Text != b.Text || a.Style != b.Style || a.MaxLines != b.MaxLines || len(a.Spans) != len(b.Spans) {
		return false
	}
	for i := range a.Spans {
		if a.Spans[i] != b.Spans[i] {
			return false
		}
	}
	return true
}

// TextStyle is resolved text style: nothing in it is unset.
type TextStyle struct {
	Font  Font
	Color Color
	// LineHeight is the height of a line; 0 is the font's.
	LineHeight    Points
	LetterSpacing Points
	Align         TextAlign
	Decoration    TextDecoration
}

// Font is what a text engine needs to pick a font.
type Font struct {
	// Family is a font family name; empty is the system font.
	Family string
	Size   Points
	Weight FontWeight
	Italic bool
}

// ApproxTextEngine measures text without a platform: every character is
// 0.6 of the font size wide, lines are 1.2 of it tall, and lines wrap at
// spaces. Headless tests use it.
type ApproxTextEngine struct{}

// Prepare implements TextEngine.
func (ApproxTextEngine) Prepare(t *AttributedText) PreparedText {
	return approxText{*t}
}

type approxText struct{ t AttributedText }

// ApproxLayout is the TextLayout of ApproxTextEngine: the text's lines.
type ApproxLayout struct {
	Lines []string
	Size  Size
}

// Release implements TextLayout.
func (*ApproxLayout) Release() {}

func (a approxText) charWidth() Points { return a.t.Style.Font.Size*0.6 + a.t.Style.LetterSpacing }
func (a approxText) lineHeight() Points {
	if h := a.t.Style.LineHeight; h > 0 {
		return h
	}
	return a.t.Style.Font.Size * 1.2
}

func (a approxText) lines(maxWidth Points) []string {
	var out []string
	cw := a.charWidth()
	for _, para := range strings.Split(a.t.Text, "\n") {
		if maxWidth.IsAuto() || cw <= 0 {
			out = append(out, para)
			continue
		}
		perLine := max(int(maxWidth/cw), 1)
		line := ""
		for _, word := range strings.Fields(para) {
			for utf8.RuneCountInString(word) > perLine {
				if line != "" {
					out = append(out, line)
					line = ""
				}
				r := []rune(word)
				out = append(out, string(r[:perLine]))
				word = string(r[perLine:])
			}
			switch {
			case line == "":
				line = word
			case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) <= perLine:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		out = append(out, line)
	}
	if n := a.t.MaxLines; n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

func (a approxText) size(lines []string) Size {
	var w Points
	for _, l := range lines {
		w = max(w, Points(utf8.RuneCountInString(l))*a.charWidth())
	}
	return Size{w, Points(len(lines)) * a.lineHeight()}
}

func (a approxText) Measure(maxWidth Points) Size { return a.size(a.lines(maxWidth)) }

func (a approxText) Layout(size Size) TextLayout {
	lines := a.lines(size.Width + 0.01)
	return &ApproxLayout{Lines: lines, Size: size}
}

func (approxText) Release() {}
