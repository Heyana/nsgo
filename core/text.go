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
	// Attachments are views inside the text, in order: each takes the
	// place of an AttachmentRune of Text, which a span styles.
	Attachments []TextAttachment
}

// TextSpan styles the bytes from Start to End of an AttributedText.
type TextSpan struct {
	Start, End int
	Style      TextStyle
	// Link marks a span with a press handler: a press on it reports
	// EventPress with Index set to a byte offset inside it.
	Link bool
	// Background is drawn behind the span's characters: a box per line
	// from the font's ascent to its descent, behind the selection and
	// the glyphs.
	Background TextBackground
}

// TextBackground is the background of a span, from its style's
// Background and CornerRadius. It is drawing only: it changes no line.
type TextBackground struct {
	Color Color
	// Radius rounds each line's box, at most to half its height.
	Radius Points
}

// AttachmentRune stands in a text for an attachment, as the object
// replacement character does in attributed strings.
const AttachmentRune = '\uFFFC'

// TextAttachment is a view inside a text, sized by layout before the text
// is prepared. Every engine places it with the same rule: its top Ascent
// above the baseline, its line reserving Extent, so a line grows where an
// attachment reaches past the font around it.
type TextAttachment struct {
	// Offset is the byte offset of its AttachmentRune.
	Offset int
	Size   Size
	Align  VerticalAlign
}

// FontExtent is the vertical metrics of the font around an attachment:
// its placeholder's.
type FontExtent struct {
	// Ascent and Descent reach above and below the baseline; both are
	// positive.
	Ascent, Descent Points
	XHeight         Points
}

// Ascent returns how far a's top is above the baseline of a line in font,
// below it when negative: CSS's vertical-align for a replaced inline box.
func (a TextAttachment) Ascent(font FontExtent) Points {
	h := a.Size.Height
	switch a.Align {
	case VerticalAlignTextTop:
		return font.Ascent
	case VerticalAlignTextBottom:
		return h - font.Descent
	case VerticalAlignMiddle:
		return (h + font.XHeight) / 2
	}
	return h
}

// Extent returns what a's line reserves for it above and below the
// baseline, each at least 0. Engines give the placeholder these metrics
// and draw its top Ascent above the baseline.
func (a TextAttachment) Extent(font FontExtent) (above, below Points) {
	asc := a.Ascent(font)
	return max(asc, 0), max(a.Size.Height-asc, 0)
}

// AttachmentLayout is a TextLayout of text with attachments. The engine
// asks it where each went after laying the text out.
type AttachmentLayout interface {
	// Attachment returns the box of attachment i in the content box, or
	// false when the line limit cut it off.
	Attachment(i int) (Bounds, bool)
}

