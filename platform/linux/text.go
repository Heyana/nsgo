//go:build linux && !android

package linux

import (
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

	pangoEllipsizeEnd = 3

	pangoStyleNormal  = 0
	pangoStyleOblique = 1
	pangoStyleItalic  = 2

	pangoUnderlineSingle = 1
)

// textLayout is a prepared text laid out at its final width. The engine
// measures it with Pango on its own thread; the main thread builds the
// PangoLayout it draws from the same plain data, so both use one font
// configuration.
type textLayout struct {
	text     string
	style    core.TextStyle
	spans    []core.TextSpan
	maxLines int
	width    core.Points
	height   core.Points

	// The main thread's PangoLayout, made when first drawn and freed by
	// Release.
	layout uintptr
}

// preparedText is a text shaped on the engine thread.
type preparedText struct {
	w        *window
	text     string
	style    core.TextStyle
	spans    []core.TextSpan
	maxLines int
	layout   uintptr // the engine thread's PangoLayout, reused across measures
	width    core.Points
	height   core.Points
}

// Prepare implements core.TextEngine.
func (w *window) Prepare(t *core.AttributedText) core.PreparedText {
	p := &preparedText{w: w, text: t.Text, style: t.Style, spans: t.Spans, maxLines: t.MaxLines}
	return p
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

// newLayout makes a PangoLayout on the current thread with the text and
// attributes of t.
func newLayout(context uintptr, text string, style core.TextStyle, spans []core.TextSpan, maxLines int, width core.Points) uintptr {
	l := pangoLayoutNew(context)
	applyLayout(l, text, style, spans, maxLines, width)
	return l
}

// applyLayout gives l its text, attributes and box.
func applyLayout(l uintptr, text string, style core.TextStyle, spans []core.TextSpan, maxLines int, width core.Points) {
	if text == "" {
		text = " "
	}
	attrs := textAttributes(text, style, spans)
	pangoLayoutSetAttributes(l, attrs)
	pangoAttrListUnref(attrs)
	pangoLayoutSetText(l, cstr(text), int32(len(text)))
	if maxLines == 1 || width.IsAuto() {
		pangoLayoutSetWidth(l, -1)
	} else {
		pangoLayoutSetWidth(l, int32(width*pangoScale+0.5))
	}
	switch style.Align {
	case core.TextAlignCenter:
		pangoLayoutSetAlignment(l, pangoAlignCenter)
	case core.TextAlignEnd:
		pangoLayoutSetAlignment(l, pangoAlignRight)
	default:
		pangoLayoutSetAlignment(l, pangoAlignLeft)
	}
	if maxLines > 1 {
		pangoLayoutSetWrap(l, pangoWrapWord)
	}
	if maxLines > 0 {
		pangoLayoutSetEllipsize(l, pangoEllipsizeEnd)
		pangoLayoutSetHeight(l, -int32(maxLines))
	}
}

// textAttributes returns the Pango attributes of text: the base style
// over all of it, then each span over its bytes.
func textAttributes(text string, style core.TextStyle, spans []core.TextSpan) uintptr {
	list := pangoAttrListNew()
	addStyle(list, len(text), 0, len(text), style)
	for _, sp := range spans {
		addStyle(list, len(text), sp.Start, sp.End, sp.Style)
	}
	return list
}

// addStyle adds s's font, color, spacing and decoration over start:end,
// clipped to textLen bytes.
func addStyle(list uintptr, textLen, start, end int, s core.TextStyle) {
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
	r16 := uint16(s.Color>>24&0xff) * 0x101
	g16 := uint16(s.Color>>16&0xff) * 0x101
	b16 := uint16(s.Color>>8&0xff) * 0x101
	add(pangoAttrForegroundNew(r16, g16, b16))
	if s.LetterSpacing != 0 {
		add(pangoAttrLetterSpacingNew(int32(s.LetterSpacing * pangoScale)))
	}
	if s.LineHeight > 0 {
		add(pangoAttrLineHeightAbsoluteNew(int32(s.LineHeight * pangoScale)))
	}
	if s.Decoration&core.TextDecorationUnderline != 0 {
		add(pangoAttrUnderlineNew(pangoUnderlineSingle))
	}
	if s.Decoration&core.TextDecorationLineThrough != 0 {
		add(pangoAttrStrikethroughNew(1))
	}
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
		p.layout = newLayout(p.w.measureContext(), p.text, p.style, p.spans, p.maxLines, width)
	} else {
		if p.maxLines == 1 || width.IsAuto() {
			pangoLayoutSetWidth(p.layout, -1)
		} else {
			pangoLayoutSetWidth(p.layout, int32(width*pangoScale+0.5))
		}
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
	spans := make([]core.TextSpan, len(p.spans))
	copy(spans, p.spans)
	return &textLayout{
		text:     p.text,
		style:    p.style,
		spans:    spans,
		maxLines: p.maxLines,
		width:    size.Width,
		height:   measured.Height,
	}
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
	applyLayout(l.layout, l.text, l.style, l.spans, l.maxLines, l.width)
	pangoCairoUpdateLayout(cr, l.layout)
}

// draw paints the text with its top-left at x, y.
func (l *textLayout) draw(cr uintptr, x, y core.Points) {
	if l.text == "" {
		return
	}
	l.ensure(cr)
	cairoMoveTo(cr, float64(x), float64(y))
	pangoCairoShowLayout(cr, l.layout)
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
