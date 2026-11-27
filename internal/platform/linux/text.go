//go:build linux && !android

package linux

import (
	"math"
	"slices"
	"structs"
	"unicode/utf8"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	pangoAlignLeft   = 0
	pangoAlignCenter = 1
	pangoAlignRight  = 2

	pangoDirectionLTR = 0
	pangoDirectionRTL = 1

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
	style    driver.TextStyle
	spans    []driver.TextSpan
	maxLines int
	// ui is the UI font when the text was prepared (uiFontFamily), which its system families take on both threads.
	ui string
	// ellipsize cuts what maxLines leaves out; a clipped text is measured
	// as if it ended with an ellipsis and drawn whole, clipped to its box.
	ellipsize driver.EllipsizeMode
	shadow    driver.TextShadow
	// atts are the text's attachments and fonts the metrics of the font
	// around each, which places it.
	atts  []driver.TextAttachment
	fonts []driver.FontExtent
}

// textLayout is a prepared text laid out at its final width. The engine
// measures it with Pango on its own thread; the main thread builds the
// PangoLayout it draws from the same plain data, so both use one font
// configuration.
type textLayout struct {
	textSource
	width  driver.Points
	height driver.Points

	// under are the spans' background boxes, from the content box's top
	// left; attachments are where the attachments went.
	under       []rect
	underColors []driver.Color
	attachments []attachmentBox
	// over are the decoration lines Pango cannot draw, drawn over the
	// glyphs.
	over []decorationLine

	// lines are the lines OnTextLayout reports.
	lines []driver.TextLine
	// carets hit-tests the lines, read when first asked.
	carets driver.LazyCarets

	// The main thread's PangoLayout, made lazily for hit-testing, Cairo
	// drawing or fallback rendering, and freed by Release.
	layout uintptr

	// Shaped glyph runs extracted on the engine thread so the main thread
	// builds GskTextNode directly without re-shaping with Pango/HarfBuzz.
	runs      []shapedRun
	runsBuilt bool

	// The main thread's cached GskRenderNode for snapshot, built once at
	// (nodeX, nodeY) and freed by Release.
	node      uintptr
	nodeReady bool
	nodeX     driver.Points
	nodeY     driver.Points
}

// pangoGlyphGeometry, pangoGlyphInfo, pangoGlyphString and pangoGlyphItem
// match their public structs in <pango/pango-glyph.h> and
// <pango/pango-glyph-item.h>.
type pangoGlyphGeometry struct {
	_       structs.HostLayout
	width   int32
	xOffset int32
	yOffset int32
}

type pangoGlyphInfo struct {
	_        structs.HostLayout
	glyph    uint32
	geometry pangoGlyphGeometry
	attr     uint32
}

type pangoGlyphString struct {
	_           structs.HostLayout
	numGlyphs   int32
	glyphs      *pangoGlyphInfo
	logClusters *int32
	space       int32
}

type pangoGlyphItem struct {
	_      structs.HostLayout
	item   *pangoItem
	glyphs *pangoGlyphString
}

// shapedRun is one shaped Pango glyph run ready to become a GskTextNode on the
// main thread.
type shapedRun struct {
	fontDesc string
	x        float32
	baseline float32
	endUnits int32 // pen x after the last glyph in Pango units, for coalescing adjacent runs
	color    driver.Color
	glyphs   []pangoGlyphInfo
}

// attachmentBox is an attachment's box in the content box, unless a line
// limit cut it off.
type attachmentBox struct {
	box   driver.Bounds
	shown bool
}

// preparedText is a text shaped on the engine thread.
type preparedText struct {
	textSource
	w      *window
	layout uintptr // the engine thread's PangoLayout, reused across measures
	width  driver.Points
	height driver.Points
}

// Prepare implements core.TextEngine.
func (w *window) Prepare(t *driver.AttributedText) driver.PreparedText {
	src := textSource{text: t.Text, style: t.Style, spans: t.Spans, maxLines: t.MaxLines, ellipsize: t.Ellipsize,
		shadow: t.Shadow, ui: uiFontFamily()}
	if len(t.Attachments) > 0 {
		src.atts = append([]driver.TextAttachment(nil), t.Attachments...)
		src.fonts = make([]driver.FontExtent, len(t.Attachments))
		for i, at := range t.Attachments {
			src.fonts[i] = w.fontExtent(src.styleAt(at.Offset).Font, src.ui)
		}
	}
	return &preparedText{w: w, textSource: src}
}

// styleAt is the style of the span at byte offset i: for an attachment,
// the font it is placed against.
func (s *textSource) styleAt(i int) driver.TextStyle {
	for _, sp := range s.spans {
		if sp.Start <= i && i < sp.End {
			return sp.Style
		}
	}
	return s.style
}

// hasHidden reports whether any span of s is hidden.
func (s *textSource) hasHidden() bool {
	for _, sp := range s.spans {
		if sp.Hidden {
			return true
		}
	}
	return false
}

// isHidden reports whether byte offset i falls inside a hidden span of s.
func (s *textSource) isHidden(i int) bool {
	for _, sp := range s.spans {
		if sp.Hidden && sp.Start <= i && i < sp.End {
			return true
		}
	}
	return false
}

// isAttachment reports whether a run at byte offset i of length n is an attachment placeholder.
func (s *textSource) isAttachment(i, n int) bool {
	if n != placeholderLen {
		return false
	}
	for _, at := range s.atts {
		if at.Offset == i {
			return true
		}
	}
	return false
}

// pangoAnalysis and pangoItem match PangoAnalysis and PangoItem in <pango/pango-item.h>.
type pangoAnalysis struct {
	_                             structs.HostLayout
	shapeEngine, langEngine, font uintptr
	level, gravity, flags, script uint8
	language, extraAttrs          uintptr
}

type pangoItem struct {
	_                        structs.HostLayout
	offset, length, numChars int32
	analysis                 pangoAnalysis
}

// measureContext returns the Pango context the engine thread measures
// with, made on first use from a font map of its own. It is never the
// default context, which belongs to the main thread.
func (w *window) measureContext(rtl bool) uintptr {
	i := 0
	if rtl {
		i = 1
	}
	if w.measureMap[i] == 0 {
		w.measureMap[i] = newTextContext(rtl)
	}
	return w.measureMap[i]
}

// newTextContext gives the context a font map of its own. The context retains the map, so the map's initial reference belongs here only.
func newTextContext(rtl bool) uintptr {
	fontMap := pangoCairoFontMapNew()
	context := pangoFontMapCreateContext(fontMap)
	gObjectUnref(fontMap)
	pangoContextSetBaseDir(context, textDirection(rtl))
	return context
}

