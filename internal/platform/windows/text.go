//go:build windows

package windows

// The DirectWrite text engine (engine goroutine) and text drawing (main
// thread).
//
// Prepare shapes the text once into an IDWriteTextLayout of DirectWrite's
// shared factory, which is free-threaded; Measure breaks it again at each
// width asked. Layout makes a layout of its own at the final width and
// hands it to the main thread, which draws it with Direct2D's
// DrawTextLayout: DirectWrite's font fallback picks faces for what the
// text's font lacks, and color fonts draw emoji in color. Span colors are
// drawing effects, brushes set on the layout when it is first drawn on a
// device. Span backgrounds and decorations are boxes found by hit testing,
// at layout, and drawn by the painter, as NSH's text.rs draws them, so a
// struck-through space is struck too.
//
// Line heights are DirectWrite's own (each line as tall as its fonts and
// attachments) unless the text sets LineHeight. Then lines are laid out on
// that uniform pitch, the baseline centering the text's font in it, and a
// line whose attachments reach past it grows: it is drawn moved down by
// what the lines above it grew, clipped to its band.
//
// Attachments are inline objects of their size: DirectWrite gives them
// their place in the line and the engine their view.

import (
	"math"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unicode/utf8"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	// unbounded is the width of a layout with no limit: small enough that
	// positions in centered or trailing text keep their fractions.
	unbounded = 1 << 14
	// unboundedHeight is the height of a layout with no limit.
	unboundedHeight = 1e7
)

// drawnText is a TextLayout of this backend, drawn by it.
type drawnText interface {
	driver.TextLayout
	inkBounds(box driver.Bounds) rectF
	drawBackgrounds(p *painter, x, y float32, tint *driver.Color)
	draw(p *painter, x, y float32)
}

// faceMetrics are what a font's lines and decorations are placed by, in
// ems.
type faceMetrics struct {
	ascent, descent, xHeight      float32
	capHeight                     float32
	underline, underlineThickness float32 // the position above the baseline
	strike, strikeThickness       float32
}

// defaultFaceMetrics stand for a font DirectWrite cannot find.
var defaultFaceMetrics = faceMetrics{ascent: 1.07, descent: 0.26, xHeight: 0.5, capHeight: 0.7,
	underline: -0.1, underlineThickness: 0.05, strike: 0.3, strikeThickness: 0.05}

type faceKey struct {
	family   string
	language string
	weight   driver.FontWeight
	italic   bool
}

var (
	facesMu sync.Mutex
	faces   = map[faceKey]faceMetrics{}
	// theSystemFonts is DirectWrite's collection of the system's fonts, or
	// nil.
	theSystemFonts = sync.OnceValue(func() *comObject {
		f := dwriteFactory()
		if f == nil {
			return nil
		}
		var c *comObject
		if failed(f.call(dwGetSystemFontCollection, uintptr(unsafe.Pointer(&c)), 0)) {
			return nil
		}
		return c
	})
)

var (
	fallbacksMu sync.Mutex
	// fallbacks holds an IDWriteFontFallback per FontFamily list of more than one family, which formats share and keep until the app's fonts change.
	fallbacks = map[string]*comObject{}
)

// unicodeRange is a DWRITE_UNICODE_RANGE.
type unicodeRange struct {
	first, last uint32
}

// fallbackNames returns the Windows family each family of list names, in order.
func fallbackNames(list string) []string {
	families := driver.FontFamilies(list)
	names := make([]string, 0, len(families))
	for _, f := range families {
		switch f.Kind {
		case driver.FamilyMonospace:
			names = append(names, "Consolas")
		case driver.FamilySerif:
			names = append(names, "Times New Roman")
		case driver.FamilySystem, driver.FamilyRounded:
			names = append(names, defaultFontName)
		default:
			names = append(names, f.Name)
		}
	}
	return names
}

// fontFallback returns a font fallback that sets each character in the first family of list that has it, then where the system's fallback would, or nil for a list of one family or a Windows without IDWriteFactory2: DirectWrite's own fallback follows the system's locale rather than the list. The caller releases it.
func fontFallback(list string) *comObject {
	names := fallbackNames(list)
	if len(names) < 2 {
		return nil
	}
	fallbacksMu.Lock()
	defer fallbacksMu.Unlock()
	if f, ok := fallbacks[list]; ok {
		f.addRef()
		return f
	}
	factory := dwriteFactory()
	if factory == nil {
		return nil
	}
	f2 := factory.query(&iidDWriteFactory2)
	if f2 == nil {
		return nil
	}
	defer f2.release()
	var builder, system, fallback *comObject
	defer releaseAll(&builder, &system)
	if failed(f2.call(dw2CreateFontFallbackBuilder, uintptr(unsafe.Pointer(&builder)))) {
		return nil
	}
	fonts := fontCollection()
	if fonts != nil {
		defer fonts.release()
	}
	all := unicodeRange{first: 0, last: 0x10ffff}
	targets := make([]*uint16, len(names))
	for i, name := range names {
		targets[i] = wide(name)
	}
	if failed(builder.callF(fallbackAddMapping, 1<<7, uintptr(unsafe.Pointer(&all)), 1,
		uintptr(unsafe.Pointer(&targets[0])), uintptr(len(targets)), fonts.ptr(), 0, 0, f32(1))) {
		return nil
	}
	// A custom fallback replaces the system's, which still covers what no family of the list has, such as emoji.
	if !failed(f2.call(dw2GetSystemFontFallback, uintptr(unsafe.Pointer(&system)))) {
		builder.call(fallbackAddMappings, system.ptr())
	}
	if failed(builder.call(fallbackCreateFontFallback, uintptr(unsafe.Pointer(&fallback)))) {
		return nil
	}
	fallbacks[list] = fallback
	fallback.addRef()
	return fallback
}

const maxTypographies = 256

var (
	typographiesMu sync.Mutex
	// typographies holds an IDWriteTypography per resolved feature set, which
	// texts share and AddRef when borrowing.
	typographies = map[string]*comObject{}
)

// typography returns an AddRef'd IDWriteTypography that applies f's OpenType
// features (merged from Variant and Features), or nil when it has none. The
// caller releases it.
func typography(f driver.Font) *comObject {
	feats := driver.ResolvedFontFeatures(f)
	if len(feats) == 0 {
		return nil
	}
	key := driver.FontFeaturesKey(f)
	typographiesMu.Lock()
	defer typographiesMu.Unlock()
	if t, ok := typographies[key]; ok {
		if t != nil {
			t.addRef()
		}
		return t
	}
	var t *comObject
	if factory := dwriteFactory(); factory == nil || failed(factory.call(dwCreateTypography, uintptr(unsafe.Pointer(&t)))) {
		t = nil
	}
	for _, feat := range feats {
		// A DWRITE_FONT_FEATURE is a tag and a parameter, passed by value
		// as a text range's start and length are.
		name := driver.FontTagLE(feat.Tag)
		param := uint32(max(feat.Value, 0))
		if t != nil {
			t.callRange(typographyAddFontFeature, 0, name, param)
		}
	}
	if len(typographies) >= maxTypographies {
		for k, old := range typographies {
			old.release()
			delete(typographies, k)
		}
	}
	if t != nil {
		t.addRef()
	}
	typographies[key] = t
	return t
}