func (a *AttributedText) equal(b *AttributedText) bool {
	if a.Text != b.Text || a.Style != b.Style || a.MaxLines != b.MaxLines ||
		len(a.Spans) != len(b.Spans) || len(a.Attachments) != len(b.Attachments) {
		return false
	}
	for i := range a.Spans {
		if a.Spans[i] != b.Spans[i] {
			return false
		}
	}
	for i := range a.Attachments {
		if a.Attachments[i] != b.Attachments[i] {
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

// FontMonospace is the family name of the platform's fixed-pitch font,
// for code.
const FontMonospace = "monospace"

// Font is what a text engine needs to pick a font.
type Font struct {
	// Family is a font family name; empty is the system font, and
	// FontMonospace the platform's fixed-pitch font.
	Family string
	Size   Points
	Weight FontWeight
	Italic bool
}

// ApproxTextEngine measures text without a platform: every character is
// 0.6 of the font size wide, lines are 1.2 of it tall, and lines wrap at
// spaces. Its font's ascent is the line, its descent 0 and its x height
// half the font size. Headless tests use it.
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
	// attachments are the boxes of the text's attachments, cut off
	// unless shown.
	attachments []approxBox
}

type approxBox struct {
	box   Bounds
	shown bool
}

// Release implements TextLayout.
func (*ApproxLayout) Release() {}

// Attachment implements AttachmentLayout.
func (l *ApproxLayout) Attachment(i int) (Bounds, bool) {
	if i < 0 || i >= len(l.attachments) {
		return Bounds{}, false
	}
	return l.attachments[i].box, l.attachments[i].shown
}

func (a approxText) charWidth() Points { return a.t.Style.Font.Size*0.6 + a.t.Style.LetterSpacing }
func (a approxText) lineHeight() Points {
	if h := a.t.Style.LineHeight; h > 0 {
		return h
	}
	return a.t.Style.Font.Size * 1.2
}

// approxLine is a line of approxText: its words joined by single spaces,
// and where its attachments sit.
type approxLine struct {
	text           string
	width          Points
	ascent, height Points
	atts           []approxAtt
}

type approxAtt struct {
	index int    // in the text's attachments
	x     Points // from the line's start
}

// attachmentAt returns the index of the attachment at byte offset i.
func (a approxText) attachmentAt(i int) int {
	for k, at := range a.t.Attachments {
		if at.Offset == i {
			return k
		}
	}
	return -1
}

// wordWidth is the width of word, which starts at byte offset at.
func (a approxText) wordWidth(word string, at int) Points {
	var w Points
	for i, r := range word {
		if k := a.attachmentAt(at + i); r == AttachmentRune && k >= 0 {
			w += a.t.Attachments[k].Size.Width
		} else {
			w += a.charWidth()
		}
	}
	return w
}

func (a approxText) lines(maxWidth Points) []approxLine {
	var out []approxLine
	cw := a.charWidth()
	lh := a.lineHeight()
	font := FontExtent{Ascent: lh, XHeight: a.t.Style.Font.Size / 2}
	var line approxLine
	started := false
	flush := func() {
		line.ascent, line.height = lh, lh
		var below Points
		for _, at := range line.atts {
			above, b := a.t.Attachments[at.index].Extent(font)
			line.ascent = max(line.ascent, above)
			below = max(below, b)
		}
		line.height = line.ascent + below
		out = append(out, line)
		line, started = approxLine{}, false
	}
	place := func(word string, at int) {
		if started {
			line.text += " "
			line.width += cw
		}
		for i, r := range word {
			if k := a.attachmentAt(at + i); r == AttachmentRune && k >= 0 {
				line.atts = append(line.atts, approxAtt{index: k, x: line.width})
				line.width += a.t.Attachments[k].Size.Width
			} else {
				line.width += cw
			}
		}
		line.text += word
		started = true
	}
	start := 0
	for _, para := range strings.Split(a.t.Text, "\n") {
		for i := 0; i < len(para); {
			if para[i] == ' ' {
				i++
				continue
			}
			j := i
			for j < len(para) && para[j] != ' ' {
				j++
			}
			word, at := para[i:j], start+i
			i = j
			if maxWidth.IsAuto() || cw <= 0 {
				place(word, at)
				continue
			}
			ww := a.wordWidth(word, at)
			if started && line.width+cw+ww > maxWidth+0.01 {
				flush()
			}
			// A word wider than the line breaks between its characters.
			for ww > maxWidth+0.01 && utf8.RuneCountInString(word) > 1 {
				fit, w := 0, Points(0)
				for k, r := range word {
					rw := cw
					if n := a.attachmentAt(at + k); r == AttachmentRune && n >= 0 {
						rw = a.t.Attachments[n].Size.Width
					}
					if k > 0 && w+rw > maxWidth+0.01 {
						break
					}
					w += rw
					fit = k + utf8.RuneLen(r)
				}
				if fit >= len(word) {
					break
				}
				place(word[:fit], at)
				flush()
				word, at = word[fit:], at+fit
				ww = a.wordWidth(word, at)
			}
			place(word, at)
		}
		flush()
		start += len(para) + 1
	}
	if n := a.t.MaxLines; n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

func (a approxText) size(lines []approxLine) Size {
	var s Size
	for _, l := range lines {
		s.Width = max(s.Width, l.width)
		s.Height += l.height
	}
	return s
}

func (a approxText) Measure(maxWidth Points) Size { return a.size(a.lines(maxWidth)) }

func (a approxText) Layout(size Size) TextLayout {
	lines := a.lines(size.Width + 0.01)
	l := &ApproxLayout{Size: size, attachments: make([]approxBox, len(a.t.Attachments))}
	var y Points
	for _, line := range lines {
		l.Lines = append(l.Lines, line.text)
		for _, at := range line.atts {
			att := a.t.Attachments[at.index]
			font := FontExtent{Ascent: a.lineHeight(), XHeight: a.t.Style.Font.Size / 2}
			top := y + line.ascent - att.Ascent(font)
			l.attachments[at.index] = approxBox{box: Bounds{Origin: Point{at.x, top}, Size: att.Size}, shown: true}
		}
		y += line.height
	}
	return l
}

func (approxText) Release() {}