// releaseTextContexts drops both threads' contexts once their users have stopped, or before they prepare text again with new fonts.
func (w *window) releaseTextContexts() {
	for font := range w.fontDescs {
		if font != 0 {
			gObjectUnref(font)
		}
	}
	clear(w.fontDescs)
	for i, context := range w.measureMap {
		if context != 0 {
			gObjectUnref(context)
			w.measureMap[i] = 0
		}
	}
	w.hitMu.Lock()
	for i, context := range w.hitMap {
		if context != 0 {
			gObjectUnref(context)
			w.hitMap[i] = 0
		}
	}
	w.hitMu.Unlock()
}

// fontDescriptionString returns font's absolute-size Pango description string
// on the engine thread, retaining a bounded map of PangoFont refs so repeated
// runs in the same font pay one map lookup instead of allocating C strings.
func (w *window) fontDescriptionString(font uintptr) string {
	if s, ok := w.fontDescs[font]; ok {
		return s
	}
	desc := pangoFontDescribeWithAbsoluteSize(font)
	if desc == 0 {
		return ""
	}
	cs := pangoFontDescriptionToString(desc)
	s := goString(cs)
	gFree(cs)
	pangoFontDescriptionFree(desc)
	if w.fontDescs == nil {
		w.fontDescs = map[uintptr]string{}
	} else if len(w.fontDescs) >= maxFontMetrics {
		for f := range w.fontDescs {
			if f != 0 {
				gObjectUnref(f)
			}
		}
		clear(w.fontDescs)
	}
	gObjectRef(font)
	w.fontDescs[font] = s
	return s
}

// textDirection is the PangoDirection of a base direction.
func textDirection(rtl bool) int32 {
	if rtl {
		return pangoDirectionRTL
	}
	return pangoDirectionLTR
}

// fontExtent returns the vertical metrics of f, from a line of "x" in
// it: its ascent and descent, and the top of the x's ink, its system families in the UI font ui. The engine thread calls it; it caches what it measured.
func (w *window) fontExtent(f driver.Font, ui string) driver.FontExtent {
	key := fontKey{driver.FontKeyOf(f), ui}
	if e, ok := w.extents[key]; ok {
		return e
	}
	l := pangoLayoutNew(w.measureContext(false))
	attrs := pangoAttrListNew()
	addStyle(attrs, "x", 0, 1, driver.TextStyle{Font: f}, ui, nil, false, false)
	pangoLayoutSetAttributes(l, attrs)
	pangoAttrListUnref(attrs)
	pangoLayoutSetText(l, cstr("x"), 1)
	var ink, logical pangoRectangle
	pangoLayoutGetExtents(l, &ink, &logical)
	base := pangoLayoutGetBaseline(l)
	gObjectUnref(l)
	e := driver.FontExtent{
		Ascent:  fromPango(base - logical.Y),
		Descent: fromPango(logical.Y + logical.Height - base),
		XHeight: fromPango(base - ink.Y),
	}
	remember(&w.extents, key, e)
	return e
}

// pangoRectangle is PangoRectangle, in Pango units.
type pangoRectangle struct{ X, Y, Width, Height int32 }

// newLayout makes a PangoLayout on the current thread with the text and
// attributes of s, to measure.
func newLayout(context uintptr, s *textSource, width driver.Points) uintptr {
	l := pangoLayoutNew(context)
	applyLayout(l, s, width, false)
	return l
}

// applyLayout gives l its text, attributes and box; drawn, a clipped
// text keeps every line.
func applyLayout(l uintptr, s *textSource, width driver.Points, drawn bool) {
	text := s.text
	if text == "" {
		text = " "
	}
	attrs := textAttributes(text, s)
	pangoLayoutSetAttributes(l, attrs)
	pangoAttrListUnref(attrs)
	pangoLayoutSetTextStr(l, text)
	pangoLayoutSetWidth(l, s.pangoWidth(width, drawn))
	// The text's own base direction, not its first letter's; start and
	// end follow it.
	pangoLayoutSetAutoDir(l, false)
	if drawn {
		pangoContextSetBaseDir(pangoLayoutGetContext(l), textDirection(s.style.RTL))
		pangoLayoutContextChanged(l)
	}
	start, end := int32(pangoAlignLeft), int32(pangoAlignRight)
	if s.style.RTL {
		start, end = end, start
	}
	switch s.style.Align {
	case driver.TextAlignCenter:
		pangoLayoutSetAlignment(l, pangoAlignCenter)
	case driver.TextAlignEnd:
		pangoLayoutSetAlignment(l, end)
	default:
		pangoLayoutSetAlignment(l, start)
	}
	pangoLayoutSetJustify(l, s.style.Align == driver.TextAlignJustify)
	if s.maxLines != 1 && hasKeepAll(s) {
		pangoLayoutSetWrap(l, pangoWrapWordChar)
	} else if s.maxLines > 1 {
		pangoLayoutSetWrap(l, pangoWrapWord)
	}
	if s.maxLines > 0 && !(drawn && s.ellipsize == driver.EllipsizeClip) {
		mode := int32(pangoEllipsizeEnd)
		switch s.ellipsize {
		case driver.EllipsizeHead:
			mode = pangoEllipsizeStart
		case driver.EllipsizeMiddle:
			mode = pangoEllipsizeMiddle
		}
		pangoLayoutSetEllipsize(l, mode)
		pangoLayoutSetHeight(l, -int32(s.maxLines))
	}
}

// pangoWidth is the width Pango wraps and ellipsizes s at: none for a
// clipped line drawn whole, which its box cuts.
func (s *textSource) pangoWidth(width driver.Points, drawn bool) int32 {
	return pangoWrapWidth(width, drawn && s.maxLines == 1 && s.ellipsize == driver.EllipsizeClip)
}

// placeholderLen is the length in bytes of an attachment's placeholder.
var placeholderLen = len(string(driver.AttachmentRune))