// axisValues converts f's resolved variable font axes into DirectWrite's
// DWRITE_FONT_AXIS_VALUE array.
func axisValues(f driver.Font) []fontAxisValue {
	axes := driver.ResolvedFontAxes(f)
	if len(axes) == 0 {
		return nil
	}
	out := make([]fontAxisValue, len(axes))
	for i, a := range axes {
		out[i] = fontAxisValue{axisTag: driver.FontTagLE(a.Tag), value: a.Value}
	}
	return out
}

// fontName returns the family DirectWrite is asked for.
func fontName(family string) string {
	switch family {
	case "":
		return defaultFontName
	case driver.FontMonospace:
		return "Consolas"
	}
	return firstInstalled(family)
}

// familiesMu guards families.
var (
	familiesMu sync.Mutex
	families   = map[string]string{}
)

// firstInstalled is the first family of a fontFamily list the system has,
// else the first: DirectWrite takes one family.
func firstInstalled(list string) string {
	familiesMu.Lock()
	defer familiesMu.Unlock()
	if name, ok := families[list]; ok {
		return name
	}
	names := fallbackNames(list)
	if len(names) == 0 {
		return defaultFontName
	}
	name := names[0]
	if len(names) > 1 {
		if c := systemFonts(); c != nil {
			defer c.release()
			for _, n := range names {
				var index uint32
				var exists int32
				c.call(collectionFindFamilyName, uintptr(unsafe.Pointer(wide(n))), uintptr(unsafe.Pointer(&index)),
					uintptr(unsafe.Pointer(&exists)))
				if exists != 0 {
					name = n
					break
				}
			}
		}
	}
	families[list] = name
	return name
}

func fontWeight(w driver.FontWeight) uintptr {
	if w == 0 {
		return 400
	}
	return uintptr(w)
}

func fontStyle(italic bool) uintptr {
	if italic {
		return dwStyleItalic
	}
	return dwStyleNormal
}

// systemFonts is the font collection text looks families up in, held for
// the caller, which releases it: the system's, with the app's fonts when
// it registered any, or nil.
func systemFonts() *comObject {
	if c := fontCollection(); c != nil {
		return c
	}
	c := theSystemFonts()
	if c != nil {
		c.addRef()
	}
	return c
}

// metricsOf returns the metrics of the face font is set in, the default
// family's when the system has no such family.
func metricsOf(font driver.Font) faceMetrics {
	key := faceKey{fontName(font.Family), font.Language, font.Weight, font.Italic}
	facesMu.Lock()
	defer facesMu.Unlock()
	if m, ok := faces[key]; ok {
		return m
	}
	m := defaultFaceMetrics
	if c := systemFonts(); c != nil {
		defer c.release()
		var index uint32
		var exists int32
		name := wide(key.family)
		c.call(collectionFindFamilyName, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&index)),
			uintptr(unsafe.Pointer(&exists)))
		if exists == 0 {
			c.call(collectionFindFamilyName, uintptr(unsafe.Pointer(wide(defaultFontName))), uintptr(unsafe.Pointer(&index)),
				uintptr(unsafe.Pointer(&exists)))
		}
		var family, f *comObject
		if exists != 0 && !failed(c.call(collectionGetFontFamily, uintptr(index), uintptr(unsafe.Pointer(&family)))) {
			if !failed(family.call(familyGetFirstMatchingFont, fontWeight(key.weight), dwStretchNormal, fontStyle(key.italic),
				uintptr(unsafe.Pointer(&f)))) {
				var fm fontMetrics
				f.call(fontGetMetrics, uintptr(unsafe.Pointer(&fm)))
				if em := float32(fm.designUnitsPerEm); em > 0 {
					m = faceMetrics{
						ascent: float32(fm.ascent) / em, descent: float32(fm.descent) / em, xHeight: float32(fm.xHeight) / em,
						capHeight: float32(fm.capHeight) / em,
						underline: float32(fm.underlinePosition) / em, underlineThickness: float32(fm.underlineThickness) / em,
						strike: float32(fm.strikethroughPosition) / em, strikeThickness: float32(fm.strikethroughThickness) / em,
					}
					if m.xHeight <= 0 {
						m.xHeight = 0.5
					}
				}
				f.release()
			}
			family.release()
		}
	}
	faces[key] = m
	return m
}

// textFormats caches immutable configured IDWriteTextFormat objects keyed by
// their font, paragraph, wrapping, spacing, and trimming configuration.
const maxTextFormats = 256

var (
	textFormatsMu sync.Mutex
	textFormats   = map[formatKey]*comObject{}
)

func clearTextFormats() {
	textFormatsMu.Lock()
	for k, f := range textFormats {
		f.release()
		delete(textFormats, k)
	}
	textFormatsMu.Unlock()
}

// cachedTextFormat returns an AddRef'd configured IDWriteTextFormat for key, or nil.
func cachedTextFormat(key formatKey, axes map[string]float32) *comObject {
	textFormatsMu.Lock()
	if f := textFormats[key]; f != nil {
		f.addRef()
		textFormatsMu.Unlock()
		return f
	}
	textFormatsMu.Unlock()

	font := driver.Font{
		Family:   key.family,
		Language: key.language,
		Weight:   key.weight,
		Italic:   key.italic,
		Size:     driver.Points(key.size),
		Axes:     axes,
	}
	format := textFormat(font)
	if format == nil {
		return nil
	}
	align := uintptr(dwAlignLeading)
	switch key.align {
	case driver.TextAlignCenter:
		align = dwAlignCenter
	case driver.TextAlignEnd:
		align = dwAlignTrailing
	case driver.TextAlignJustify:
		align = dwAlignJustified
	}
	format.call(formatSetTextAlignment, align)
	if key.rtl {
		// Leading and trailing alignments follow it.
		format.call(formatSetReadingDirection, dwReadingRTL)
	}
	if key.wrap != 0 {
		format.call(formatSetWordWrapping, uintptr(key.wrap))
	}
	if key.step > 0 {
		format.callF(formatSetLineSpacing, 0b110, dwLineSpacingUniform, f32(key.step), f32(key.baseline))
	}
	if key.trim {
		var sign *comObject
		dwriteFactory().call(dwCreateEllipsisTrimmingSign, format.ptr(), uintptr(unsafe.Pointer(&sign)))
		trim := trimming{granularity: dwTrimmingCharacter}
		format.call(formatSetTrimming, uintptr(unsafe.Pointer(&trim)), sign.ptr())
		sign.release()
	}

	textFormatsMu.Lock()
	if existing := textFormats[key]; existing != nil {
		existing.addRef()
		textFormatsMu.Unlock()
		format.release()
		return existing
	}
	if len(textFormats) >= maxTextFormats {
		for k, f := range textFormats {
			f.release()
			delete(textFormats, k)
		}
	}
	format.addRef()
	textFormats[key] = format
	textFormatsMu.Unlock()
	return format
}

