package driver

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

// AttributedText is styled text for a TextEngine.
type AttributedText struct {
	Text  string
	Style TextStyle
	// Spans style byte ranges of Text over Style, in order, without gaps
	// or overlaps where they cover it.
	Spans []TextSpan
	// MaxLines limits the lines shown, cutting the rest as Ellipsize
	// says; 0 shows every line.
	MaxLines  int
	Ellipsize EllipsizeMode
	// Shadow is drawn behind the whole text's glyphs. It is drawing only:
	// it changes no line.
	Shadow TextShadow
	// Attachments are views inside the text, in order: each takes the
	// place of an AttachmentRune of Text, which a span styles.
	Attachments []TextAttachment
	// Origin is, for a text an engine cut from another (Cut, CutToFit,
	// CutRest), the index of each attachment in the text it was cut
	// from, so the engine reports where they went by the app's indices.
	Origin []int
}

// TextSpan styles the bytes from Start to End of an AttributedText.
type TextSpan struct {
	Start, End int
	Style      TextStyle
	// Cursor is the pointer's look over the span's visible characters, ahead of the text view's own; Auto leaves the view's.
	Cursor Cursor
	// Link marks a span with a press handler: a press on it reports
	// EventPress with Index set to a byte offset inside it.
	Link bool
	// Hidden hides the span in place, as CSS's visibility: hidden on an inline element: its characters lay out as visible ones, but each engine skips its glyphs, decoration lines and shadow as it draws, they take no press, and copies and screen readers leave them out. It changes no line.
	Hidden bool
	// Background is drawn behind the span's characters: a box per line between the font's ascent and descent, behind the selection and glyphs.
	Background TextBackground
	// Copy is what a copy takes in place of the attachment the span styles, as a formula copies as its TeX; empty leaves the attachment out. Screen readers read the attachment's label instead (ShownText).
	Copy string
	// CopyItem holds extra clipboard representations offered alongside Copy when a selection copies this single attachment, such as a formula's MathML and text/x-tex source.
	CopyItem *ClipboardItem
}

// SpanCursor returns the cursor a span names over byte offset i (-1 off the text's glyphs), and false where none does.
func SpanCursor(spans []TextSpan, i int) (Cursor, bool) {
	for _, sp := range spans {
		if !sp.Hidden && sp.Cursor != CursorAuto && i >= sp.Start && i < sp.End {
			return sp.Cursor, true
		}
	}
	return CursorAuto, false
}

// HasSpanCursor reports whether any span names a cursor, so a backend knows the cursor under a still pointer may change with the text.
func HasSpanCursor(spans []TextSpan) bool {
	for _, sp := range spans {
		if sp.Cursor != CursorAuto {
			return true
		}
	}
	return false
}

// PressesAt reports whether a pressable text takes a press on byte
// offset i (-1 off its glyphs): a text with links takes only those on a
// link, as the core gives it no press of its own then.
func PressesAt(spans []TextSpan, i int) bool {
	linked := false
	for _, sp := range spans {
		if sp.Link {
			if i >= sp.Start && i < sp.End {
				return true
			}
			linked = true
		}
	}
	return !linked
}

// TextBackground is the background of a span. It changes only drawing.
type TextBackground struct {
	Color Color
	// Radius rounds each line's box, at most to half its painted height.
	Radius Points
	// Top and Bottom inset the box in points or as a percentage of its height.
	Top, Bottom Length
}

// VerticalBounds returns the background's top and bottom offsets from the top of a box of the given height. A band with bottom <= top is empty.
func (bg TextBackground) VerticalBounds(height Points) (top, bottom Points) {
	resolve := func(l Length) Points {
		p := l.Resolve(height)
		if p > 0 {
			return min(p, height)
		}
		return 0
	}
	return resolve(bg.Top), height - resolve(bg.Bottom)
}

// HasHidden reports whether a span of t is hidden.
func (t *AttributedText) HasHidden() bool {
	for _, sp := range t.Spans {
		if sp.Hidden {
			return true
		}
	}
	return false
}

// CopiedText is what a copy of text's bytes from to to takes: its hidden spans left out, and each attachment the copy takes whole replaced by its span's Copy, or left out without one.
func CopiedText(text string, spans []TextSpan, from, to int) string {
	var b strings.Builder
	eachShown(text, spans, from, to, func(s string, start int) {
		for {
			i := strings.IndexRune(s, AttachmentRune)
			if i < 0 {
				b.WriteString(s)
				return
			}
			b.WriteString(s[:i])
			b.WriteString(attachmentCopy(spans, start+i))
			n := len(string(AttachmentRune))
			s, start = s[i+n:], start+i+n
		}
	})
	return b.String()
}