func hasKeepAll(s *textSource) bool {
	if driver.KeepAllWords(s.style.Font.Language) {
		return true
	}
	for _, sp := range s.spans {
		if driver.KeepAllWords(sp.Style.Font.Language) {
			return true
		}
	}
	return false
}

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
	baseKeepAll := driver.KeepAllWords(s.style.Font.Language)
	hasHidden := s.hasHidden()
	addStyle(list, text, 0, len(text), s.style, s.ui, holes, false, hasHidden)
	for _, sp := range s.spans {
		addStyle(list, text, sp.Start, sp.End, sp.Style, s.ui, holes, baseKeepAll, hasHidden)
		if sp.Hidden {
			// Split PangoLayoutRun at hidden span boundaries without altering layout.
			start, end := min(max(sp.Start, 0), len(text)), min(max(sp.End, 0), len(text))
			if end > start {
				attr := pangoAttrForegroundAlphaNew(1)
				setAttributeRange(attr, uint32(start), uint32(end))
				pangoAttrListInsert(list, attr)
			}
		}
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

// addStyle adds s's font, its system families in the UI font ui, and its color, spacing and decoration over start:end, clipped to text bytes. Its line height leaves out holes, which are in order. With skipDecorations, as for a text with hidden spans, Pango gets no underline or strikethrough and draw paints them itself, leaving out the hidden spans'.
func addStyle(list uintptr, text string, start, end int, s driver.TextStyle, ui string, holes [][2]int, undoKeepAll, skipDecorations bool) {
	start, end = min(max(start, 0), len(text)), min(max(end, 0), len(text))
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
	if f.Language != "" {
		add(pangoAttrLanguageNew(pangoLanguageFromString(cstr(f.Language))))
	}
	if driver.KeepAllWords(f.Language) {
		for _, r := range driver.KeepAllWordRanges(text[start:end]) {
			if attr := pangoAttrAllowBreaksNew(0); attr != 0 {
				setAttributeRange(attr, uint32(start+r[0]), uint32(start+r[1]))
				pangoAttrListInsert(list, attr)
			}
		}
	} else if undoKeepAll {
		add(pangoAttrAllowBreaksNew(1))
	}
	add(pangoAttrFamilyNew(cstr(pangoFamilies(f.Family, ui))))
	if f.Size > 0 {
		add(pangoAttrSizeNewAbsolute(toPango(f.Size)))
	}
	if f.Weight > 0 {
		add(pangoAttrWeightNew(int32(f.Weight)))
	}
	if f.Italic {
		add(pangoAttrStyleNew(pangoStyleItalic))
	}
	if vars := driver.FontPangoVariations(f); vars != "" {
		desc := pangoFontDescriptionNew()
		pangoFontDescriptionSetVariations(desc, cstr(vars))
		add(pangoAttrFontDescNew(desc))
		pangoFontDescriptionFree(desc)
	}
	if features := driver.FontPangoFeatures(f); features != "" {
		add(pangoAttrFontFeaturesNew(cstr(features)))
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
				attr := pangoAttrLineHeightAbsoluteNew(toPango(s.LineHeight))
				setAttributeRange(attr, uint32(from), uint32(to))
				pangoAttrListInsert(list, attr)
			}
			from = max(from, h[1])
		}
	}
	// Pango draws no dotted or dashed lines, no thickness and no offset,
	// and strikes through solid: placeDecorations draws those lines (and every visible line when the text has hidden spans).
	if !skipDecorations && s.Decoration&driver.TextDecorationUnderline != 0 && !drawsUnderline(s) {
		line := int32(pangoUnderlineSingle)
		switch s.DecorationStyle {
		case driver.DecorationDouble:
			line = pangoUnderlineDouble
		case driver.DecorationWavy:
			line = pangoUnderlineError
		}
		add(pangoAttrUnderlineNew(line))
		if s.DecorationColor != 0 {
			add(pangoAttrUnderlineColorNew(rgb16(s.DecorationColor)))
		}
	}
	if !skipDecorations && s.Decoration&driver.TextDecorationLineThrough != 0 && !drawsStrike(s) {
		add(pangoAttrStrikethroughNew(1))
		if s.DecorationColor != 0 {
			add(pangoAttrStrikethroughColorNew(rgb16(s.DecorationColor)))
		}
	}
}

// drawsUnderline and drawsStrike report whether s's underline or
// strikethrough is drawn by placeDecorations rather than by Pango.
func drawsUnderline(s driver.TextStyle) bool {
	return s.DecorationStyle == driver.DecorationDotted || s.DecorationStyle == driver.DecorationDashed ||
		s.DecorationThickness > 0 || !s.UnderlineOffset.IsAuto()
}

func drawsStrike(s driver.TextStyle) bool {
	return s.DecorationStyle > driver.DecorationSolid || s.DecorationThickness > 0
}

// rgb16 returns c's channels as Pango's 16-bit ones.
func rgb16(c driver.Color) (r, g, b uint16) {
	return uint16(c>>24&0xff) * 0x101, uint16(c>>16&0xff) * 0x101, uint16(c>>8&0xff) * 0x101
}

// setAttributeRange fills a PangoAttribute's byte range, the first three
// fields of its public struct.
func setAttributeRange(attr uintptr, start, end uint32) {
	*(*uint32)(unsafe.Add(cPointer(attr), 8)) = start
	*(*uint32)(unsafe.Add(cPointer(attr), 12)) = end
}

// measure shapes and measures the text at width, in Pango units of the
// engine thread's font map, keeping fractional points for the core to
// round up to a device pixel.
func (p *preparedText) measure(width driver.Points) driver.Size {
	limit := p.pangoWidth(width, false)
	if p.layout == 0 {
		p.layout = newLayout(p.w.measureContext(p.style.RTL), &p.textSource, width)
	} else {
		pangoLayoutSetWidth(p.layout, limit)
	}
	var w, h int32
	pangoLayoutGetSize(p.layout, &w, &h)
	if spaced := p.spacedWidth(limit); spaced > w {
		w = spaced
	}
	return pangoMeasuredSize(w, h, width, limit)
}

// spacedWidth returns, in Pango units, the width at which the measuring layout breaks its lines as it has at limit: Pango leaves the letter spacing before a line's first cluster and after its last out of the line's width, but counts it when it breaks lines, so a text laid out again at its measured width would break once more (#489). It is no wider than limit, at which the lines already fit.
func (p *preparedText) spacedWidth(limit int32) int32 {
	if !p.letterSpaced() {
		return 0
	}
	iter := pangoLayoutGetIter(p.layout)
	defer pangoLayoutIterFree(iter)
	var need int32
	for {
		line := pangoLayoutIterGetLine(iter)
		from := int(*(*int32)(unsafe.Add(cPointer(line), 8)))
		to := min(from+int(*(*int32)(unsafe.Add(cPointer(line), 12))), len(p.text))
		if from < to {
			var ink, logical pangoRectangle
			pangoLayoutIterGetLineExtents(iter, &ink, &logical)
			_, last := utf8.DecodeLastRuneInString(p.text[from:to])
			before, _ := letterSpace(p.styleAt(from).LetterSpacing)
			_, after := letterSpace(p.styleAt(to - last).LetterSpacing)
			need = max(need, logical.Width+before+after)
		}
		if pangoLayoutIterNextLine(iter) == 0 {
			break
		}
	}
	return pangoSpacedWidth(need, limit)
}

// letterSpaced reports whether any of the text has positive letter spacing.
func (p *preparedText) letterSpaced() bool {
	if p.text == "" {
		return false
	}
	if p.style.LetterSpacing > 0 {
		return true
	}
	for _, sp := range p.spans {
		if sp.Style.LetterSpacing > 0 {
			return true
		}
	}
	return false
}

// Measure implements core.PreparedText.
func (p *preparedText) Measure(maxWidth driver.Points) driver.Size {
	size := p.measure(maxWidth)
	p.width, p.height = size.Width, size.Height
	return size
}