// textFormat returns a format of font, or nil.
func textFormat(font driver.Font) *comObject {
	f := dwriteFactory()
	if f == nil {
		return nil
	}
	var format *comObject
	fonts := fontCollection()
	if fonts != nil {
		// The format holds the collection itself.
		defer fonts.release()
	}
	if failed(f.callF(dwCreateTextFormat, 1<<5, uintptr(unsafe.Pointer(wide(fontName(font.Family)))), fonts.ptr(),
		fontWeight(font.Weight), fontStyle(font.Italic), dwStretchNormal, f32(max(float32(font.Size), 0.1)),
		uintptr(unsafe.Pointer(wide(font.Language))), uintptr(unsafe.Pointer(&format)))) {
		return nil
	}
	if fallback := fontFallback(font.Family); fallback != nil {
		// The format holds the fallback itself.
		if format1 := format.query(&iidDWriteTextFormat1); format1 != nil {
			format1.call(formatSetFontFallback, fallback.ptr())
			format1.release()
		}
		fallback.release()
	}
	if vals := axisValues(font); len(vals) > 0 {
		if format3 := format.query(&iidDWriteTextFormat3); format3 != nil {
			format3.call(formatSetFontAxisValues, uintptr(unsafe.Pointer(&vals[0])), uintptr(len(vals)))
			format3.release()
		}
	}
	return format
}

// inlineObject is an attachment's placeholder: an IDWriteInlineObject of
// the attachment's size that draws nothing, as its view is drawn as a
// child of the text's. DirectWrite holds it by reference counting.
type inlineObject struct {
	vtbl    *[7]uintptr
	refs    int32
	metrics inlineMetrics
}

var (
	inlineVtbl = [7]uintptr{
		syscall.NewCallback(inlineQueryInterface),
		syscall.NewCallback(inlineAddRef),
		syscall.NewCallback(inlineRelease),
		syscall.NewCallback(inlineDraw),
		syscall.NewCallback(inlineGetMetrics),
		syscall.NewCallback(inlineGetOverhang),
		syscall.NewCallback(inlineGetBreakConditions),
	}
	inlinesMu sync.Mutex
	inlines   = map[*inlineObject]struct{}{}
)

// newInline returns a placeholder holding one reference, the caller's.
func newInline(m inlineMetrics) *inlineObject {
	o := &inlineObject{vtbl: &inlineVtbl, refs: 1, metrics: m}
	inlinesMu.Lock()
	inlines[o] = struct{}{}
	inlinesMu.Unlock()
	return o
}

func (o *inlineObject) ptr() uintptr { return uintptr(unsafe.Pointer(o)) }

func inlineQueryInterface(o *inlineObject, iid *guid, out *unsafe.Pointer) uintptr {
	if *iid == iidUnknown || *iid == iidDWriteInline {
		*out = unsafe.Pointer(o)
		inlineAddRef(o)
		return sOK
	}
	*out = nil
	return eNoInterface
}

func inlineAddRef(o *inlineObject) uintptr {
	inlinesMu.Lock()
	defer inlinesMu.Unlock()
	o.refs++
	return uintptr(o.refs)
}

func inlineRelease(o *inlineObject) uintptr {
	inlinesMu.Lock()
	defer inlinesMu.Unlock()
	if o.refs--; o.refs == 0 {
		delete(inlines, o)
	}
	return uintptr(o.refs)
}

// inlineDraw draws nothing. Its float arguments are not where a Go
// callback reads them, but it reads none.
func inlineDraw(o *inlineObject, context, renderer, a, b, c, d, e uintptr) uintptr { return sOK }

func inlineGetMetrics(o *inlineObject, out *inlineMetrics) uintptr {
	*out = o.metrics
	return sOK
}

func inlineGetOverhang(o *inlineObject, out *overhangMetrics) uintptr {
	*out = overhangMetrics{}
	return sOK
}

func inlineGetBreakConditions(o *inlineObject, before, after *uint32) uintptr {
	*before, *after = 0, 0 // DWRITE_BREAK_CONDITION_NEUTRAL
	return sOK
}

// textRun is a range of the text in one style, in UTF-16 units.
type textRun struct {
	start, end uint32
	style      driver.TextStyle
	bg         driver.TextBackground
	hidden     bool
}

// textAttachment is an attachment's place and extents, in points.
type textAttachment struct {
	pos          uint32 // its UTF-16 offset
	size         driver.Size
	top          float32 // how far its top is above the baseline
	above, below float32
}

// preparedText is text shaped once, measured on the engine goroutine.
type preparedText struct {
	w           *window
	units       []uint16
	offsets     []int // each unit's UTF-8 offset, and the text's length last
	origUTF16   []int // each unit's original UTF-16 offset when word joiners were inserted, else nil
	keepAll     bool
	style       driver.TextStyle
	runs        []textRun
	attachments []textAttachment
	maxLines    int
	ellipsize   driver.EllipsizeMode
	shadow      driver.TextShadow
	hasHidden   bool
	// cut is the text, kept when it ellipsizes at its head or middle,
	// which DirectWrite's trimming cannot: Layout cuts it, its last line's
	// rest when it has several (rest).
	cut  *driver.AttributedText
	rest bool
	// step is the uniform line height, 0 for DirectWrite's own, and
	// baseline where the baseline is in each of its lines.
	step, baseline float32
	layout         *comObject // the engine's, for measuring
	inlines        []*inlineObject
}

