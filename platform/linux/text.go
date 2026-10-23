//go:build linux && !android

package linux

import (
	"math"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

// Pango units per point. Pango sizes, widths and spacing use these.
const pangoScale = 1024

const (
	pangoAlignLeft   = 0
	pangoAlignCenter = 1
	pangoAlignRight  = 2

	pangoWrapWord     = 0
	pangoWrapChar     = 1
	pangoWrapWordChar = 2

	pangoEllipsizeNone   = 0
	pangoEllipsizeStart  = 1
	pangoEllipsizeMiddle = 2
	pangoEllipsizeEnd    = 3

	pangoStyleNormal  = 0
	pangoStyleOblique = 1
	pangoStyleItalic  = 2

	pangoUnderlineSingle = 1
	pangoUnderlineDouble = 2
	pangoUnderlineError  = 4 // wavy
)

// textSource is what a text is laid out from: plain data, so the engine
// thread and the main thread each build a PangoLayout of their own.
type textSource struct {
	text     string
	style    core.TextStyle
	spans    []core.TextSpan
	maxLines int
	// ellipsize cuts what maxLines leaves out; a clipped text is measured
	// as if it ended with an ellipsis and drawn whole, clipped to its box.
	ellipsize core.EllipsizeMode
	shadow    core.TextShadow
	// atts are the text's attachments and fonts the metrics of the font
	// around each, which places it.
	atts  []core.TextAttachment
	fonts []core.FontExtent
}

// textLayout is a prepared text laid out at its final width. The engine
// measures it with Pango on its own thread; the main thread builds the
// PangoLayout it draws from the same plain data, so both use one font
// configuration.
type textLayout struct {
	textSource
	width  core.Points
	height core.Points

	// under are the spans' background boxes, from the content box's top
	// left; attachments are where the attachments went.
	under       []rect
	underColors []core.Color
	attachments []attachmentBox

	// lines are the lines OnTextLayout reports.
	lines []core.TextLine

	// The main thread's PangoLayout, made when first drawn and freed by
	// Release.
	layout uintptr
}

// attachmentBox is an attachment's box in the content box, unless a line
// limit cut it off.
type attachmentBox struct {
	box   core.Bounds
	shown bool
}

// preparedText is a text shaped on the engine thread.
type preparedText struct {
	textSource
	w      *window
	layout uintptr // the engine thread's PangoLayout, reused across measures
	width  core.Points
	height core.Points
}

// Prepare implements core.TextEngine.
func (w *window) Prepare(t *core.AttributedText) core.PreparedText {
	src := textSource{text: t.Text, style: t.Style, spans: t.Spans, maxLines: t.MaxLines, ellipsize: t.Ellipsize,
		shadow: t.Shadow}
	if len(t.Attachments) > 0 {
		src.atts = append([]core.TextAttachment(nil), t.Attachments...)
		src.fonts = make([]core.FontExtent, len(t.Attachments))
		for i, at := range t.Attachments {
			src.fonts[i] = w.fontExtent(src.styleAt(at.Offset).Font)
		}
	}
	return &preparedText{w: w, textSource: src}
}

// styleAt is the style of the span at byte offset i: for an attachment,
// the font it is placed against.
func (s *textSource) styleAt(i int) core.TextStyle {
	for _, sp := range s.spans {
		if sp.Start <= i && i < sp.End {
			return sp.Style
		}
	}
	return s.style
}

// measureContext returns the Pango context the engine thread measures
// with, made on first use from a font map of its own. It is never the
// default context, which belongs to the main thread.
func (w *window) measureContext() uintptr {
	if w.measureMap == 0 {
		w.measureMap = pangoFontMapCreateContext(pangoCairoFontMapNew())
	}
	return w.measureMap
}

// fontExtent returns the vertical metrics of f, from a line of "x" in
// it: its ascent and descent, and the top of the x's ink. The engine
// thread calls it; it caches what it measured.
func (w *window) fontExtent(f core.Font) core.FontExtent {
	if e, ok := w.extents[f]; ok {
		return e
	}
	l := pangoLayoutNew(w.measureContext())
	attrs := pangoAttrListNew()
	addStyle(attrs, 1, 0, 1, core.TextStyle{Font: f}, nil)
	pangoLayoutSetAttributes(l, attrs)
	pangoAttrListUnref(attrs)
	pangoLayoutSetText(l, cstr("x"), 1)
	var ink, logical pangoRectangle
	pangoLayoutGetExtents(l, &ink, &logical)
	base := pangoLayoutGetBaseline(l)
	gObjectUnref(l)
	e := core.FontExtent{
		Ascent:  fromPango(base - logical.Y),
		Descent: fromPango(logical.Y + logical.Height - base),
		XHeight: fromPango(base - ink.Y),
	}
	if w.extents == nil {
		w.extents = map[core.Font]core.FontExtent{}
	}
	w.extents[f] = e
	return e
}

// pangoRectangle is PangoRectangle, in Pango units.
type pangoRectangle struct{ X, Y, Width, Height int32 }

func toPango(p core.Points) int32   { return int32(math.Round(float64(p * pangoScale))) }
func fromPango(v int32) core.Points { return core.Points(v) / pangoScale }

// newLayout makes a PangoLayout on the current thread with the text and
// attributes of s, to measure.
func newLayout(context uintptr, s *textSource, width core.Points) uintptr {
	l := pangoLayoutNew(context)
	applyLayout(l, s, width, false)
	return l
}

// applyLayout gives l its text, attributes and box; drawn, a clipped
// text keeps every line.
func applyLayout(l uintptr, s *textSource, width core.Points, drawn bool) {
	text := s.text
	if text == "" {
		text = " "
	}
	attrs := textAttributes(text, s)
	pangoLayoutSetAttributes(l, attrs)
	pangoAttrListUnref(attrs)
	pangoLayoutSetText(l, cstr(text), int32(len(text)))
	pangoLayoutSetWidth(l, s.pangoWidth(width, drawn))
	switch s.style.Align {
	case core.TextAlignCenter:
		pangoLayoutSetAlignment(l, pangoAlignCenter)
	case core.TextAlignEnd:
		pangoLayoutSetAlignment(l, pangoAlignRight)
	default:
		pangoLayoutSetAlignment(l, pangoAlignLeft)
	}
	pangoLayoutSetJustify(l, s.style.Align == core.TextAlignJustify)
	if s.maxLines > 1 {
		pangoLayoutSetWrap(l, pangoWrapWord)
	}
	if s.maxLines > 0 && !(drawn && s.ellipsize == core.EllipsizeClip) {
		mode := int32(pangoEllipsizeEnd)
		switch s.ellipsize {
		case core.EllipsizeHead:
			mode = pangoEllipsizeStart
		case core.EllipsizeMiddle:
			mode = pangoEllipsizeMiddle
		}
		pangoLayoutSetEllipsize(l, mode)
		pangoLayoutSetHeight(l, -int32(s.maxLines))
	}
}

// pangoWidth is the width Pango wraps and ellipsizes s at: none for a
// clipped line drawn whole, which its box cuts.
func (s *textSource) pangoWidth(width core.Points, drawn bool) int32 {
	if width.IsAuto() || (drawn && s.maxLines == 1 && s.ellipsize == core.EllipsizeClip) {
		return -1
	}
	return int32(width*pangoScale + 0.5)
}

// placeholderLen is the length in bytes of an attachment's placeholder.
var placeholderLen = len(string(core.AttachmentRune))

// textAttributes returns the Pango attributes of text: the base style
// over all of it, then each span over its bytes, then the attachments'
// placeholders. A placeholder is a shape of the attachment's width that
// draws nothing: its logical rectangle is what its line reserves (the
// attachment's Extent), its ink the attachment, so its line grows where
// the attachment reaches past the font. Line heights leave placeholders
// out, as they would hold a line to them.
func textAttributes(text string, s *textSource) uintptr {
	var holes [][2]int
	for _, at := range s.atts {
		if at.Offset >= 0 && at.Offset+placeholderLen <= len(text) {
			holes = append(holes, [2]int{at.Offset, at.Offset + placeholderLen})
		}
	}
	list := pangoAttrListNew()
	addStyle(list, len(text), 0, len(text), s.style, holes)
	for _, sp := range s.spans {
		addStyle(list, len(text), sp.Start, sp.End, sp.Style, holes)
	}
	for i, at := range s.atts {
		if at.Offset < 0 || at.Offset+placeholderLen > len(text) {
			continue
		}
		top := at.Ascent(s.fonts[i])
		above, below := at.Extent(s.fonts[i])
		ink := pangoRectangle{0, -toPango(top), toPango(at.Size.Width), toPango(at.Size.Height)}
		logical := pangoRectangle{0, -toPango(above), toPango(at.Size.Width), toPango(above + below)}
		for _, attr := range []uintptr{
			pangoAttrShapeNew(&ink, &logical),
			// Lines are not drawn through an attachment.
			pangoAttrUnderlineNew(0),
			pangoAttrStrikethroughNew(0),
		} {
			setAttributeRange(attr, uint32(at.Offset), uint32(at.Offset+placeholderLen))
			pangoAttrListInsert(list, attr)
		}
	}
	return list
}

// addStyle adds s's font, color, spacing and decoration over start:end,
// clipped to textLen bytes. Its line height leaves out holes, which are
// in order.
func addStyle(list uintptr, textLen, start, end int, s core.TextStyle, holes [][2]int) {
	start, end = min(max(start, 0), textLen), min(max(end, 0), textLen)
	if end <= start {
		return
	}
	add := func(attr uintptr) {
		if attr == 0 {
			return
		}
		setAttributeRange(attr, uint32(start), uint32(end))
		pangoAttrListInsert(list, attr)
	}
	f := s.Font
	if f.Family != "" {
		add(pangoAttrFamilyNew(cstr(f.Family)))
	}
	if f.Size > 0 {
		add(pangoAttrSizeNew(int32(f.Size*pangoScale + 0.5)))
	}
	if f.Weight > 0 {
		add(pangoAttrWeightNew(int32(f.Weight)))
	}
	if f.Italic {
		add(pangoAttrStyleNew(pangoStyleItalic))
	}
	add(pangoAttrForegroundNew(rgb16(s.Color)))
	if s.LetterSpacing != 0 {
		add(pangoAttrLetterSpacingNew(int32(s.LetterSpacing * pangoScale)))
	}
	if s.LineHeight > 0 {
		from := start
		for _, h := range append(holes, [2]int{end, end}) {
			to := min(max(h[0], from), end)
			if to > from {
				attr := pangoAttrLineHeightAbsoluteNew(int32(s.LineHeight * pangoScale))
				setAttributeRange(attr, uint32(from), uint32(to))
				pangoAttrListInsert(list, attr)
			}
			from = max(from, h[1])
		}
	}
	// Pango draws no dotted or dashed lines, and strikes through solid.
	if s.Decoration&core.TextDecorationUnderline != 0 {
		line := int32(pangoUnderlineSingle)
		switch s.DecorationStyle {
		case core.DecorationDouble:
			line = pangoUnderlineDouble
		case core.DecorationWavy:
			line = pangoUnderlineError
		}
		add(pangoAttrUnderlineNew(line))
		if s.DecorationColor != 0 {
			add(pangoAttrUnderlineColorNew(rgb16(s.DecorationColor)))
		}
	}
	if s.Decoration&core.TextDecorationLineThrough != 0 {
		add(pangoAttrStrikethroughNew(1))
		if s.DecorationColor != 0 {
			add(pangoAttrStrikethroughColorNew(rgb16(s.DecorationColor)))
		}
	}
}

// rgb16 returns c's channels as Pango's 16-bit ones.
func rgb16(c core.Color) (r, g, b uint16) {
	return uint16(c>>24&0xff) * 0x101, uint16(c>>16&0xff) * 0x101, uint16(c>>8&0xff) * 0x101
}

// setAttributeRange fills a PangoAttribute's byte range, the first three
// fields of its public struct.
func setAttributeRange(attr uintptr, start, end uint32) {
	*(*uint32)(unsafe.Add(cPointer(attr), 8)) = start
	*(*uint32)(unsafe.Add(cPointer(attr), 12)) = end
}

// measure shapes and measures the text at width, in Pango units of the
// engine thread's font map.
func (p *preparedText) measure(width core.Points) core.Size {
	if p.layout == 0 {
		p.layout = newLayout(p.w.measureContext(), &p.textSource, width)
	} else {
		pangoLayoutSetWidth(p.layout, p.pangoWidth(width, false))
	}
	var w, h int32
	pangoLayoutGetPixelSize(p.layout, &w, &h)
	return core.Size{Width: core.Points(w), Height: core.Points(h)}
}

// Measure implements core.PreparedText.
func (p *preparedText) Measure(maxWidth core.Points) core.Size {
	size := p.measure(maxWidth)
	p.width, p.height = size.Width, size.Height
	return size
}

// Layout implements core.PreparedText.
func (p *preparedText) Layout(size core.Size) core.TextLayout {
	// Measure once at the final width so the engine's layout and the
	// main thread's agree on the text's height.
	measured := p.measure(size.Width)
	src := p.textSource
	src.spans = append([]core.TextSpan(nil), p.spans...)
	l := &textLayout{textSource: src, width: size.Width, height: measured.Height}
	p.placeBackgrounds(l)
	p.placeAttachments(l)
	p.placeLines(l)
	return l
}

// placeLines lists the layout's lines for OnTextLayout, with the metrics
// of the font each starts in.
func (p *preparedText) placeLines(l *textLayout) {
	if p.text == "" {
		return
	}
	iter := pangoLayoutGetIter(p.layout)
	defer pangoLayoutIterFree(iter)
	for {
		line := pangoLayoutIterGetLine(iter)
		from := int(*(*int32)(unsafe.Add(cPointer(line), 8)))
		to := from + int(*(*int32)(unsafe.Add(cPointer(line), 12)))
		var ink, logical pangoRectangle
		pangoLayoutIterGetLineExtents(iter, &ink, &logical)
		baseline := pangoLayoutIterGetBaseline(iter)
		font := p.styleAt(from).Font
		ext := p.w.fontExtent(font)
		l.lines = append(l.lines, core.TextLine{Start: min(from, len(p.text)), End: min(to, len(p.text)),
			Bounds: core.Bounds{Origin: core.Point{X: fromPango(logical.X), Y: fromPango(logical.Y)},
				Size: core.Size{Width: fromPango(logical.Width), Height: fromPango(logical.Height)}},
			Ascender: fromPango(baseline - logical.Y), Descender: fromPango(logical.Y + logical.Height - baseline),
			CapHeight: p.w.capHeight(font), XHeight: ext.XHeight})
		if pangoLayoutIterNextLine(iter) == 0 {
			break
		}
	}
}

// TextLines implements core.LineLayout.
func (l *textLayout) TextLines() []core.TextLine { return l.lines }

// capHeight returns the height of f's capitals, from the ink of an "H".
// The engine thread calls it; it caches what it measured.
func (w *window) capHeight(f core.Font) core.Points {
	if h, ok := w.capHeights[f]; ok {
		return h
	}
	l := pangoLayoutNew(w.measureContext())
	attrs := pangoAttrListNew()
	addStyle(attrs, 1, 0, 1, core.TextStyle{Font: f}, nil)
	pangoLayoutSetAttributes(l, attrs)
	pangoAttrListUnref(attrs)
	pangoLayoutSetText(l, cstr("H"), 1)
	var ink, logical pangoRectangle
	pangoLayoutGetExtents(l, &ink, &logical)
	h := fromPango(pangoLayoutGetBaseline(l) - ink.Y)
	gObjectUnref(l)
	if w.capHeights == nil {
		w.capHeights = map[core.Font]core.Points{}
	}
	w.capHeights[f] = h
	return h
}

// blank reports whether c is a space or a line break, which a line's
// end hangs and a span's background leaves out.
func blank(c byte) bool { return c == ' ' || c == '\n' || c == '\r' || c == '\t' }

// placeBackgrounds puts a box per line behind each span with a
// background, from its font's ascent to its descent.
func (p *preparedText) placeBackgrounds(l *textLayout) {
	has := false
	for _, sp := range p.spans {
		has = has || sp.Background.Color&0xff != 0
	}
	if !has || p.text == "" {
		return
	}
	iter := pangoLayoutGetIter(p.layout)
	defer pangoLayoutIterFree(iter)
	for {
		line := pangoLayoutIterGetLine(iter)
		from := int(*(*int32)(unsafe.Add(cPointer(line), 8)))
		to := from + int(*(*int32)(unsafe.Add(cPointer(line), 12)))
		baseline := fromPango(pangoLayoutIterGetBaseline(iter))
		for _, sp := range p.spans {
			bg := sp.Background
			if bg.Color&0xff == 0 {
				continue
			}
			a, b := max(sp.Start, from), min(sp.End, to, len(p.text))
			for b > a && blank(p.text[b-1]) {
				b--
			}
			if a >= b {
				continue
			}
			font := p.w.fontExtent(sp.Style.Font)
			h := float64(font.Ascent + font.Descent)
			var ranges uintptr
			var n int32
			pangoLayoutLineGetXRanges(line, int32(a), int32(b), &ranges, &n)
			if ranges == 0 {
				continue
			}
			xs := unsafe.Slice((*int32)(cPointer(ranges)), 2*n)
			for i := 0; i < int(n); i++ {
				x0, x1 := fromPango(xs[2*i]), fromPango(xs[2*i+1])
				if x1 <= x0 {
					continue
				}
				l.under = append(l.under, rect{x: float64(x0), y: float64(baseline - font.Ascent),
					w: float64(x1 - x0), h: h, r: min(float64(bg.Radius), h/2)})
				l.underColors = append(l.underColors, bg.Color)
			}
			gFree(ranges)
		}
		if pangoLayoutIterNextLine(iter) == 0 {
			break
		}
	}
}

// placeAttachments finds each attachment's placeholder among the
// layout's runs: its top is its ascent above the baseline. One that is
// not a run of its own was cut off by the line limit or the ellipsis.
func (p *preparedText) placeAttachments(l *textLayout) {
	if len(p.atts) == 0 {
		return
	}
	l.attachments = make([]attachmentBox, len(p.atts))
	iter := pangoLayoutGetIter(p.layout)
	defer pangoLayoutIterFree(iter)
	for {
		if run := pangoLayoutIterGetRun(iter); run != 0 {
			item := *(*uintptr)(cPointer(run))
			offset := int(*(*int32)(cPointer(item)))
			length := int(*(*int32)(unsafe.Add(cPointer(item), 4)))
			for i, at := range p.atts {
				if at.Offset != offset || length != placeholderLen {
					continue
				}
				var logical pangoRectangle
				pangoLayoutIterGetRunExtents(iter, nil, &logical)
				top := fromPango(pangoLayoutIterGetBaseline(iter)) - at.Ascent(p.fonts[i])
				l.attachments[i] = attachmentBox{shown: true, box: core.Bounds{
					Origin: core.Point{X: fromPango(logical.X), Y: top}, Size: at.Size}}
			}
		}
		if pangoLayoutIterNextRun(iter) == 0 {
			break
		}
	}
}

// Attachment implements core.AttachmentLayout.
func (l *textLayout) Attachment(i int) (core.Bounds, bool) {
	if i < 0 || i >= len(l.attachments) {
		return core.Bounds{}, false
	}
	return l.attachments[i].box, l.attachments[i].shown
}

// Release implements core.PreparedText.
func (p *preparedText) Release() {
	if p.layout != 0 {
		gObjectUnref(p.layout)
		p.layout = 0
	}
}

// ensure makes the PangoLayout the main thread draws, once, within cr.
func (l *textLayout) ensure(cr uintptr) {
	if l.layout != 0 {
		pangoCairoUpdateLayout(cr, l.layout)
		return
	}
	l.layout = pangoCairoCreateLayout(cr)
	applyLayout(l.layout, &l.textSource, l.width, true)
	pangoCairoUpdateLayout(cr, l.layout)
}

// draw paints the text with its top-left at x, y: its spans'
// backgrounds, then its glyphs and their lines.
func (l *textLayout) draw(cr uintptr, x, y core.Points) {
	if l.text == "" {
		return
	}
	for i, r := range l.under {
		r.x += float64(x)
		r.y += float64(y)
		setColor(cr, l.underColors[i])
		roundRect(cr, r)
		cairoFill(cr)
	}
	l.ensure(cr)
	clip := l.maxLines > 0 && l.ellipsize == core.EllipsizeClip
	if clip {
		cairoSave(cr)
		cairoRectangle(cr, float64(x), float64(y), float64(l.width), float64(l.height))
		cairoClip(cr)
	}
	cairoMoveTo(cr, float64(x), float64(y))
	pangoCairoShowLayout(cr, l.layout)
	if clip {
		cairoRestore(cr)
	}
}

// indexAt returns the byte offset of the character nearest x, y, in the
// text's points.
func (l *textLayout) indexAt(x, y core.Points) int {
	if l.layout == 0 {
		return 0
	}
	var index, trailing int32
	pangoLayoutXyToIndex(l.layout, int32(x*pangoScale), int32(y*pangoScale), &index, &trailing)
	return int(index)
}

// Height returns the height the text laid out at.
func (l *textLayout) Height() core.Points { return l.height }

// Release implements core.TextLayout on the main thread.
func (l *textLayout) Release() {
	if l.layout != 0 {
		gObjectUnref(l.layout)
		l.layout = 0
	}
}