// Layout implements core.PreparedText.
func (p *preparedText) Layout(size driver.Size) driver.TextLayout {
	// Measure once at the final width so the engine's layout and the
	// main thread's agree on the text's height.
	measured := p.measure(size.Width)
	src := p.textSource
	src.spans = append([]driver.TextSpan(nil), p.spans...)
	l := &textLayout{textSource: src, width: size.Width, height: measured.Height}
	p.placeBackgrounds(l)
	p.placeDecorations(l)
	p.placeAttachments(l)
	p.placeLines(l)
	p.placeRuns(l)
	w := p.w
	l.carets.Set(func() driver.CaretLayout { return l.caretLines(w) })
	return l
}

// pangoDecorated reports whether Pango itself draws any underline or
// strikethrough on s (rather than placeDecorations).
func (s *textSource) pangoDecorated() bool {
	has := func(st driver.TextStyle) bool {
		return st.Decoration&driver.TextDecorationUnderline != 0 && !drawsUnderline(st) ||
			st.Decoration&driver.TextDecorationLineThrough != 0 && !drawsStrike(st)
	}
	if has(s.style) {
		return true
	}
	for _, sp := range s.spans {
		if has(sp.Style) {
			return true
		}
	}
	return false
}

// placeRuns extracts the already-shaped glyph runs from p.layout on the engine
// thread so the main thread builds GskTextNode directly without re-shaping.
func (p *preparedText) placeRuns(l *textLayout) {
	if p.text == "" || p.hasHidden() || p.pangoDecorated() || (p.maxLines > 0 && p.ellipsize == driver.EllipsizeClip) {
		return
	}
	const (
		pangoAnalysisFlagIsEllipsis = 1 << 1
		pangoGlyphVisAttrMask       = 0b11
	)
	iter := pangoLayoutGetIter(p.layout)
	defer pangoLayoutIterFree(iter)
	for more := true; more; more = pangoLayoutIterNextRun(iter) != 0 {
		run := pangoLayoutIterGetRun(iter)
		if run == 0 {
			continue
		}
		gi := (*pangoGlyphItem)(cPointer(run))
		if gi.item == nil || gi.glyphs == nil || gi.glyphs.numGlyphs <= 0 || gi.glyphs.glyphs == nil {
			continue
		}
		offset, length := int(gi.item.offset), int(gi.item.length)
		ellipsis := gi.item.analysis.flags&pangoAnalysisFlagIsEllipsis != 0
		if !ellipsis && p.isAttachment(offset, length) {
			continue
		}
		font := gi.item.analysis.font
		if font == 0 {
			continue
		}
		desc := p.w.fontDescriptionString(font)
		if desc == "" {
			l.runs = nil
			return
		}
		var logical pangoRectangle
		pangoLayoutIterGetRunExtents(iter, nil, &logical)
		baseline := pangoLayoutIterGetBaseline(iter)
		color := p.style.Color
		if !ellipsis && len(p.spans) > 0 {
			color = p.styleAt(offset).Color
		}
		if color&0xff == 0 {
			continue
		}
		n := int(gi.glyphs.numGlyphs)
		src := unsafe.Slice(gi.glyphs.glyphs, n)
		var widthUnits int32
		for i := range src {
			widthUnits += src[i].geometry.width
		}
		basePt := float32(fromPango(baseline))
		// Pango splits CJK text into one run per character: coalesce
		// contiguous runs sharing font, color and baseline into one
		// GskTextNode.
		if k := len(l.runs); k > 0 {
			last := &l.runs[k-1]
			if last.fontDesc == desc && last.color == color && last.baseline == basePt && last.endUnits == logical.X {
				start := len(last.glyphs)
				last.glyphs = slices.Grow(last.glyphs, n)[:start+n]
				for i := range src {
					g := src[i]
					g.attr &= pangoGlyphVisAttrMask
					last.glyphs[start+i] = g
				}
				last.endUnits = logical.X + widthUnits
				continue
			}
		}
		glyphs := make([]pangoGlyphInfo, n)
		for i := range src {
			g := src[i]
			g.attr &= pangoGlyphVisAttrMask
			glyphs[i] = g
		}
		l.runs = append(l.runs, shapedRun{
			fontDesc: desc,
			x:        float32(fromPango(logical.X)),
			baseline: basePt,
			endUnits: logical.X + widthUnits,
			color:    color,
			glyphs:   glyphs,
		})
	}
	l.runsBuilt = true
}

// HitTest implements driver.TextHitTester.
func (l *textLayout) HitTest(p driver.Point) driver.Caret { return l.carets.HitTest(p) }

// CaretRect implements driver.TextHitTester.
func (l *textLayout) CaretRect(c driver.Caret) (driver.Bounds, bool) { return l.carets.CaretRect(c) }

// RangeRects implements driver.TextHitTester.
func (l *textLayout) RangeRects(start, end int, out []driver.Bounds) []driver.Bounds {
	return l.carets.RangeRects(start, end, out)
}