// Prepare implements core.TextEngine on the engine goroutine.
func (w *window) Prepare(t *driver.AttributedText) driver.PreparedText {
	pt := &preparedText{w: w, style: t.Style, maxLines: t.MaxLines, ellipsize: t.Ellipsize, shadow: t.Shadow, hasHidden: t.HasHidden()}
	if t.Ellipsize == driver.EllipsizeHead || t.Ellipsize == driver.EllipsizeMiddle {
		switch {
		case t.MaxLines == 1 && !strings.ContainsAny(t.Text, "\n\r"):
			c := *t
			pt.cut = &c
		case t.MaxLines > 1:
			c := *t
			pt.cut, pt.rest = &c, true
		}
	}
	type rawRun struct {
		start, end int
		style      driver.TextStyle
		bg         driver.TextBackground
		hidden     bool
	}
	var rawRuns []rawRun
	var keepAllRanges [][2]int
	at := 0
	addRaw := func(start, end int, style driver.TextStyle, bg driver.TextBackground, hidden bool) {
		if end > start {
			rawRuns = append(rawRuns, rawRun{start, end, style, bg, hidden})
			if driver.KeepAllWords(style.Font.Language) {
				for _, r := range driver.KeepAllWordRanges(t.Text[start:end]) {
					keepAllRanges = append(keepAllRanges, [2]int{start + r[0], start + r[1]})
				}
			}
		}
	}
	for _, sp := range t.Spans {
		start, end := min(max(sp.Start, at), len(t.Text)), min(max(sp.End, at), len(t.Text))
		addRaw(at, start, t.Style, driver.TextBackground{}, false)
		addRaw(start, end, sp.Style, sp.Background, sp.Hidden)
		at = max(at, end)
	}
	addRaw(at, len(t.Text), t.Style, driver.TextBackground{}, false)
	pt.units, pt.offsets, pt.origUTF16 = encodeKeepAllText(t.Text, keepAllRanges)
	pt.keepAll = len(keepAllRanges) > 0
	// unit returns the UTF-16 offset of byte offset b.
	unit := func(b int) uint32 {
		lo, hi := 0, len(pt.offsets)-1
		for lo < hi {
			mid := (lo + hi) / 2
			if pt.offsets[mid] < b {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		return uint32(lo)
	}
	for _, r := range rawRuns {
		pt.runs = append(pt.runs, textRun{unit(r.start), unit(r.end), r.style, r.bg, r.hidden})
	}
	for _, a := range t.Attachments {
		if a.Offset < 0 || a.Offset >= len(t.Text) {
			continue
		}
		if r, _ := utf8.DecodeRuneInString(t.Text[a.Offset:]); r != driver.AttachmentRune {
			continue
		}
		font := pt.runAt(unit(a.Offset)).style.Font
		m := metricsOf(font)
		size := float32(font.Size)
		extent := driver.FontExtent{Ascent: driver.Points(m.ascent * size), Descent: driver.Points(m.descent * size),
			XHeight: driver.Points(m.xHeight * size)}
		above, below := a.Extent(extent)
		pt.attachments = append(pt.attachments, textAttachment{pos: unit(a.Offset), size: a.Size,
			top: float32(a.Ascent(extent)), above: float32(above), below: float32(below)})
	}
	if t.Style.LineHeight > 0 {
		m := metricsOf(t.Style.Font)
		size := float32(t.Style.Font.Size)
		pt.step = float32(t.Style.LineHeight)
		// The font centered in the line.
		pt.baseline = (pt.step-(m.ascent+m.descent)*size)/2 + m.ascent*size
	}
	for _, a := range pt.attachments {
		pt.inlines = append(pt.inlines, newInline(inlineMetrics{width: float32(a.size.Width),
			height: a.above + a.below, baseline: a.above}))
	}
	return pt
}

// runAt returns the run holding unit i.
func (pt *preparedText) runAt(i uint32) *textRun {
	for k := range pt.runs {
		if i < pt.runs[k].end {
			return &pt.runs[k]
		}
	}
	if n := len(pt.runs); n > 0 {
		return &pt.runs[n-1]
	}
	return &textRun{style: pt.style}
}

// build returns a layout of the text with no width limit, or nil.
func (pt *preparedText) build() *comObject {
	var wrap uint32
	if pt.maxLines == 1 {
		// A text limited to one line does not wrap: it is truncated.
		wrap = dwNoWrap
	} else if pt.keepAll {
		// Keep Korean words whole between spaces while breaking a single word that overflows the line.
		wrap = dwEmergencyBreak
	}
	base := pt.style.Font
	baseAxesKey := driver.FontAxesKey(base)
	key := formatKey{
		family:   base.Family,
		language: base.Language,
		weight:   base.Weight,
		italic:   base.Italic,
		axes:     baseAxesKey,
		size:     max(float32(base.Size), 0.1),
		align:    pt.style.Align,
		rtl:      pt.style.RTL,
		wrap:     wrap,
		step:     pt.step,
		baseline: pt.baseline,
		trim:     pt.maxLines > 0 && pt.ellipsize != driver.EllipsizeClip,
	}
	format := cachedTextFormat(key, base.Axes)
	if format == nil {
		return nil
	}
	defer format.release()
	units := pt.units
	if len(units) == 0 {
		units = []uint16{0}
	}
	var layout *comObject
	if failed(dwriteFactory().callF(dwCreateTextLayout, 0b11000, uintptr(unsafe.Pointer(&units[0])), uintptr(len(pt.units)),
		format.ptr(), f32(unbounded), f32(unboundedHeight), uintptr(unsafe.Pointer(&layout)))) {
		return nil
	}
	var spacing, layout4 *comObject
	baseSize := max(float32(base.Size), 0.1)
	for _, r := range pt.runs {
		n := r.end - r.start
		f := r.style.Font
		if f.Family != base.Family {
			name := wide(fontName(f.Family))
			layout.callRange(layoutSetFontFamilyName, 0, r.start, n, uintptr(unsafe.Pointer(name)))
		}
		if f.Weight != base.Weight {
			layout.callRange(layoutSetFontWeight, 0, r.start, n, fontWeight(f.Weight))
		}
		if f.Italic != base.Italic {
			layout.callRange(layoutSetFontStyle, 0, r.start, n, fontStyle(f.Italic))
		}
		if sz := max(float32(f.Size), 0.1); sz != baseSize {
			layout.callRange(layoutSetFontSize, 1, r.start, n, f32(sz))
		}
		if f.Language != base.Language {
			// The format has the text's own; a span in another language picks its glyphs and fallback by its own.
			locale := wide(f.Language)
			layout.callRange(layoutSetLocaleName, 0, r.start, n, uintptr(unsafe.Pointer(locale)))
		}
		if driver.FontAxesKey(f) != baseAxesKey {
			if layout4 == nil {
				layout4 = layout.query(&iidDWriteTextLayout4)
			}
			if layout4 != nil {
				vals := axisValues(f)
				var ptr uintptr
				if len(vals) > 0 {
					ptr = uintptr(unsafe.Pointer(&vals[0]))
				}
				layout4.callRange(layoutSetFontAxisValues, 0, r.start, n, ptr, uintptr(len(vals)))
			}
		}
		if ls := float32(r.style.LetterSpacing); ls != 0 {
			if spacing == nil {
				spacing = layout.query(&iidDWriteTextLayout1)
			}
			if spacing != nil {
				spacing.callRange(layoutSetCharacterSpacing, 0b111, r.start, n, f32(0), f32(ls), f32(0))
			}
		}
		if t := typography(f); t != nil {
			layout.callRange(layoutSetTypography, 0, r.start, n, t.ptr())
			t.release()
		}
	}
	spacing.release()
	layout4.release()
	for i, a := range pt.attachments {
		layout.callRange(layoutSetInlineObject, 0, a.pos, 1, pt.inlines[i].ptr())
	}
	return layout
}

// lineMetricsOf returns layout's lines.
func lineMetricsOf(layout *comObject) []lineMetrics {
	var count uint32
	layout.call(layoutGetLineMetrics, 0, 0, uintptr(unsafe.Pointer(&count)))
	if count == 0 {
		return nil
	}
	lines := make([]lineMetrics, count)
	layout.call(layoutGetLineMetrics, uintptr(unsafe.Pointer(&lines[0])), uintptr(count), uintptr(unsafe.Pointer(&count)))
	return lines[:min(int(count), len(lines))]
}

// lineBox is a line where it is drawn: its units, its top and baseline,
// and how far it moves down from where DirectWrite put it.
type lineBox struct {
	start, end        uint32 // end includes trailing spaces and the line break
	trailing, newline uint32
	top, height       float32
	baseline          float32
	shift             float32
	dwTop, dwHeight   float32
}

// stack places the lines of a layout.
func (pt *preparedText) stack(lines []lineMetrics) []lineBox {
	boxes := make([]lineBox, len(lines))
	var start uint32
	var top, dwTop float32
	k := 0 // the next attachment
	for i, l := range lines {
		b := lineBox{start: start, end: start + l.length, trailing: l.trailingWhitespace, newline: l.newline,
			top: top, height: l.height, baseline: top + l.baseline, dwTop: dwTop, dwHeight: l.height}
		if pt.step > 0 {
			above, below := pt.baseline, pt.step-pt.baseline
			for ; k < len(pt.attachments) && pt.attachments[k].pos < b.end; k++ {
				above = max(above, pt.attachments[k].above)
				below = max(below, pt.attachments[k].below)
			}
			b.height, b.baseline = above+below, top+above
			b.shift = b.baseline - (dwTop + l.baseline)
		}
		boxes[i] = b
		start, top, dwTop = b.end, top+b.height, dwTop+l.height
	}
	return boxes
}

// shown returns how many of n lines show.
func (pt *preparedText) shown(n int) int {
	if pt.maxLines > 0 {
		return min(n, pt.maxLines)
	}
	return n
}

// inkWidth returns how wide units from start to end are drawn in layout.
func inkWidth(layout *comObject, start, end uint32) (left, right float32) {
	left, right = float32(math.Inf(1)), float32(math.Inf(-1))
	if end <= start {
		return 0, 0
	}
	var buf [16]hitTestMetrics
	var count uint32
	layout.callF(layoutHitTestTextRange, 0b1100, uintptr(start), uintptr(end-start), f32(0), f32(0),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&count)))
	for _, m := range buf[:min(int(count), len(buf))] {
		left, right = min(left, m.left), max(right, m.left+m.width)
	}
	if right < left {
		return 0, 0
	}
	return left, right
}