// attachmentCopy is the Copy of the span styling the attachment at byte offset i.
func attachmentCopy(spans []TextSpan, i int) string {
	for _, sp := range spans {
		if sp.Start == i && sp.End == i+len(string(AttachmentRune)) {
			return sp.Copy
		}
	}
	return ""
}

// attachmentSpan returns the span styling the attachment at byte offset i.
func attachmentSpan(spans []TextSpan, i int) (TextSpan, bool) {
	for _, sp := range spans {
		if sp.Start == i && sp.End == i+len(string(AttachmentRune)) {
			return sp, true
		}
	}
	return TextSpan{}, false
}

// copiedSingleAttachment reports whether the visible bytes of text[from:to] consist solely of one attachment span with CopyItem representations.
func copiedSingleAttachment(text string, spans []TextSpan, from, to int) (TextSpan, bool) {
	var found TextSpan
	hasAttachment := false
	hasOther := false
	attachLen := len(string(AttachmentRune))
	eachShown(text, spans, from, to, func(s string, start int) {
		if hasOther || s == "" {
			return
		}
		if s == string(AttachmentRune) && !hasAttachment {
			if sp, ok := attachmentSpan(spans, start); ok && sp.CopyItem != nil && len(sp.CopyItem.Representations) > 0 {
				found, hasAttachment = sp, true
				return
			}
		}
		if len(s) != attachLen || s != string(AttachmentRune) {
			hasOther = true
			return
		}
		hasOther = true
	})
	return found, hasAttachment && !hasOther
}

// VisibleText is text with its hidden spans left out, its attachments'
// places kept.
func VisibleText(text string, spans []TextSpan) string {
	var b strings.Builder
	eachShown(text, spans, 0, len(text), func(s string, _ int) { b.WriteString(s) })
	return b.String()
}

// eachShown calls f with each piece of text's bytes from to to outside its hidden spans, in order, with its offset in text.
func eachShown(text string, spans []TextSpan, from, to int, f func(s string, start int)) {
	at := from
	for _, sp := range spans {
		if !sp.Hidden || sp.End <= at || sp.Start >= to {
			continue
		}
		if sp.Start > at {
			f(text[at:sp.Start], at)
		}
		at = max(at, sp.End)
	}
	if at < to {
		f(text[at:to], at)
	}
}

// ShownText is text as screen readers read it: its hidden spans left out, and its attachments, in order, read as labels say, separated from the text around them by a space; one past the labels is left out. A span's Copy is never read: a formula reads as its spoken label, not its TeX.
func ShownText(text string, spans []TextSpan, labels []string) string {
	var b strings.Builder
	// write writes a label, apart from what comes before it.
	write := func(label string) {
		if b.Len() > 0 && b.String()[b.Len()-1] != ' ' {
			b.WriteByte(' ')
		}
		b.WriteString(label)
	}
	eachShown(text, spans, 0, len(text), func(s string, start int) {
		n := len(string(AttachmentRune))
		for {
			i := strings.IndexRune(s, AttachmentRune)
			if i < 0 {
				b.WriteString(s)
				return
			}
			b.WriteString(s[:i])
			// Attachments in hidden spans count too, so labels keep their order.
			if k := strings.Count(text[:start+i], string(AttachmentRune)); k < len(labels) && labels[k] != "" {
				write(labels[k])
				if rest := s[i+n:]; rest != "" && rest[0] != ' ' {
					b.WriteByte(' ')
				}
			}
			s, start = s[i+n:], start+i+n
		}
	})
	return b.String()
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
	// Descent is how far an attachment aligned to the baseline reaches below it: the height of its own baseline above its bottom, as a formula's. 0 puts its bottom on the baseline; other aligns ignore it.
	Descent Points
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
	return h - a.Descent
}

// Extent returns what a's line reserves for it above and below the
// baseline, each at least 0. Engines give the placeholder these metrics
// and draw its top Ascent above the baseline.
func (a TextAttachment) Extent(font FontExtent) (above, below Points) {
	asc := a.Ascent(font)
	return max(asc, 0), max(a.Size.Height-asc, 0)
}