// caretLines reads where the lines' characters are from a PangoLayout of
// the text as the main thread draws it, made on a context of hit
// testing's own so either thread may ask: each cluster's logical extents,
// its direction from the sign of its position's width.
func (l *textLayout) caretLines(w *window) driver.CaretLayout {
	w.hitMu.Lock()
	defer w.hitMu.Unlock()
	i := 0
	if l.style.RTL {
		i = 1
	}
	if w.hitMap[i] == 0 {
		w.hitMap[i] = newTextContext(l.style.RTL)
	}
	pl := pangoLayoutNew(w.hitMap[i])
	defer gObjectUnref(pl)
	applyLayout(pl, &l.textSource, l.width, true)
	text := l.text
	units := make([]int, len(text)+1) // UTF-16 offsets by byte offset
	for k, r := range text {
		n := 1
		if r >= 0x10000 {
			n = 2
		}
		for b := k + 1; b <= k+utf8.RuneLen(r) && b <= len(text); b++ {
			units[b] = units[k] + n
		}
	}
	unit := func(b int) int { return units[min(max(b, 0), len(text))] }
	var out driver.CaretLayout
	iter := pangoLayoutGetIter(pl)
	defer pangoLayoutIterFree(iter)
	var line, lineTo uintptr
	for {
		if at := pangoLayoutIterGetLine(iter); at != line {
			line = at
			from := int(*(*int32)(unsafe.Add(cPointer(at), 8)))
			to := from + int(*(*int32)(unsafe.Add(cPointer(at), 12)))
			lineTo = uintptr(to)
			var ink, logical pangoRectangle
			pangoLayoutIterGetLineExtents(iter, &ink, &logical)
			out = append(out, driver.CaretLine{Start: unit(from), End: unit(to), RTL: l.style.RTL,
				Box: driver.Bounds{Origin: driver.Point{X: fromPango(logical.X), Y: fromPango(logical.Y)},
					Size: driver.Size{Width: fromPango(logical.Width), Height: fromPango(logical.Height)}}})
		}
		if pangoLayoutIterGetRun(iter) != 0 && text != "" {
			idx := pangoLayoutIterGetIndex(iter)
			var ink, logical pangoRectangle
			pangoLayoutIterGetClusterExt(iter, &ink, &logical)
			var pos pangoRectangle
			pangoLayoutIndexToPos(pl, idx, &pos)
			cl := &out[len(out)-1]
			cl.Clusters = append(cl.Clusters, driver.CaretCluster{Start: int(idx), End: int(lineTo),
				Left: fromPango(logical.X), Right: fromPango(logical.X + logical.Width), RTL: pos.Width < 0})
		}
		if pangoLayoutIterNextCluster(iter) == 0 {
			break
		}
	}
	// The clusters came in visual order: each reaches to the next in
	// logical order, the last to the line's break.
	for k := range out {
		cl := &out[k]
		cs := cl.Clusters
		slices.SortFunc(cs, func(a, b driver.CaretCluster) int { return a.Start - b.Start })
		for j := range cs {
			if j+1 < len(cs) {
				cs[j].End = cs[j+1].Start
			}
			end := min(cs[j].End, len(text))
			for end > cs[j].Start && (text[end-1] == '\n' || text[end-1] == '\r') {
				end--
			}
			cs[j].Start, cs[j].End = unit(cs[j].Start), unit(end)
		}
		// The break itself takes no caret.
		for n := len(cs); n > 0 && cs[n-1].End <= cs[n-1].Start; n = len(cs) {
			cs = cs[:n-1]
		}
		cl.Clusters = cs
	}
	return out
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
		ext := p.w.fontExtent(font, p.ui)
		l.lines = append(l.lines, driver.TextLine{Start: min(from, len(p.text)), End: min(to, len(p.text)),
			Bounds: driver.Bounds{Origin: driver.Point{X: fromPango(logical.X), Y: fromPango(logical.Y)},
				Size: driver.Size{Width: fromPango(logical.Width), Height: fromPango(logical.Height)}},
			Ascender: fromPango(baseline - logical.Y), Descender: fromPango(logical.Y + logical.Height - baseline),
			CapHeight: p.w.capHeight(font, p.ui), XHeight: ext.XHeight})
		if pangoLayoutIterNextLine(iter) == 0 {
			break
		}
	}
}

// TextLines implements core.LineLayout.
func (l *textLayout) TextLines() []driver.TextLine { return l.lines }

// capHeight returns the height of f's capitals, from the ink of an "H", its system families in the UI font ui. The engine thread calls it; it caches what it measured.
func (w *window) capHeight(f driver.Font, ui string) driver.Points {
	key := fontKey{driver.FontKeyOf(f), ui}
	if h, ok := w.capHeights[key]; ok {
		return h
	}
	l := pangoLayoutNew(w.measureContext(false))
	attrs := pangoAttrListNew()
	addStyle(attrs, "H", 0, 1, driver.TextStyle{Font: f}, ui, nil, false, false)
	pangoLayoutSetAttributes(l, attrs)
	pangoAttrListUnref(attrs)
	pangoLayoutSetText(l, cstr("H"), 1)
	var ink, logical pangoRectangle
	pangoLayoutGetExtents(l, &ink, &logical)
	h := fromPango(pangoLayoutGetBaseline(l) - ink.Y)
	gObjectUnref(l)
	remember(&w.capHeights, key, h)
	return h
}

// blank reports whether c is a space or a line break, which a line's
// end hangs and a span's background leaves out.
func blank(c byte) bool { return c == ' ' || c == '\n' || c == '\r' || c == '\t' }

// placeBackgrounds puts a box per line behind each span with a background, within its font's ascent and descent.
func (p *preparedText) placeBackgrounds(l *textLayout) {
	has := false
	for _, sp := range p.spans {
		has = has || !sp.Hidden && sp.Background.Color&0xff != 0
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
			if sp.Hidden || bg.Color&0xff == 0 {
				continue
			}
			a, b := max(sp.Start, from), min(sp.End, to, len(p.text))
			for b > a && blank(p.text[b-1]) {
				b--
			}
			if a >= b {
				continue
			}
			font := p.w.fontExtent(sp.Style.Font, p.ui)
			top, bottom := bg.VerticalBounds(font.Ascent + font.Descent)
			h := float64(bottom - top)
			if h <= 0 {
				continue
			}
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
				l.under = append(l.under, rect{x: float64(x0), y: float64(baseline - font.Ascent + top),
					w: float64(x1 - x0), h: h, r: max(0, min(float64(bg.Radius), h/2))})
				l.underColors = append(l.underColors, bg.Color)
			}
			gFree(ranges)
		}
		if pangoLayoutIterNextLine(iter) == 0 {
			break
		}
	}
}

// lineMetrics are where a font puts its decoration lines: each line's
// top above the baseline, and its thickness.
type lineMetrics struct {
	underline, underlineThickness driver.Points
	strike, strikeThickness       driver.Points
}

// fontLines returns f's decoration lines, its system families in the UI font ui. The engine thread calls it; it caches what it read.
func (w *window) fontLines(f driver.Font, ui string) lineMetrics {
	key := fontKey{driver.FontKeyOf(f), ui}
	if m, ok := w.lineMetrics[key]; ok {
		return m
	}
	desc := fontDescription(f, ui)
	metrics := pangoContextGetMetrics(w.measureContext(false), desc, 0)
	pangoFontDescriptionFree(desc)
	m := lineMetrics{
		underline: fromPango(pangoFontMetricsUnderlinePos(metrics)), underlineThickness: fromPango(pangoFontMetricsUnderlineThick(metrics)),
		strike: fromPango(pangoFontMetricsStrikePos(metrics)), strikeThickness: fromPango(pangoFontMetricsStrikeThick(metrics)),
	}
	pangoFontMetricsUnref(metrics)
	remember(&w.lineMetrics, key, m)
	return m
}

// decorationLine is a decoration line placed in the content box: its
// box's top left, width and thickness.
type decorationLine struct {
	x, y, w, t float64
	color      driver.Color
	style      driver.TextDecorationStyle
}