// spanEdges returns where units from start to end begin and end on their
// line, trailing spaces included, which hit testing a range leaves out.
func spanEdges(layout *comObject, start, end uint32) (left, right float32) {
	if end <= start {
		return 0, 0
	}
	edge := func(pos uint32, trailing uintptr) float32 {
		var x, y float32
		var m hitTestMetrics
		layout.call(layoutHitTestTextPosition, uintptr(pos), trailing, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)),
			uintptr(unsafe.Pointer(&m)))
		return x
	}
	a, b := edge(start, 0), edge(end-1, 1)
	return min(a, b), max(a, b)
}

func (pt *preparedText) Measure(maxWidth driver.Points) driver.Size {
	if pt.layout == nil {
		pt.layout = pt.build()
		if pt.layout == nil {
			return driver.Size{}
		}
	}
	limit := float32(unbounded)
	if !maxWidth.IsAuto() {
		limit = max(float32(maxWidth), 0)
	}
	pt.layout.callF(layoutSetMaxHeight, 1, f32(unboundedHeight))
	pt.layout.callF(layoutSetMaxWidth, 1, f32(limit))
	raw := lineMetricsOf(pt.layout)
	boxes := pt.stack(raw)
	shown := pt.shown(len(boxes))
	var width, height float32
	for _, b := range boxes[:shown] {
		height += b.height
	}
	if shown < len(boxes) {
		var preWidth, dwHeight float32
		for _, b := range boxes[:shown] {
			l, r := spanEdges(pt.layout, b.start, b.end-b.newline)
			preWidth = max(preWidth, r-l)
			dwHeight += b.dwHeight
		}
		var trimmedWidth float32
		var trimmedLines uint32
		if pt.ellipsize != driver.EllipsizeClip {
			// Apply the same height limit Layout uses so GetMetrics reflects
			// DirectWrite's truncation before resetting the shared layout.
			pt.layout.callF(layoutSetMaxHeight, 1, f32(dwHeight+0.01))
			var m textMetrics
			pt.layout.call(layoutGetMetrics, uintptr(unsafe.Pointer(&m)))
			pt.layout.callF(layoutSetMaxHeight, 1, f32(unboundedHeight))
			trimmedWidth, trimmedLines = m.widthWithTrailing, m.lineCount
		}
		softWrap := shown > 0 && boxes[shown-1].newline == 0
		width = limitedMeasureWidth(limitedMeasureInput{
			preWidth:     preWidth,
			trimmedWidth: trimmedWidth,
			trimmedLines: int(trimmedLines),
			shown:        shown,
			limit:        limit,
			bounded:      limit < unbounded,
			softWrap:     softWrap,
			ellipsize:    pt.ellipsize,
			useCut:       pt.cut != nil,
		})
	} else {
		var m textMetrics
		pt.layout.call(layoutGetMetrics, uintptr(unsafe.Pointer(&m)))
		firstTrimmed := len(raw) > 0 && raw[0].trimmed != 0
		width = singlePassMeasureWidth(m.widthWithTrailing, limit, limit < unbounded, pt.cut != nil, firstTrimmed)
	}
	return driver.Size{Width: driver.Points(width), Height: driver.Points(height)}
}