// CutToFit returns t cut at its head or middle, as its Ellipsize says,
// keeping the most characters with which fits reports true.
func (t *AttributedText) CutToFit(fits func(*AttributedText) bool) AttributedText {
	head := t.Ellipsize == EllipsizeHead
	cut := func(k int) AttributedText {
		front := (k + 1) / 2
		return t.Cut(head, front, k-front)
	}
	lo, hi := 0, utf8.RuneCountInString(t.Text)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if c := cut(mid); fits(&c) {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return cut(max(lo, 0))
}

// Equal reports whether a and b lay out and draw the same.
func (a *AttributedText) Equal(b *AttributedText) bool {
	if a.Text != b.Text || a.Style != b.Style || a.MaxLines != b.MaxLines ||
		a.Ellipsize != b.Ellipsize || a.Shadow != b.Shadow ||
		len(a.Spans) != len(b.Spans) || len(a.Attachments) != len(b.Attachments) {
		return false
	}
	for i := range a.Spans {
		if !textSpanEqual(a.Spans[i], b.Spans[i]) {
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

func textSpanEqual(a, b TextSpan) bool {
	return a.Start == b.Start && a.End == b.End && a.Style == b.Style &&
		a.Cursor == b.Cursor && a.Link == b.Link && a.Hidden == b.Hidden &&
		a.Background == b.Background && a.Copy == b.Copy &&
		ClipboardItemEqual(a.CopyItem, b.CopyItem)
}

// TextStyle is resolved text style: nothing in it is unset.
type TextStyle struct {
	Font  Font
	Color Color
	// LineHeight is the height of a line; 0 is the font's.
	LineHeight    Points
	LetterSpacing Points
	// Align places lines; TextAlignStart is the left, or the right when
	// RTL.
	Align TextAlign
	// RTL makes the paragraphs' base direction right to left, for the
	// order of mixed-direction runs and where lines start.
	RTL        bool
	Decoration TextDecoration
	// DecorationStyle is how Decoration's lines are drawn, solid when 0;
	// DecorationColor colors them, Color when 0.
	DecorationStyle TextDecorationStyle
	DecorationColor Color
	// DecorationThickness is how thick Decoration's lines are, the
	// font's when 0.
	DecorationThickness Points
	// UnderlineOffset is how far below the baseline an underline's top
	// is; auto is where the font puts it.
	UnderlineOffset Length
	// Transform is the core's TextTransform, which it applied to the
	// text already: platforms draw the text as given, tests read it.
	Transform uint8
}

// EllipsizeMode is where a text cut by its line limit loses the rest.
type EllipsizeMode uint8

const (
	// EllipsizeTail ends the last line with an ellipsis: "Lorem ips…".
	// It is the default.
	EllipsizeTail EllipsizeMode = iota + 1
	// EllipsizeHead starts it with one, "…sum dolor", and EllipsizeMiddle
	// puts one in its middle, "Lorem…dolor". Platforms do it on a single
	// line; on several lines some cut only the last one's end.
	EllipsizeHead
	EllipsizeMiddle
	// EllipsizeClip cuts the text without an ellipsis.
	EllipsizeClip
)

// TextShadow is a shadow behind a text's glyphs; a zero Color is none.
type TextShadow struct {
	Color  Color
	Offset Point
	// Radius blurs it, as a view's ShadowRadius does.
	Radius Points
}

// TextLine is a line of laid-out text, as Text.OnTextLayout reports it.
type TextLine struct {
	// Text is its characters, from byte Start to byte End of the text.
	Text       string
	Start, End int
	// Bounds is its box in the text's content box: its typographic width
	// and its height.
	Bounds Bounds
	// Ascender and Descender reach above and below its baseline;
	// CapHeight and XHeight are the heights of its font's capitals and
	// lowercase letters.
	Ascender, Descender Points
	CapHeight, XHeight  Points
}

// Cut returns t with the characters between keep runes from its start
// and keep from its end replaced by an ellipsis, "Lorem…dolor", or with
// only the last keep runes after one when head is set, "…sum dolor". The
// ellipsis takes the style of the first character it replaces, and
// attachments cut out are dropped. The result is one line, clipped.
// Engines that cannot ellipsize a line's head or middle themselves search
// for the most characters that still fit.
func (t *AttributedText) Cut(head bool, front, back int) AttributedText {
	bounds := make([]int, 0, len(t.Text)+1)
	for i := range t.Text {
		bounds = append(bounds, i)
	}
	bounds = append(bounds, len(t.Text))
	n := len(bounds) - 1
	if head {
		front = 0
	}
	front, back = min(max(front, 0), n), min(max(back, 0), n)
	if front+back >= n {
		return *t
	}
	a, b := bounds[front], bounds[n-back]
	const ellipsis = "…"
	out := *t
	out.Text = t.Text[:a] + ellipsis + t.Text[b:]
	out.MaxLines, out.Ellipsize = 1, EllipsizeClip
	// What was cut moves to the ellipsis's end, so the span of the first
	// character cut takes the ellipsis.
	moved := func(i int) int {
		switch {
		case i <= a:
			return i
		case i >= b:
			return i - (b - a) + len(ellipsis)
		}
		return a + len(ellipsis)
	}
	out.Spans = nil
	for _, sp := range t.Spans {
		sp.Start, sp.End = moved(sp.Start), moved(sp.End)
		if sp.End > sp.Start {
			out.Spans = append(out.Spans, sp)
		}
	}
	out.Attachments, out.Origin = nil, nil
	for i, at := range t.Attachments {
		if at.Offset < a || at.Offset >= b {
			at.Offset = moved(at.Offset)
			out.Attachments = append(out.Attachments, at)
			out.Origin = append(out.Origin, t.origin(i))
		}
	}
	return out
}

// origin is the index attachment i of t has in the text t was cut from.
func (t *AttributedText) origin(i int) int {
	if i < len(t.Origin) {
		return t.Origin[i]
	}
	return i
}

// slice returns the bytes from a to b of t as a text of their own, its
// spans and attachments with them.
func (t *AttributedText) slice(a, b int) AttributedText {
	out := *t
	out.Text = t.Text[a:b]
	out.Spans, out.Attachments, out.Origin = nil, nil, nil
	for _, sp := range t.Spans {
		sp.Start, sp.End = max(sp.Start, a)-a, min(sp.End, b)-a
		if sp.End > sp.Start {
			out.Spans = append(out.Spans, sp)
		}
	}
	for i, at := range t.Attachments {
		if at.Offset >= a && at.Offset < b {
			at.Offset -= a
			out.Attachments = append(out.Attachments, at)
			out.Origin = append(out.Origin, t.origin(i))
		}
	}
	return out
}

// CutRest returns t with its lines before the one starting at byte start
// as they are, then its rest cut at its head or middle, as Ellipsize
// says, into the most characters with which fits reports the rest fits
// on one line: what CoreText does to a several-line text's last line,
// for engines that ellipsize a head or middle only on one line. A line
// break joins the two, and the result clips at MaxLines.
func (t *AttributedText) CutRest(start int, fits func(*AttributedText) bool) AttributedText {
	start = min(max(start, 0), len(t.Text))
	head, rest := t.slice(0, start), t.slice(start, len(t.Text))
	rest.MaxLines = 1
	cut := rest.CutToFit(fits)
	out := head
	if !strings.HasSuffix(out.Text, "\n") {
		out.Text += "\n"
	}
	shift := len(out.Text)
	out.Text += cut.Text
	for _, sp := range cut.Spans {
		sp.Start, sp.End = sp.Start+shift, sp.End+shift
		out.Spans = append(out.Spans, sp)
	}
	for i, at := range cut.Attachments {
		at.Offset += shift
		out.Attachments = append(out.Attachments, at)
		out.Origin = append(out.Origin, cut.origin(i))
	}
	out.MaxLines, out.Ellipsize = t.MaxLines, EllipsizeClip
	return out
}

// FontMonospace is the family name of the platform's fixed-pitch font,
// for code.
const FontMonospace = "monospace"

// Font is what a text engine needs to pick a font.
type Font struct {
	// Family is a font family name; empty is the system font, and
	// FontMonospace the platform's fixed-pitch font.
	Family string
	// Language is the text's BCP 47 tag, which picks the glyph forms of characters several languages share, such as Han, and the fallback fonts and line breaking where the platform's text stack takes it; empty is the platform's own choice.
	Language string
	Size     Points
	Weight   FontWeight
	Italic   bool
	// Variant turns on OpenType features: small capitals and the forms
	// and widths of figures.
	Variant FontVariant
}

// FontVariant is a set of OpenType features, as CSS's font-variant-caps
// and font-variant-numeric set them; the values combine.
type FontVariant uint8

const (
	// FontVariantNormal is a variant that turns no feature on: a style
	// sets it to undo an inherited variant. Engines ignore it.
	FontVariantNormal FontVariant = 1 << iota
	// FontSmallCaps draws lowercase letters as small capitals (smcp).
	FontSmallCaps
	// FontOldstyleNums draws figures that rise and fall as lowercase
	// letters do (onum); FontLiningNums draws them at the capitals'
	// height (lnum).
	FontOldstyleNums
	FontLiningNums
	// FontTabularNums gives every figure the same width (tnum), so a
	// changing value keeps its place; FontProportionalNums gives each its
	// own (pnum).
	FontTabularNums
	FontProportionalNums
)

// fontFeatures are the OpenType tags of FontVariant's features, in the
// order of their bits.
var fontFeatures = [...]struct {
	bit FontVariant
	tag string
}{
	{FontSmallCaps, "smcp"},
	{FontOldstyleNums, "onum"},
	{FontLiningNums, "lnum"},
	{FontTabularNums, "tnum"},
	{FontProportionalNums, "pnum"},
}

// Tags returns the OpenType tags of the features v turns on.
func (v FontVariant) Tags() []string {
	var tags []string
	for _, f := range fontFeatures {
		if v&f.bit != 0 {
			tags = append(tags, f.tag)
		}
	}
	return tags
}

// KeepAllWords reports whether language breaks lines between words rather than inside words of CJK scripts, as Korean does under CSS's word-break: keep-all.
func KeepAllWords(language string) bool {
	if len(language) < 2 {
		return false
	}
	if (language[0] != 'k' && language[0] != 'K') || (language[1] != 'o' && language[1] != 'O') {
		return false
	}
	return len(language) == 2 || language[2] == '-' || language[2] == '_'
}

// KeepAllLanguage returns language with the BCP 47 Unicode line-break word extension lw-keepall added when KeepAllWords(language) is true and no explicit lw keyword is already set, preserving any other extensions and private-use subtags.
func KeepAllLanguage(language string) string {
	if !KeepAllWords(language) {
		return language
	}
	tag := LanguageTag(language)
	parts := strings.Split(tag, "-")
	uAt, xAt := -1, -1
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) != 1 {
			continue
		}
		switch parts[i][0] {
		case 'u', 'U':
			if uAt < 0 {
				uAt = i
			}
		case 'x', 'X':
			if xAt < 0 {
				xAt = i
			}
		}
		if xAt >= 0 {
			break
		}
	}
	if uAt >= 0 {
		end := len(parts)
		for i := uAt + 1; i < len(parts); i++ {
			if len(parts[i]) == 1 {
				end = i
				break
			}
			if len(parts[i]) == 2 && strings.EqualFold(parts[i], "lw") {
				return tag
			}
		}
		return strings.Join(slices.Insert(parts, end, "lw", "keepall"), "-")
	}
	if xAt >= 0 {
		return strings.Join(slices.Insert(parts, xAt, "u", "lw", "keepall"), "-")
	}
	return tag + "-u-lw-keepall"
}

// isKeepAllCJK reports whether r is a Hangul, Han or Kana character that UAX #14 breaks around unless word-break: keep-all suppresses it.
func isKeepAllCJK(r rune) bool {
	return r >= 0xAC00 && r <= 0xD7AF || // Hangul Syllables
		r >= 0x1100 && r <= 0x11FF || // Hangul Jamo
		r >= 0x3130 && r <= 0x318F || // Hangul Compatibility Jamo
		r >= 0xA960 && r <= 0xA97F || // Hangul Jamo Extended-A
		r >= 0xD7B0 && r <= 0xD7FF || // Hangul Jamo Extended-B
		r >= 0x3400 && r <= 0x4DBF || // CJK Unified Ideographs Extension A
		r >= 0x4E00 && r <= 0x9FFF || // CJK Unified Ideographs
		r >= 0xF900 && r <= 0xFAFF || // CJK Compatibility Ideographs
		r >= 0x20000 && r <= 0x3FFFF || // CJK Unified Ideographs Extensions B+
		r >= 0x3040 && r <= 0x30FF || // Hiragana and Katakana
		r >= 0x31F0 && r <= 0x31FF || // Katakana Phonetic Extensions
		r >= 0xFF66 && r <= 0xFF9F // Halfwidth Katakana
}

func isKeepAllWordRune(r rune) bool {
	if r == AttachmentRune {
		return false
	}
	return isKeepAllCJK(r) || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}

// KeepAllWordRanges returns the byte ranges [start, end] of words in text that should not break internally under CSS's word-break: keep-all: contiguous letter, digit and CJK runs of at least two runes containing at least one Hangul, Han or Kana character.
func KeepAllWordRanges(text string) [][2]int {
	var out [][2]int
	start, runes, hasCJK := -1, 0, false
	flush := func(end int) {
		if start >= 0 && runes >= 2 && hasCJK {
			out = append(out, [2]int{start, end})
		}
		start, runes, hasCJK = -1, 0, false
	}
	for i, r := range text {
		if isKeepAllWordRune(r) {
			if start < 0 {
				start = i
			}
			runes++
			if !hasCJK && isKeepAllCJK(r) {
				hasCJK = true
			}
		} else {
			flush(i)
		}
	}
	flush(len(text))
	return out
}