// placeDecorations puts the underlines and strikethroughs Pango cannot
// draw on each line their runs reach, and every visible span's lines when the text has hidden spans.
func (p *preparedText) placeDecorations(l *textLayout) {
	type run struct {
		start, end int
		style      driver.TextStyle
	}
	hasHidden := p.hasHidden()
	var runs []run
	at := 0
	for _, sp := range p.spans {
		start, end := min(max(sp.Start, at), len(p.text)), min(max(sp.End, at), len(p.text))
		if start > at {
			runs = append(runs, run{at, start, p.style})
		}
		if !sp.Hidden && end > start {
			runs = append(runs, run{start, end, sp.Style})
		}
		at = max(at, end)
	}
	if at < len(p.text) {
		runs = append(runs, run{at, len(p.text), p.style})
	}
	runs = slices.DeleteFunc(runs, func(r run) bool {
		d := r.style.Decoration
		return !(d&driver.TextDecorationUnderline != 0 && (hasHidden || drawsUnderline(r.style)) ||
			d&driver.TextDecorationLineThrough != 0 && (hasHidden || drawsStrike(r.style)))
	})
	if len(runs) == 0 || p.text == "" {
		return
	}
	iter := pangoLayoutGetIter(p.layout)
	defer pangoLayoutIterFree(iter)
	for {
		line := pangoLayoutIterGetLine(iter)
		from := int(*(*int32)(unsafe.Add(cPointer(line), 8)))
		to := from + int(*(*int32)(unsafe.Add(cPointer(line), 12)))
		baseline := float64(fromPango(pangoLayoutIterGetBaseline(iter)))
		for _, r := range runs {
			a, b := max(r.start, from), min(r.end, to, len(p.text))
			for b > a && blank(p.text[b-1]) {
				b--
			}
			if a >= b {
				continue
			}
			st := r.style
			m := p.w.fontLines(st.Font, p.ui)
			color := st.Color
			if st.DecorationColor != 0 {
				color = st.DecorationColor
			}
			var placed []decorationLine
			thick := func(font driver.Points) float64 {
				if st.DecorationThickness > 0 {
					return float64(st.DecorationThickness)
				}
				return float64(max(font, 1))
			}
			if st.Decoration&driver.TextDecorationUnderline != 0 && (hasHidden || drawsUnderline(st)) {
				top := baseline - float64(m.underline)
				if off, ok := st.UnderlineOffset.Points(); ok {
					top = baseline + float64(off)
				}
				placed = append(placed, decorationLine{y: top, t: thick(m.underlineThickness), color: color, style: st.DecorationStyle})
			}
			if st.Decoration&driver.TextDecorationLineThrough != 0 && (hasHidden || drawsStrike(st)) {
				t := thick(m.strikeThickness)
				// The font's line stays centered where it was.
				top := baseline - float64(m.strike) + float64(max(m.strikeThickness, 1))/2 - t/2
				placed = append(placed, decorationLine{y: top, t: t, color: color, style: st.DecorationStyle})
			}
			var ranges uintptr
			var n int32
			pangoLayoutLineGetXRanges(line, int32(a), int32(b), &ranges, &n)
			if ranges == 0 {
				continue
			}
			xs := unsafe.Slice((*int32)(cPointer(ranges)), 2*n)
			for i := 0; i < int(n); i++ {
				x0, x1 := float64(fromPango(xs[2*i])), float64(fromPango(xs[2*i+1]))
				if x1 <= x0 {
					continue
				}
				for _, d := range placed {
					d.x, d.w = x0, x1-x0
					l.over = append(l.over, d)
				}
			}
			gFree(ranges)
		}
		if pangoLayoutIterNextLine(iter) == 0 {
			break
		}
	}
}

// bounds returns the area d paints in the content box, with room for
// double lines, waves and antialiasing.
func (d decorationLine) bounds() (x0, y0, x1, y1 float64) {
	pad := 0.0
	hpad := 1.0
	switch d.style {
	case driver.DecorationDouble:
		pad = d.t
	case driver.DecorationWavy:
		pad = max(1.5*d.t, 2)
		hpad = max(d.t/2, 1) + 1
	}
	return d.x - hpad, d.y - pad - 1, d.x + d.w + hpad, d.y + d.t + pad + 1
}

// decorationBounds returns the whole-point box covering l's drawn
// decoration lines when placed at x, y, or false when there are none.
func (l *textLayout) decorationBounds(x, y driver.Points) (grapheneRect, bool) {
	if len(l.over) == 0 {
		return grapheneRect{}, false
	}
	ox, oy := float64(x), float64(y)
	x0, y0 := math.Inf(1), math.Inf(1)
	x1, y1 := math.Inf(-1), math.Inf(-1)
	for _, d := range l.over {
		a, b, c, e := d.bounds()
		x0, y0 = min(x0, ox+a), min(y0, oy+b)
		x1, y1 = max(x1, ox+c), max(y1, oy+e)
	}
	x0, y0 = math.Floor(x0), math.Floor(y0)
	x1, y1 = math.Ceil(x1), math.Ceil(y1)
	return grapheneRect{float32(x0), float32(y0), float32(x1 - x0), float32(y1 - y0)}, true
}

// contentBounds returns the Cairo node bounds holding l's glyphs, span
// backgrounds and decoration lines when placed at x, y inside a view of
// size width by height.
func (l *textLayout) contentBounds(x, y, width, height float32) grapheneRect {
	x0 := float64(min(0, x) - textOverhang)
	y0 := float64(min(0, y) - textOverhang)
	x1 := float64(max(width, x+float32(l.width)) + textOverhang)
	y1 := float64(max(height, y+float32(l.height)) + textOverhang)
	ox, oy := float64(x), float64(y)
	for _, r := range l.under {
		x0, y0 = min(x0, ox+r.x-1), min(y0, oy+r.y-1)
		x1, y1 = max(x1, ox+r.x+r.w+1), max(y1, oy+r.y+r.h+1)
	}
	if dec, ok := l.decorationBounds(driver.Points(x), driver.Points(y)); ok {
		x0, y0 = min(x0, float64(dec.x)), min(y0, float64(dec.y))
		x1, y1 = max(x1, float64(dec.x+dec.w)), max(y1, float64(dec.y+dec.h))
	}
	x0, y0 = math.Floor(x0), math.Floor(y0)
	x1, y1 = math.Ceil(x1), math.Ceil(y1)
	return grapheneRect{float32(x0), float32(y0), float32(x1 - x0), float32(y1 - y0)}
}

// draw draws the line with the content box's top left at x, y: a box,
// two boxes a thickness apart, or a dotted, dashed or wavy stroke along
// its middle.
func (d decorationLine) draw(cr uintptr, x, y float64) {
	x0, top, t := x+d.x, y+d.y, d.t
	x1, mid := x0+d.w, top+t/2
	cairoSave(cr)
	setColor(cr, d.color)
	switch d.style {
	case driver.DecorationDouble:
		cairoRectangle(cr, x0, top-t, d.w, t)
		cairoRectangle(cr, x0, top+t, d.w, t)
		cairoFill(cr)
	case driver.DecorationDotted, driver.DecorationDashed:
		dash := [2]float64{t, t}
		if d.style == driver.DecorationDashed {
			dash = [2]float64{3 * t, 2 * t}
		}
		cairoSetLineWidth(cr, t)
		cairoSetDash(cr, &dash[0], 2, 0)
		cairoMoveTo(cr, x0, mid)
		cairoLineTo(cr, x1, mid)
		cairoStroke(cr)
	case driver.DecorationWavy:
		amp := max(1.5*t, 2)
		step := 2 * amp
		cairoSetLineWidth(cr, t)
		cairoMoveTo(cr, x0, mid)
		up := true
		for cx := x0; cx < x1; cx += step {
			nx := min(cx+step, x1)
			peak := mid + amp
			if up {
				peak = mid - amp
			}
			// The quadratic through peak, as a cubic.
			cairoCurveTo(cr, cx+2*((cx+nx)/2-cx)/3, mid+2*(peak-mid)/3, nx+2*((cx+nx)/2-nx)/3, mid+2*(peak-mid)/3, nx, mid)
			up = !up
		}
		cairoStroke(cr)
	default:
		cairoRectangle(cr, x0, top, d.w, t)
		cairoFill(cr)
	}
	cairoRestore(cr)
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
				l.attachments[i] = attachmentBox{shown: true, box: driver.Bounds{
					Origin: driver.Point{X: fromPango(logical.X), Y: top}, Size: at.Size}}
			}
		}
		if pangoLayoutIterNextRun(iter) == 0 {
			break
		}
	}
}