// Layout lays the text out at size in a layout of its own, which the
// main thread draws and releases.
func (pt *preparedText) Layout(size driver.Size) driver.TextLayout {
	// The most characters that fit around the ellipsis, as a text of
	// their own.
	fits := func(c *driver.AttributedText) bool {
		p := pt.w.Prepare(c)
		defer p.Release()
		return p.Measure(driver.Auto).Width <= size.Width+0.01
	}
	var cut driver.AttributedText
	switch {
	case pt.cut == nil:
	case pt.rest:
		// The lines before the last as they wrap, the rest cut.
		whole := *pt.cut
		whole.MaxLines = 0
		p := pt.w.Prepare(&whole)
		l := p.Layout(size).(*textLayout)
		lines := l.TextLines()
		l.Release()
		p.Release()
		if len(lines) > pt.cut.MaxLines {
			cut = pt.cut.CutRest(lines[pt.cut.MaxLines-1].Start, fits)
		}
	case pt.Measure(driver.Auto).Width > size.Width:
		cut = pt.cut.CutToFit(fits)
	}
	if cut.Text != "" {
		p := pt.w.Prepare(&cut)
		defer p.Release()
		l := p.Layout(size).(*textLayout)
		// Attachments by the app's indices: those cut out are hidden.
		if len(pt.cut.Attachments) > 0 {
			boxes := make([]attachmentBox, len(pt.cut.Attachments))
			for k, o := range cut.Origin {
				if k < len(l.attachments) && o < len(boxes) {
					boxes[o] = l.attachments[k]
				}
			}
			l.attachments = boxes
		}
		return l
	}
	l := &textLayout{color: pt.style.Color, offsets: pt.offsets, origUTF16: pt.origUTF16, attachments: make([]attachmentBox, len(pt.attachments)),
		shadow: pt.shadow, hasHidden: pt.hasHidden, runs: pt.runs, style: pt.style}
	layout := pt.layout
	if layout != nil {
		pt.layout = nil
		layout.callF(layoutSetMaxHeight, 1, f32(unboundedHeight))
	} else {
		layout = pt.build()
	}
	if layout == nil {
		return l
	}
	l.layout = layout
	l.carets.Set(l.caretLines)
	// A little over the width, so that rounding does not wrap what fit.
	layout.callF(layoutSetMaxWidth, 1, f32(max(float32(size.Width), 0)+0.01))
	boxes := pt.stack(lineMetricsOf(layout))
	shown := pt.shown(len(boxes))
	if shown < len(boxes) {
		// DirectWrite trims what does not fit its height and ends the
		// last line that does with the ellipsis.
		var h float32
		for _, b := range boxes[:shown] {
			h += b.dwHeight
		}
		layout.callF(layoutSetMaxHeight, 1, f32(h+0.01))
	}
	l.lines = boxes[:shown]
	if pt.maxLines > 0 && pt.ellipsize == driver.EllipsizeClip && len(l.lines) > 0 {
		last := l.lines[len(l.lines)-1]
		l.clip = rectF{0, 0, float32(size.Width), last.top + last.height}
	}
	for i, b := range l.lines {
		if n := len(l.groups); n > 0 && l.groups[n-1].shift == b.shift {
			l.groups[n-1].to = i + 1
		} else {
			l.groups = append(l.groups, lineGroup{from: i, to: i + 1, shift: b.shift})
		}
	}
	for i, a := range pt.attachments {
		line := -1
		for k, b := range l.lines {
			if a.pos >= b.start && a.pos < b.end {
				line = k
			}
		}
		if line < 0 {
			continue
		}
		var x, y float32
		var m hitTestMetrics
		layout.call(layoutHitTestTextPosition, uintptr(a.pos), 0, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)),
			uintptr(unsafe.Pointer(&m)))
		if m.isTrimmed != 0 {
			continue
		}
		l.attachments[i] = attachmentBox{shown: true, box: driver.Bounds{
			Origin: driver.Point{X: driver.Points(x), Y: driver.Points(l.lines[line].baseline - a.top)}, Size: a.size}}
	}
	thin := float32(1 / pt.w.scale)
	for _, r := range pt.runs {
		if r.hidden {
			l.hidden = append(l.hidden, textRange{r.start, r.end - r.start})
			continue
		}
		if visible(r.style.Color) && r.style.Color != l.color {
			l.colors = append(l.colors, colorRange{r.start, r.end - r.start, r.style.Color})
		}
		bg := visible(r.bg.Color)
		deco := r.style.Decoration & (driver.TextDecorationUnderline | driver.TextDecorationLineThrough)
		lineColor := r.style.Color
		if r.style.DecorationColor != 0 {
			lineColor = r.style.DecorationColor
		}
		if !bg && deco == 0 {
			continue
		}
		m := metricsOf(r.style.Font)
		size := float32(r.style.Font.Size)
		for _, b := range l.lines {
			if bg {
				// Hanging spaces at the line's end are not painted.
				if x0, x1 := inkWidth(layout, max(r.start, b.start), min(r.end, b.end-b.trailing-b.newline)); x1 > x0 {
					top, bottom := r.bg.VerticalBounds(driver.Points((m.ascent + m.descent) * size))
					if bottom > top {
						y := b.baseline - m.ascent*size
						box := rectF{x0, y + float32(top), x1, y + float32(bottom)}
						radius := max(0, min(float32(r.bg.Radius), (box.bottom-box.top)/2))
						l.backgrounds = append(l.backgrounds, backgroundBox{box, r.bg.Color, radius})
					}
				}
			}
			if deco == 0 {
				continue
			}
			x0, x1 := spanEdges(layout, max(r.start, b.start), min(r.end, b.end-b.newline))
			if x1 <= x0 {
				continue
			}
			line := func(position, thickness float32) {
				t := max(thickness*size, thin)
				if dt := float32(r.style.DecorationThickness); dt > 0 {
					t = dt
				}
				y := b.baseline - position*size
				l.decorations = append(l.decorations, decoration{rectF{x0, y - t/2, x1, y + t/2}, lineColor, r.style.DecorationStyle})
			}
			if deco&driver.TextDecorationUnderline != 0 {
				position := m.underline
				if off, ok := r.style.UnderlineOffset.Points(); ok {
					// The app's offset puts the line's top that far below
					// the baseline.
					t := max(m.underlineThickness*size, thin)
					if dt := float32(r.style.DecorationThickness); dt > 0 {
						t = dt
					}
					position = -(float32(off) + t/2) / size
				}
				line(position, m.underlineThickness)
			}
			if deco&driver.TextDecorationLineThrough != 0 {
				line(m.strike, m.strikeThickness)
			}
		}
	}
	for _, h := range l.hidden {
		layout.callRange(layoutSetDrawingEffect, 0, h.start, h.length, hiddenEffectObj.ptr())
	}
	return l
}

func (pt *preparedText) Release() {
	releaseAll(&pt.layout)
	for _, o := range pt.inlines {
		inlineRelease(o)
	}
	pt.inlines = nil
}

// colorRange is a span's color, set as a drawing effect.
type colorRange struct {
	start, length uint32
	color         driver.Color
}

// backgroundBox is a span's background on one line, in the text's points.
type backgroundBox struct {
	r      rectF
	color  driver.Color
	radius float32
}

// decoration is an underline or a strikethrough on one line: its line's
// box, centered on it.
type decoration struct {
	r     rectF
	color driver.Color
	style driver.TextDecorationStyle
}

// draw paints d moved by x, y in its style: a solid line fills its box,
// the others are drawn along its middle.
func (d decoration) draw(p *painter, x, y float32) {
	r := rectF{d.r.left + x, d.r.top + y, d.r.right + x, d.r.bottom + y}
	t := r.bottom - r.top
	switch d.style {
	case driver.DecorationDouble:
		p.fillRect(rectF{r.left, r.top - t, r.right, r.bottom - t}, d.color)
		p.fillRect(rectF{r.left, r.top + t, r.right, r.bottom + t}, d.color)
	case driver.DecorationDotted, driver.DecorationDashed:
		on, step := t, 2*t
		if d.style == driver.DecorationDashed {
			on, step = 3*t, 5*t
		}
		for a := r.left; a < r.right; a += step {
			p.fillRect(rectF{a, r.top, min(a+on, r.right), r.bottom}, d.color)
		}
	case driver.DecorationWavy:
		// A band along a sine, a wavelength four thicknesses long.
		amp, wave := max(t, 1), 4*max(t, 1)
		mid := (r.top + r.bottom) / 2
		var top, bottom []pointF
		for a := r.left; ; a += wave / 8 {
			a = min(a, r.right)
			dy := amp * float32(math.Sin(float64(2*math.Pi*(a-r.left)/wave)))
			top = append(top, pointF{a, mid + dy - t/2})
			bottom = append(bottom, pointF{a, mid + dy + t/2})
			if a >= r.right {
				break
			}
		}
		for i := len(bottom) - 1; i >= 0; i-- {
			top = append(top, bottom[i])
		}
		if g := p.polygon(top); g != nil {
			p.fillGeometry(g, d.color)
			g.release()
		}
	default:
		p.fillRect(r, d.color)
	}
}

type attachmentBox struct {
	box   driver.Bounds // in the content box, in points
	shown bool
}

// lineGroup is lines drawn together, moved down by shift.
type lineGroup struct {
	from, to int
	shift    float32
}

// textLayout is text laid out at its final size, drawn on the main
// thread.
type textLayout struct {
	layout      *comObject
	color       driver.Color
	offsets     []int
	origUTF16   []int
	lines       []lineBox
	groups      []lineGroup
	colors      []colorRange
	hidden      []textRange
	hasHidden   bool
	backgrounds []backgroundBox
	decorations []decoration
	attachments []attachmentBox
	// runs and style are the text's, for the fonts of its lines.
	runs  []textRun
	style driver.TextStyle
	// clip, when not empty, cuts the lines a clipped text shows.
	clip rectF
	// shadow is drawn behind the glyphs: their outline recorded, then
	// shaded by an effect, in shadowGeneration.
	shadow                   driver.TextShadow
	shadowList, shadowEffect *comObject
	shadowImage              *comObject
	shadowGeneration         uint64
	// The brushes of colors, set as drawing effects in generation.
	brushes    []*comObject
	generation uint64
	// carets hit-tests the lines, read from the layout when first asked.
	carets driver.LazyCarets
}

// HitTest implements driver.TextHitTester.
func (l *textLayout) HitTest(p driver.Point) driver.Caret { return l.carets.HitTest(p) }

// CaretRect implements driver.TextHitTester.
func (l *textLayout) CaretRect(c driver.Caret) (driver.Bounds, bool) { return l.carets.CaretRect(c) }

// RangeRects implements driver.TextHitTester.
func (l *textLayout) RangeRects(start, end int, out []driver.Bounds) []driver.Bounds {
	return l.carets.RangeRects(start, end, out)
}

func (l *textLayout) orig(u uint32) int {
	if l.origUTF16 == nil {
		return int(u)
	}
	if int(u) >= len(l.origUTF16) {
		return l.origUTF16[len(l.origUTF16)-1]
	}
	return l.origUTF16[u]
}

// caretLines reads where the lines' clusters are from DirectWrite: each
// cluster's box and bidi level at its first unit, line by line, without
// the line's break, its y the line's box.
func (l *textLayout) caretLines() driver.CaretLayout {
	out := make(driver.CaretLayout, 0, len(l.lines))
	for _, b := range l.lines {
		cl := driver.CaretLine{Start: l.orig(b.start), End: l.orig(b.end),
			Box: driver.Bounds{Origin: driver.Point{Y: driver.Points(b.top)}, Size: driver.Size{Height: driver.Points(b.height)}}}
		left, right := spanEdges(l.layout, b.start, b.end-b.trailing-b.newline)
		cl.Box.Origin.X, cl.Box.Size.Width = driver.Points(left), driver.Points(right-left)
		for pos := b.start; pos < b.end-b.newline; {
			var x, y float32
			var m hitTestMetrics
			l.layout.call(layoutHitTestTextPosition, uintptr(pos), 0, uintptr(unsafe.Pointer(&x)), uintptr(unsafe.Pointer(&y)),
				uintptr(unsafe.Pointer(&m)))
			end := max(m.position+m.length, pos+1)
			if cs, ce := l.orig(pos), l.orig(end); ce > cs {
				cl.Clusters = append(cl.Clusters, driver.CaretCluster{Start: cs, End: ce,
					Left: driver.Points(m.left), Right: driver.Points(m.left + m.width), RTL: m.bidiLevel&1 != 0})
			}
			pos = end
		}
		cl.RTL = l.style.RTL
		out = append(out, cl)
	}
	return out
}

// Attachment implements core.AttachmentLayout.
func (l *textLayout) Attachment(i int) (driver.Bounds, bool) {
	if i < 0 || i >= len(l.attachments) {
		return driver.Bounds{}, false
	}
	return l.attachments[i].box, l.attachments[i].shown
}

// Release implements core.TextLayout on the main thread.
func (l *textLayout) Release() {
	l.carets.Drop()
	releaseAll(&l.layout, &l.shadowImage, &l.shadowEffect, &l.shadowList)
	for i := range l.brushes {
		releaseAll(&l.brushes[i])
	}
	l.brushes = nil
}

// effects sets the spans' colors as brushes of the current device.
func (l *textLayout) effects(p *painter) {
	if l.generation == gfxGeneration && len(l.brushes) == len(l.colors) {
		return
	}
	for i := range l.brushes {
		releaseAll(&l.brushes[i])
	}
	l.brushes = l.brushes[:0]
	for _, c := range l.colors {
		col := colorOf(c.color)
		var brush *comObject
		p.dc.call(rtCreateSolidColorBrush, uintptr(unsafe.Pointer(&col)), 0, uintptr(unsafe.Pointer(&brush)))
		l.brushes = append(l.brushes, brush)
		l.layout.callRange(layoutSetDrawingEffect, 0, c.start, c.length, brush.ptr())
	}
	l.generation = gfxGeneration
}

// inkBounds returns the area l paints in its view when placed at box:
// its lines, span backgrounds, decoration lines (with room for double
// and wavy lines) and text shadow.
func (l *textLayout) inkBounds(box driver.Bounds) rectF {
	x, y := float32(box.Origin.X), float32(box.Origin.Y)
	r := noArea
	for _, bg := range l.backgrounds {
		r = union(r, rectF{x + bg.r.left, y + bg.r.top, x + bg.r.right, y + bg.r.bottom})
	}
	drawn := noArea
	textBottom := y + float32(box.Size.Height)
	if len(l.lines) > 0 {
		last := l.lines[len(l.lines)-1]
		textBottom = max(textBottom, y+last.top+last.height)
	}
	textBox := rectF{x, y, x + float32(box.Size.Width), textBottom}
	if !textBox.empty() {
		drawn = union(drawn, textBox)
	}
	for _, d := range l.decorations {
		dr := rectF{x + d.r.left, y + d.r.top, x + d.r.right, y + d.r.bottom}
		t := dr.bottom - dr.top
		pad := float32(0)
		switch d.style {
		case driver.DecorationDouble:
			pad = t
		case driver.DecorationWavy:
			pad = max(t, 1)
		}
		drawn = union(drawn, rectF{dr.left, dr.top - pad, dr.right, dr.bottom + pad})
	}
	if visible(l.shadow.Color) && !textBox.empty() {
		// drawShadow blurs by standardDeviation = Radius/2; 2*Radius is 4 sigma.
		blur := 2 * max(float32(l.shadow.Radius), 0)
		dx, dy := float32(l.shadow.Offset.X), float32(l.shadow.Offset.Y)
		drawn = union(drawn, rectF{textBox.left + dx - blur, textBox.top + dy - blur,
			textBox.right + dx + blur, textBox.bottom + dy + blur})
	}
	if !l.clip.empty() && !drawn.empty() {
		drawn = drawn.intersect(rectF{x + l.clip.left, y + l.clip.top, x + l.clip.right, y + l.clip.bottom})
	}
	if !drawn.empty() {
		r = union(r, drawn)
	}
	return r
}

// drawBackgrounds paints the span boxes before the selection.
func (l *textLayout) drawBackgrounds(p *painter, x, y float32, tint *driver.Color) {
	if l.layout == nil {
		return
	}
	for _, b := range l.backgrounds {
		c := b.color
		if tint != nil {
			c = tint.Faded(float32(c&0xff) / 255)
		}
		p.fillRounded(rectF{b.r.left + x, b.r.top + y, b.r.right + x, b.r.bottom + y}, b.radius, c)
	}
}