// Attachment implements core.AttachmentLayout.
func (l *textLayout) Attachment(i int) (driver.Bounds, bool) {
	if i < 0 || i >= len(l.attachments) {
		return driver.Bounds{}, false
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

// snapshotBackgrounds paints the span boxes before the selection.
func (l *textLayout) snapshotBackgrounds(snap uintptr, x, y driver.Points, tint *driver.Color) {
	for i, r := range l.under {
		c := l.underColors[i]
		if tint != nil {
			c = tint.Faded(float32(c&0xff) / 255)
		}
		rr := roundedRect(float32(r.x)+float32(x), float32(r.y)+float32(y), float32(r.w), float32(r.h), float32(r.r))
		fillRounded(snap, &rr, c)
	}
}

// mainFonts caches main-thread PangoFont objects loaded from GTK's default
// PangoCairoFontMap by font description string, plus one reusable
// PangoGlyphString for building GskTextNode without per-run C allocations.
var mainFonts struct {
	context  uintptr
	fontMap  uintptr
	fonts    map[string]uintptr
	glyphStr uintptr
}

func mainFont(desc string) uintptr {
	if mainFonts.fonts != nil {
		if f, ok := mainFonts.fonts[desc]; ok {
			return f
		}
	}
	if mainFonts.fontMap == 0 {
		mainFonts.fontMap = pangoCairoFontMapGetDefault()
		if mainFonts.fontMap == 0 {
			return 0
		}
	}
	if mainFonts.context == 0 {
		mainFonts.context = pangoFontMapCreateContext(mainFonts.fontMap)
		if mainFonts.context == 0 {
			return 0
		}
	}
	if mainFonts.fonts == nil {
		mainFonts.fonts = map[string]uintptr{}
	} else if len(mainFonts.fonts) >= maxFontMetrics {
		for _, f := range mainFonts.fonts {
			if f != 0 {
				gObjectUnref(f)
			}
		}
		clear(mainFonts.fonts)
	}
	d := pangoFontDescriptionFromString(cstr(desc))
	if d == 0 {
		return 0
	}
	f := pangoFontMapLoadFont(mainFonts.fontMap, mainFonts.context, d)
	pangoFontDescriptionFree(d)
	mainFonts.fonts[desc] = f
	return f
}

func clearMainFonts() {
	for _, f := range mainFonts.fonts {
		if f != 0 {
			gObjectUnref(f)
		}
	}
	clear(mainFonts.fonts)
	if mainFonts.context != 0 {
		gObjectUnref(mainFonts.context)
		mainFonts.context = 0
	}
}

func fillScratchGlyphString(glyphs []pangoGlyphInfo) uintptr {
	if mainFonts.glyphStr == 0 {
		mainFonts.glyphStr = pangoGlyphStringNew()
	}
	pangoGlyphStringSetSize(mainFonts.glyphStr, int32(len(glyphs)))
	gs := (*pangoGlyphString)(cPointer(mainFonts.glyphStr))
	copy(unsafe.Slice(gs.glyphs, len(glyphs)), glyphs)
	return mainFonts.glyphStr
}

// buildDirectNode constructs a cached GskRenderNode directly from the engine
// thread's shaped glyph runs via gsk_text_node_new, avoiding Pango/HarfBuzz
// re-shaping on the GTK main thread.
func (l *textLayout) buildDirectNode(x, y driver.Points) (uintptr, bool) {
	if !l.runsBuilt {
		return 0, false
	}
	if len(l.runs) == 0 && len(l.over) == 0 {
		return 0, true
	}
	fx, fy := float32(x), float32(y)
	if len(l.runs) == 1 && len(l.over) == 0 {
		run := &l.runs[0]
		font := mainFont(run.fontDesc)
		if font == 0 {
			return 0, false
		}
		gs := fillScratchGlyphString(run.glyphs)
		c := rgba(run.color)
		pt := graphenePoint{fx + run.x, fy + run.baseline}
		return gskTextNodeNew(font, gs, &c, &pt), true
	}
	snap := gtkSnapshotNew()
	for i := range l.runs {
		run := &l.runs[i]
		font := mainFont(run.fontDesc)
		if font == 0 {
			if n := gtkSnapshotFreeToNode(snap); n != 0 {
				gskRenderNodeUnref(n)
			}
			return 0, false
		}
		gs := fillScratchGlyphString(run.glyphs)
		c := rgba(run.color)
		pt := graphenePoint{fx + run.x, fy + run.baseline}
		if n := gskTextNodeNew(font, gs, &c, &pt); n != 0 {
			gtkSnapshotAppendNode(snap, n)
			gskRenderNodeUnref(n)
		}
	}
	if bounds, ok := l.decorationBounds(x, y); ok {
		cr := gtkSnapshotAppendCairo(snap, &bounds)
		for _, d := range l.over {
			d.draw(cr, float64(x), float64(y))
		}
		cairoDestroy(cr)
	}
	return gtkSnapshotFreeToNode(snap), true
}

// buildLayoutNode builds a cached GskRenderNode via a main-thread PangoLayout
// for cases the direct shaped-run path leaves to Pango (clipped ellipsis,
// hidden spans, Pango-drawn decorations, or missing font fallback).
func (l *textLayout) buildLayoutNode(x, y driver.Points) uintptr {
	l.ensure(scratchCairo())
	snap := gtkSnapshotNew()
	clip := l.maxLines > 0 && l.ellipsize == driver.EllipsizeClip
	if clip {
		box := grapheneRect{float32(x), float32(y), float32(l.width), float32(l.height)}
		gtkSnapshotPushClip(snap, &box)
	}
	if l.hasHidden() {
		// Draw visible runs and decorations through Cairo, skipping hidden runs so their glyphs, color emoji and shadow are never drawn.
		bounds := l.contentBounds(float32(x), float32(y), float32(l.width), float32(l.height))
		cr := gtkSnapshotAppendCairo(snap, &bounds)
		l.drawRuns(cr, x, y)
		for _, d := range l.over {
			d.draw(cr, float64(x), float64(y))
		}
		cairoDestroy(cr)
	} else {
		if x != 0 || y != 0 {
			gtkSnapshotSave(snap)
			pt := graphenePoint{float32(x), float32(y)}
			gtkSnapshotTranslate(snap, &pt)
		}
		// Every span sets its color; this is what none would.
		black := gdkRGBA{a: 1}
		gtkSnapshotAppendLayout(snap, l.layout, &black)
		if x != 0 || y != 0 {
			gtkSnapshotRestore(snap)
		}
		if bounds, ok := l.decorationBounds(x, y); ok {
			cr := gtkSnapshotAppendCairo(snap, &bounds)
			for _, d := range l.over {
				d.draw(cr, float64(x), float64(y))
			}
			cairoDestroy(cr)
		}
	}
	if clip {
		gtkSnapshotPop(snap)
	}
	return gtkSnapshotFreeToNode(snap)
}

// snapshot appends the glyphs with their top-left at x, y to snap from l's
// cached GskRenderNode: direct GskTextNodes built from the engine thread's
// shaped runs on the common path (avoiding main-thread Pango re-shaping), or a
// cached PangoLayout node for clipped ellipsis, hidden spans and Pango-drawn
// decorations. snapshotBackgrounds paints the spans' backgrounds first.
func (l *textLayout) snapshot(snap uintptr, x, y driver.Points) {
	if !l.nodeReady || l.nodeX != x || l.nodeY != y {
		if l.node != 0 {
			gskRenderNodeUnref(l.node)
			l.node = 0
		}
		node, ok := l.buildDirectNode(x, y)
		if !ok {
			node = l.buildLayoutNode(x, y)
		}
		l.node, l.nodeReady, l.nodeX, l.nodeY = node, true, x, y
	}
	if l.node != 0 {
		gtkSnapshotAppendNode(snap, l.node)
	}
}

// scratch is the Cairo context the main thread makes and updates text
// layouts with outside a drawing: an image's, as the nodes' were.
var scratch uintptr

func scratchCairo() uintptr {
	if scratch == 0 {
		scratch = cairoCreate(cairoImageSurfaceCreate(0, 1, 1))
	}
	return scratch
}

// drawBackgrounds paints the span boxes into a Cairo target.
func (l *textLayout) drawBackgrounds(cr uintptr, x, y driver.Points) {
	for i, r := range l.under {
		r.x += float64(x)
		r.y += float64(y)
		setColor(cr, l.underColors[i])
		roundRect(cr, r)
		cairoFill(cr)
	}
}

// draw paints the glyphs and their decorations at x, y.
func (l *textLayout) draw(cr uintptr, x, y driver.Points) {
	l.ensure(cr)
	clip := l.maxLines > 0 && l.ellipsize == driver.EllipsizeClip
	if clip {
		cairoSave(cr)
		cairoRectangle(cr, float64(x), float64(y), float64(l.width), float64(l.height))
		cairoClip(cr)
	}
	if l.hasHidden() {
		l.drawRuns(cr, x, y)
	} else {
		cairoMoveTo(cr, float64(x), float64(y))
		pangoCairoShowLayout(cr, l.layout)
	}
	for _, d := range l.over {
		d.draw(cr, float64(x), float64(y))
	}
	if clip {
		cairoRestore(cr)
	}
}

// drawRuns draws l's visible glyph runs into cr, skipping hidden runs and attachment placeholders so their glyphs, color emoji and shadow are not drawn.
func (l *textLayout) drawRuns(cr uintptr, x, y driver.Points) {
	if l.text == "" {
		return
	}
	cText := pangoLayoutGetText(l.layout)
	iter := pangoLayoutGetIter(l.layout)
	defer pangoLayoutIterFree(iter)
	const isEllipsis = 1 << 2 // PANGO_ANALYSIS_FLAG_IS_ELLIPSIS
	for {
		if run := pangoLayoutIterGetRun(iter); run != 0 {
			item := *(**pangoItem)(cPointer(run))
			offset, length := int(item.offset), int(item.length)
			ellipsis := item.analysis.flags&isEllipsis != 0
			if ellipsis || (!l.isHidden(offset) && !l.isAttachment(offset, length)) {
				var logical pangoRectangle
				pangoLayoutIterGetRunExtents(iter, nil, &logical)
				baseline := pangoLayoutIterGetBaseline(iter)
				color := l.style.Color
				if !ellipsis {
					color = l.styleAt(offset).Color
				}
				cairoSave(cr)
				setColor(cr, color)
				cairoMoveTo(cr, float64(x)+float64(fromPango(logical.X)), float64(y)+float64(fromPango(baseline)))
				pangoCairoShowGlyphItem(cr, cText, run)
				cairoRestore(cr)
			}
		}
		if pangoLayoutIterNextRun(iter) == 0 {
			break
		}
	}
}

// indexAt returns the byte offset of the character nearest x, y, in the
// text's points.
func (l *textLayout) indexAt(x, y driver.Points) int {
	if l.text == "" {
		return 0
	}
	if l.layout == 0 {
		l.ensure(scratchCairo())
	}
	var index, trailing int32
	pangoLayoutXyToIndex(l.layout, int32(x*pangoScale), int32(y*pangoScale), &index, &trailing)
	return int(index)
}

// glyphAt returns the byte offset of the character at x, y, in the text's points, or -1 off the glyphs. It makes the PangoLayout a text that has not drawn yet lacks, as snapshot does.
func (l *textLayout) glyphAt(x, y driver.Points) int {
	if l.text == "" {
		return -1
	}
	if l.layout == 0 {
		l.ensure(scratchCairo())
	}
	var index, trailing int32
	if pangoLayoutXyToIndex(l.layout, int32(x*pangoScale), int32(y*pangoScale), &index, &trailing) == 0 {
		return -1
	}
	return int(index)
}

// Height returns the height the text laid out at.
func (l *textLayout) Height() driver.Points { return l.height }

// Release implements core.TextLayout on the main thread.
func (l *textLayout) Release() {
	if l.node != 0 {
		gskRenderNodeUnref(l.node)
		l.node = 0
	}
	l.nodeReady = false
	if l.layout != 0 {
		gObjectUnref(l.layout)
		l.layout = 0
	}
}

// fontKey is what the engine thread's font metrics are cached by: a font, and the UI font its system families took.
type fontKey struct {
	font driver.FontKey
	ui   string
}

// maxFontMetrics bounds each cache of font metrics, which a font size
// slider or a change of text scale would otherwise grow without end.
const maxFontMetrics = 256

// remember caches v at key in m, emptying m first when it is full.
func remember[T any](m *map[fontKey]T, key fontKey, v T) {
	if *m == nil {
		*m = map[fontKey]T{}
	} else if len(*m) >= maxFontMetrics {
		clear(*m)
	}
	(*m)[key] = v
}