// draw paints the glyphs and their decorations at x, y in the view.
func (l *textLayout) draw(p *painter, x, y float32) {
	if l.layout == nil {
		return
	}
	l.effects(p)
	if !l.clip.empty() {
		p.pushClipRect(rectF{l.clip.left + x, l.clip.top + y, l.clip.right + x, l.clip.bottom + y})
	}
	if l.shadow.Color != 0 {
		l.drawShadow(p, x, y)
	}
	for _, g := range l.groups {
		clip := len(l.groups) > 1
		if clip {
			first, last := l.lines[g.from], l.lines[g.to-1]
			p.pushClipRect(rectF{-unbounded, y + first.top, unbounded, y + last.top + last.height})
		}
		if l.hasHidden {
			ctx := textDrawContext{dc: p.dc, defaultBrush: p.solid(l.color), scale: float32(p.scale), colorFont: true}
			l.layout.callF(layoutDraw, 0b1100, uintptr(unsafe.Pointer(&ctx)), textRendererObj.ptr(), f32(x), f32(y+g.shift))
			runtime.KeepAlive(&ctx)
		} else {
			p.dc.callPoint(rtDrawTextLayout, x, y+g.shift, l.layout.ptr(), p.solid(l.color).ptr(), d2dDrawTextEnableColorFont)
		}
		if clip {
			p.pop()
		}
	}
	for _, d := range l.decorations {
		d.draw(p, x, y)
	}
	if !l.clip.empty() {
		p.pop()
	}
	runtime.KeepAlive(l)
}

// drawShadow draws the shadow of the glyphs: the layout recorded on the
// second context, shaded in the shadow's color by Direct2D's shadow
// effect, its blur a standard deviation half the radius, as Core
// Graphics blurs.
func (l *textLayout) drawShadow(p *painter, x, y float32) {
	if l.shadowImage == nil || l.shadowGeneration != gfxGeneration {
		releaseAll(&l.shadowImage, &l.shadowEffect, &l.shadowList)
		l.shadowGeneration = gfxGeneration
		bake := p.g.bake
		if failed(bake.call(dcCreateCommandList, uintptr(unsafe.Pointer(&l.shadowList)))) {
			return
		}
		bake.call(dcSetTarget, l.shadowList.ptr())
		bake.callF(rtSetDpi, 0b11, f32(96*p.scale), f32(96*p.scale))
		bake.call(rtBeginDraw)
		t := identity.d2d()
		bake.call(rtSetTransform, uintptr(unsafe.Pointer(&t)))
		col := colorOf(driver.RGB(0))
		var brush *comObject
		bake.call(rtCreateSolidColorBrush, uintptr(unsafe.Pointer(&col)), 0, uintptr(unsafe.Pointer(&brush)))
		if brush != nil {
			for _, g := range l.groups {
				if l.hasHidden {
					ctx := textDrawContext{dc: bake, defaultBrush: brush, scale: float32(p.scale), shadow: true}
					l.layout.callF(layoutDraw, 0b1100, uintptr(unsafe.Pointer(&ctx)), textRendererObj.ptr(), f32(0), f32(g.shift))
					runtime.KeepAlive(&ctx)
				} else {
					bake.callPoint(rtDrawTextLayout, 0, g.shift, l.layout.ptr(), brush.ptr(), d2dDrawTextEnableColorFont)
				}
			}
			brush.release()
		}
		hr := bake.call(rtEndDraw, 0, 0)
		bake.call(dcSetTarget, 0)
		if failed(hr) || failed(l.shadowList.call(commandListClose)) ||
			failed(p.dc.call(dcCreateEffect, uintptr(unsafe.Pointer(&clsidShadow)), uintptr(unsafe.Pointer(&l.shadowEffect)))) {
			releaseAll(&l.shadowEffect, &l.shadowList)
			return
		}
		const standardDeviation, color = 0, 1
		sigma := max(float32(l.shadow.Radius)/2, 0)
		c := colorOf(l.shadow.Color)
		l.shadowEffect.call(effectSetValue, standardDeviation, d2dPropertyFloat, uintptr(unsafe.Pointer(&sigma)), 4)
		l.shadowEffect.call(effectSetValue, color, d2dPropertyVector4, uintptr(unsafe.Pointer(&c)), 16)
		l.shadowEffect.call(effectSetInput, 0, l.shadowList.ptr(), 1)
		l.shadowEffect.call(effectGetOutput, uintptr(unsafe.Pointer(&l.shadowImage)))
		if l.shadowImage == nil {
			return
		}
	}
	at := pointF{x + float32(l.shadow.Offset.X), y + float32(l.shadow.Offset.Y)}
	p.dc.call(dcDrawImage, l.shadowImage.ptr(), uintptr(unsafe.Pointer(&at)), 0, d2dInterpolationLinear, 0)
}

// fonts returns the metrics and size of the font unit i is set in.
func (l *textLayout) fonts(i uint32) (faceMetrics, float32) {
	f := l.style.Font
	for _, r := range l.runs {
		if i >= r.start && i < r.end {
			f = r.style.Font
			break
		}
	}
	return metricsOf(f), float32(f.Size)
}

// TextLines implements core.LineLayout: each line's units, box and the
// metrics of the font it starts in.
func (l *textLayout) TextLines() []driver.TextLine {
	out := make([]driver.TextLine, 0, len(l.lines))
	end := len(l.offsets) - 1
	for _, b := range l.lines {
		left, right := float32(0), float32(0)
		if l.layout != nil {
			left, right = spanEdges(l.layout, b.start, b.end-b.trailing-b.newline)
		}
		m, size := l.fonts(b.start)
		out = append(out, driver.TextLine{
			Start: l.offsets[min(int(b.start), end)], End: l.offsets[min(int(b.end), end)],
			Bounds: driver.Bounds{Origin: driver.Point{X: driver.Points(left), Y: driver.Points(b.top)},
				Size: driver.Size{Width: driver.Points(right - left), Height: driver.Points(b.height)}},
			Ascender: driver.Points(b.baseline - b.top), Descender: driver.Points(b.top + b.height - b.baseline),
			CapHeight: driver.Points(m.capHeight * size), XHeight: driver.Points(m.xHeight * size),
		})
	}
	return out
}

// indexAt returns the UTF-8 offset of the character at x, y, in points
// from the content's origin, or -1.
func (l *textLayout) indexAt(x, y float32) int {
	if l.layout == nil {
		return -1
	}
	for _, b := range l.lines {
		if y < b.top || y >= b.top+b.height {
			continue
		}
		var trailing, inside int32
		var m hitTestMetrics
		l.layout.callF(layoutHitTestPoint, 0b11, f32(x), f32(y-b.shift), uintptr(unsafe.Pointer(&trailing)),
			uintptr(unsafe.Pointer(&inside)), uintptr(unsafe.Pointer(&m)))
		if inside == 0 || int(m.position) >= len(l.offsets) {
			return -1
		}
		return l.offsets[m.position]
	}
	return -1
}
