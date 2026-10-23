//go:build darwin

package apple

import (
	"math"
	"structs"
	"unicode/utf8"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/oneo-me/nsgo/core"
)

var (
	cfAttributedStringCreate    func(alloc, str, attrs uintptr) uintptr
	cfAttributedStringLength    func(uintptr) int
	cfAttributedStringSubstr    func(alloc, str uintptr, r cfRange) uintptr
	cfAttributedStringAttrsAt   func(str uintptr, loc int, effective *cfRange) uintptr
	cfRetain                    func(uintptr) uintptr
	cfArrayGetCount             func(uintptr) int
	cfArrayGetValueAtIndex      func(arr uintptr, i int) uintptr
	cgPathCreateWithRect        func(rect CGRect, transform uintptr) uintptr
	cgPathRelease               func(uintptr)
	cgContextSaveGState         func(uintptr)
	cgContextRestoreGState      func(uintptr)
	cgContextTranslateCTM       func(ctx uintptr, x, y float64)
	cgContextScaleCTM           func(ctx uintptr, x, y float64)
	cgContextSetTextPosition    func(ctx uintptr, x, y float64)
	ctFontCreateUIFont          func(kind uint32, size float64, lang uintptr) uintptr
	ctFontCreateWithName        func(name uintptr, size float64, matrix uintptr) uintptr
	ctFontCopyFontDescriptor    func(font uintptr) uintptr
	ctFontDescriptorCopyAttrs   func(desc, attrs uintptr) uintptr
	ctFontCreateWithDescriptor  func(desc uintptr, size float64, matrix uintptr) uintptr
	ctFontGetAscent             func(uintptr) float64
	ctFontGetDescent            func(uintptr) float64
	ctFontGetLeading            func(uintptr) float64
	ctParagraphStyleCreate      func(settings unsafe.Pointer, n uintptr) uintptr
	ctFramesetterCreate         func(str uintptr) uintptr
	ctFramesetterSuggestSize    func(fs uintptr, r cfRange, attrs uintptr, limit CGSize, fit uintptr) CGSize
	ctFramesetterCreateFrame    func(fs uintptr, r cfRange, path, attrs uintptr) uintptr
	ctFrameGetLines             func(frame uintptr) uintptr
	ctFrameGetLineOrigins       func(frame uintptr, r cfRange, origins *CGPoint)
	ctFrameDraw                 func(frame, ctx uintptr)
	ctLineGetTypographicBounds  func(line uintptr, ascent, descent, leading *float64) float64
	ctLineGetStringRange        func(line uintptr) cfRange
	ctLineCreateWithAttrString  func(str uintptr) uintptr
	ctLineCreateTruncatedLine   func(line uintptr, width float64, kind uint32, token uintptr) uintptr
	ctLineDraw                  func(line, ctx uintptr)
	ctLineIndexForPosition      func(line uintptr, p CGPoint) int
	ctLineOffsetForIndex        func(line uintptr, i int, secondary *float64) float64
	cfAttributedStringMutable   func(alloc uintptr, max int, str uintptr) uintptr
	cfAttributedStringSetAttrs  func(str uintptr, r cfRange, attrs uintptr, clear bool)
	ctLineGetGlyphRuns          func(line uintptr) uintptr
	ctRunGetAttributes          func(run uintptr) uintptr
	ctFontGetCapHeight          func(uintptr) float64
	cgContextSetShadowWithColor func(ctx uintptr, offset CGSize, blur float64, color uintptr)
	cgContextClipToRect         func(ctx uintptr, r CGRect)

	attrFont, attrColor, attrKern, attrUnderline, attrParagraph uintptr
	attrTraits, attrWeight, attrSymbolic, attrUnderlineColor    uintptr
)

func loadText(cf, cg uintptr) {
	ct := load("/System/Library/Frameworks/CoreText.framework/CoreText")
	reg := purego.RegisterLibFunc
	reg(&cfAttributedStringCreate, cf, "CFAttributedStringCreate")
	reg(&cfAttributedStringLength, cf, "CFAttributedStringGetLength")
	reg(&cfAttributedStringSubstr, cf, "CFAttributedStringCreateWithSubstring")
	reg(&cfAttributedStringAttrsAt, cf, "CFAttributedStringGetAttributes")
	reg(&cfRetain, cf, "CFRetain")
	reg(&cfArrayGetCount, cf, "CFArrayGetCount")
	reg(&cfArrayGetValueAtIndex, cf, "CFArrayGetValueAtIndex")
	reg(&cgPathCreateWithRect, cg, "CGPathCreateWithRect")
	reg(&cgPathRelease, cg, "CGPathRelease")
	reg(&cgContextSaveGState, cg, "CGContextSaveGState")
	reg(&cgContextRestoreGState, cg, "CGContextRestoreGState")
	reg(&cgContextTranslateCTM, cg, "CGContextTranslateCTM")
	reg(&cgContextScaleCTM, cg, "CGContextScaleCTM")
	reg(&cgContextSetTextPosition, cg, "CGContextSetTextPosition")
	reg(&ctFontCreateUIFont, ct, "CTFontCreateUIFontForLanguage")
	reg(&ctFontCreateWithName, ct, "CTFontCreateWithName")
	reg(&ctFontCopyFontDescriptor, ct, "CTFontCopyFontDescriptor")
	reg(&ctFontDescriptorCopyAttrs, ct, "CTFontDescriptorCreateCopyWithAttributes")
	reg(&ctFontCreateWithDescriptor, ct, "CTFontCreateWithFontDescriptor")
	reg(&ctFontGetAscent, ct, "CTFontGetAscent")
	reg(&ctFontGetDescent, ct, "CTFontGetDescent")
	reg(&ctFontGetLeading, ct, "CTFontGetLeading")
	reg(&ctParagraphStyleCreate, ct, "CTParagraphStyleCreate")
	reg(&ctFramesetterCreate, ct, "CTFramesetterCreateWithAttributedString")
	reg(&ctFramesetterSuggestSize, ct, "CTFramesetterSuggestFrameSizeWithConstraints")
	reg(&ctFramesetterCreateFrame, ct, "CTFramesetterCreateFrame")
	reg(&ctFrameGetLines, ct, "CTFrameGetLines")
	reg(&ctFrameGetLineOrigins, ct, "CTFrameGetLineOrigins")
	reg(&ctFrameDraw, ct, "CTFrameDraw")
	reg(&ctLineGetTypographicBounds, ct, "CTLineGetTypographicBounds")
	reg(&ctLineGetStringRange, ct, "CTLineGetStringRange")
	reg(&ctLineCreateWithAttrString, ct, "CTLineCreateWithAttributedString")
	reg(&ctLineCreateTruncatedLine, ct, "CTLineCreateTruncatedLine")
	reg(&ctLineDraw, ct, "CTLineDraw")
	reg(&ctLineIndexForPosition, ct, "CTLineGetStringIndexForPosition")
	reg(&ctLineOffsetForIndex, ct, "CTLineGetOffsetForStringIndex")
	reg(&cfAttributedStringMutable, cf, "CFAttributedStringCreateMutableCopy")
	reg(&cfAttributedStringSetAttrs, cf, "CFAttributedStringSetAttributes")
	reg(&ctLineGetGlyphRuns, ct, "CTLineGetGlyphRuns")
	reg(&ctRunGetAttributes, ct, "CTRunGetAttributes")
	reg(&ctFontGetCapHeight, ct, "CTFontGetCapHeight")
	reg(&cgContextSetShadowWithColor, cg, "CGContextSetShadowWithColor")
	reg(&cgContextClipToRect, cg, "CGContextClipToRect")
	attrFont = constant(ct, "kCTFontAttributeName")
	attrColor = constant(ct, "kCTForegroundColorAttributeName")
	attrKern = constant(ct, "kCTKernAttributeName")
	attrUnderline = constant(ct, "kCTUnderlineStyleAttributeName")
	attrParagraph = constant(ct, "kCTParagraphStyleAttributeName")
	attrTraits = constant(ct, "kCTFontTraitsAttribute")
	attrWeight = constant(ct, "kCTFontWeightTrait")
	attrSymbolic = constant(ct, "kCTFontSymbolicTrait")
	attrUnderlineColor = constant(ct, "kCTUnderlineColorAttributeName")
	loadInline(ct, cg)
}

// CoreText weights of CSS's 100 to 900, from UIFontWeight.
var weights = [...]float64{-0.8, -0.6, -0.4, 0, 0.23, 0.3, 0.4, 0.56, 0.62}

// fontKey identifies a font by the four things that choose one. A table
// of numbers has two or three of these for the whole screen, and asks for
// them once per changed cell per frame.
type fontKey struct {
	family string
	size   float64
	weight core.FontWeight
	italic bool
}

// fontCacheLimit bounds the cache. A screen uses a handful of fonts; an
// app that animates its font size would otherwise add one per frame.
const fontCacheLimit = 64

var fontCache = newRefCache[fontKey](fontCacheLimit)

// Font returns a CTFont (an NSFont or UIFont) for f: the system face
// unless f names a family. The caller releases it.
//
// Fonts are immutable and few, so they are built once and handed out
// again by reference. Building one is seven CoreText calls through purego;
// a frame of a table asks for the same font forty times.
func Font(f core.Font) uintptr {
	k := fontKey{family: f.Family, size: float64(f.Size), weight: f.Weight, italic: f.Italic}
	if ref := fontCache.get(k); ref != 0 {
		return ref
	}
	font := buildFont(f)
	if font == 0 {
		return 0
	}
	return fontCache.put(k, font)
}

// buildFont returns a new CTFont for f. The caller releases it.
func buildFont(f core.Font) uintptr {
	var base uintptr
	if f.Family == core.FontMonospace {
		const uiFontUserFixedPitch = 1
		base = ctFontCreateUIFont(uiFontUserFixedPitch, float64(f.Size), 0)
	} else if f.Family != "" {
		name := CFString(f.Family)
		base = ctFontCreateWithName(name, float64(f.Size), 0)
		Release(name)
	} else {
		const uiFontSystem = 2
		base = ctFontCreateUIFont(uiFontSystem, float64(f.Size), 0)
	}
	if f.Weight == core.FontWeightNormal && !f.Italic {
		return base
	}
	i := min(max(int(f.Weight+50)/100, 1), 9) - 1
	weight := float64Number(weights[i])
	var symbolic int32
	if f.Italic {
		const italicTrait = 1 << 0
		symbolic = italicTrait
	}
	sym := int32Number(symbolic)
	traits := dictionary([]uintptr{attrWeight, attrSymbolic}, []uintptr{weight, sym})
	attrs := dictionary([]uintptr{attrTraits}, []uintptr{traits})
	desc := ctFontCopyFontDescriptor(base)
	styled := ctFontDescriptorCopyAttrs(desc, attrs)
	font := ctFontCreateWithDescriptor(styled, float64(f.Size), 0)
	for _, ref := range []uintptr{weight, sym, traits, attrs, desc, styled, base} {
		Release(ref)
	}
	return font
}

type paragraphSetting struct {
	_     structs.HostLayout
	spec  uint32
	size  uintptr
	value unsafe.Pointer
}

// CoreText alignments and paragraph specifiers.
const (
	alignLeft, alignRight, alignCenter = 0, 1, 2
	alignJustified                     = 3
	specAlignment                      = 0
	specMaxLineHeight                  = 8
	specMinLineHeight                  = 9
)

// CoreText's underline styles and patterns.
const (
	underlineSingle = 0x01
	underlineDouble = 0x09
	underlineDot    = 0x0100
	underlineDash   = 0x0200
)

// runAttributes returns a new dictionary of what st sets per character:
// font, color, letter spacing and underline, plus extra keys and values.
// A wavy underline is drawn from the lines instead. The caller releases
// it.
func runAttributes(st core.TextStyle, keys, values []uintptr) uintptr {
	font := Font(st.Font)
	color := CGColor(st.Color)
	var line int32
	if st.Decoration&core.TextDecorationUnderline != 0 {
		switch st.DecorationStyle {
		case core.DecorationDouble:
			line = underlineDouble
		case core.DecorationDotted:
			line = underlineSingle | underlineDot
		case core.DecorationDashed:
			line = underlineSingle | underlineDash
		case core.DecorationWavy:
		default:
			line = underlineSingle
		}
	}
	underline := int32Number(line)
	owned := []uintptr{font, color, underline}
	keys = append(keys, attrFont, attrColor, attrUnderline)
	values = append(values, font, color, underline)
	if line != 0 && st.DecorationColor != 0 {
		c := CGColor(st.DecorationColor)
		keys, values, owned = append(keys, attrUnderlineColor), append(values, c), append(owned, c)
	}
	// A kern of 0 would turn the font's own kerning off.
	if st.LetterSpacing != 0 {
		kern := float64Number(float64(st.LetterSpacing))
		keys, values, owned = append(keys, attrKern), append(values, kern), append(owned, kern)
	}
	attrs := dictionary(keys, values)
	for _, ref := range owned {
		Release(ref)
	}
	return attrs
}

// utf16Offsets maps text's UTF-8 byte offsets to UTF-16 offsets, and
// back: toUTF16 has an entry per byte and one past the end, toUTF8 one
// per UTF-16 unit and one past the end.
func utf16Offsets(text string) (toUTF16, toUTF8 []int) {
	toUTF16 = make([]int, len(text)+1)
	n := 0
	for i, r := range text {
		size := utf8.RuneLen(r)
		if size < 0 {
			size = 1
		}
		for j := range size {
			toUTF16[i+j] = n
		}
		units := 1
		if r >= 0x10000 {
			units = 2
		}
		for range units {
			toUTF8 = append(toUTF8, i)
		}
		n += units
	}
	toUTF16[len(text)] = n
	toUTF8 = append(toUTF8, len(text))
	return toUTF16, toUTF8
}

// attrsKey is everything buildAttributes reads: the style, whether the
// text has attachments (a line holding one may grow past its line
// height), and whether it is a whole text, which alone takes the
// paragraph style. All are small and fixed, so all belong in the key.
type attrsKey struct {
	st              core.TextStyle
	withAttachments bool
	paragraph       bool
}

// attrsCacheLimit bounds the cache. A screen has a handful of styles.
const attrsCacheLimit = 256

var attrsCache = newRefCache[attrsKey](attrsCacheLimit)

// textAttributes returns a new CFDictionary of a whole text's attributes:
// what st sets per character and the paragraph style its alignment and
// line height ask for. The caller releases it.
//
// The attributes depend on the style, never on the text — except that a
// line holding an attachment may grow past its line height, which the
// paragraph style says. A screen builds a handful and asks for them again
// for every cell of every frame: without the cache each ask rebuilt a
// font, a colour, four CFNumbers, a paragraph style and the dictionary,
// all through purego.
func textAttributes(st core.TextStyle, withAttachments bool) uintptr {
	return attributes(attrsKey{st: st, withAttachments: withAttachments, paragraph: true})
}

// spanAttributes returns a new CFDictionary of what a span sets per
// character. The paragraph style belongs to the whole text, as NSH sets
// it, so a span's own alignment or line height changes nothing. The
// caller releases it.
func spanAttributes(st core.TextStyle) uintptr {
	return attributes(attrsKey{st: st})
}

// attributes returns the cached dictionary for k, building it on a miss.
func attributes(k attrsKey) uintptr {
	if d := attrsCache.get(k); d != 0 {
		return d
	}
	d := buildAttributes(k)
	if d == 0 {
		return 0
	}
	return attrsCache.put(k, d)
}

// buildAttributes returns a new dictionary of k's attributes: the font,
// colour, letter spacing and underline from runAttributes, plus, for a
// whole text, the paragraph style its alignment and line height ask for.
// The caller releases it.
func buildAttributes(k attrsKey) uintptr {
	var keys, values, owned []uintptr
	if k.paragraph {
		align := uint8(alignLeft)
		switch k.st.Align {
		case core.TextAlignCenter:
			align = alignCenter
		case core.TextAlignEnd:
			align = alignRight
		case core.TextAlignJustify:
			align = alignJustified
		}
		lineHeight := float64(k.st.LineHeight)
		settings := []paragraphSetting{{spec: specAlignment, size: 1, value: unsafe.Pointer(&align)}}
		if lineHeight > 0 {
			settings = append(settings, paragraphSetting{spec: specMinLineHeight, size: 8, value: unsafe.Pointer(&lineHeight)})
			// A line grows where an attachment reaches past it.
			if !k.withAttachments {
				settings = append(settings, paragraphSetting{spec: specMaxLineHeight, size: 8, value: unsafe.Pointer(&lineHeight)})
			}
		}
		para := ctParagraphStyleCreate(unsafe.Pointer(&settings[0]), uintptr(len(settings)))
		keys, values, owned = append(keys, attrParagraph), append(values, para), append(owned, para)
	}
	attrs := runAttributes(k.st, keys, values)
	for _, ref := range owned {
		Release(ref)
	}
	return attrs
}

// attributed returns a new CFAttributedString of t, its spans styled over
// its style. The caller releases it.
func attributed(t *core.AttributedText, toUTF16 []int, fonts []core.FontExtent) uintptr {
	attrs := textAttributes(t.Style, len(t.Attachments) > 0)
	str := CFString(t.Text)
	out := cfAttributedStringCreate(0, str, attrs)
	Release(str)
	Release(attrs)
	if len(t.Spans) == 0 && len(t.Attachments) == 0 {
		return out
	}
	m := cfAttributedStringMutable(0, 0, out)
	Release(out)
	for _, sp := range t.Spans {
		start, end := min(max(sp.Start, 0), len(t.Text)), min(max(sp.End, 0), len(t.Text))
		if end <= start {
			continue
		}
		a := spanAttributes(sp.Style)
		r := cfRange{Location: toUTF16[start], Length: toUTF16[end] - toUTF16[start]}
		cfAttributedStringSetAttrs(m, r, a, false)
		Release(a)
	}
	// An attachment's placeholder is a run of its size that draws nothing.
	for i, at := range t.Attachments {
		if at.Offset < 0 || at.Offset >= len(t.Text) {
			continue
		}
		above, below := at.Extent(fonts[i])
		d := runDelegate(extent{above: float64(above), below: float64(below), width: float64(at.Size.Width)})
		st := placeholderStyle(t, at.Offset)
		st.Color = 0
		a := runAttributes(st, []uintptr{attrRunDelegate}, []uintptr{d})
		r := cfRange{Location: toUTF16[at.Offset], Length: toUTF16[at.Offset+len(string(core.AttachmentRune))] - toUTF16[at.Offset]}
		cfAttributedStringSetAttrs(m, r, a, false)
		Release(a)
		Release(d)
	}
	return m
}

// placeholderStyle is the style of the span at byte offset i: the font an
// attachment there is placed against.
func placeholderStyle(t *core.AttributedText, i int) core.TextStyle {
	for _, sp := range t.Spans {
		if sp.Start <= i && i < sp.End {
			return sp.Style
		}
	}
	return t.Style
}

// TextEngine implements core.TextEngine with CoreText. Strikethrough and
// span backgrounds, which CoreText has no attributes for, are drawn from
// the lines' offsets; attachments' placeholders are sized by run
// delegates.
type TextEngine struct{}

// Prepare implements core.TextEngine. It is safe off the main thread.
func (TextEngine) Prepare(t *core.AttributedText) core.PreparedText {
	toUTF16, toUTF8 := utf16Offsets(t.Text)
	fonts := make([]core.FontExtent, len(t.Attachments))
	for i, at := range t.Attachments {
		fonts[i] = fontExtent(placeholderStyle(t, at.Offset))
	}
	str := attributed(t, toUTF16, fonts)
	p := &prepared{str: str, fs: ctFramesetterCreate(str), maxLines: t.MaxLines, ellipsize: t.Ellipsize,
		shadow: t.Shadow, text: t.Text, toUTF8: toUTF8,
		toUTF16: toUTF16, decos: decorations(t, toUTF16), atts: t.Attachments, fonts: fonts}
	if cfAttributedStringLength(str) == 0 {
		f := Font(t.Style.Font)
		p.emptyHeight = ctFontGetAscent(f) + ctFontGetDescent(f) + ctFontGetLeading(f)
		if t.Style.LineHeight > 0 {
			p.emptyHeight = float64(t.Style.LineHeight)
		}
		Release(f)
	}
	return p
}

type prepared struct {
	str, fs     uintptr
	maxLines    int
	ellipsize   core.EllipsizeMode
	shadow      core.TextShadow
	emptyHeight float64
	text        string
	toUTF8      []int // byte offsets of UTF-16 offsets
	toUTF16     []int
	decos       []decoration
	atts        []core.TextAttachment
	fonts       []core.FontExtent // the attachments'
	// measured caches Measure by wrap width: layout asks the same prepared
	// text its size at the width it tried before, two or three times per
	// frame for a table of numbers. Each entry costs one CTFramesetter call
	// through purego, so the answer is kept per width.
	measured map[measureKey]core.Size
}

// measureKey is a wrap width. Auto is a NaN, which no map can key on, so
// it gets its own field.
type measureKey struct {
	auto  bool
	width float32
}

func keyOf(maxWidth core.Points) measureKey {
	if maxWidth.IsAuto() {
		return measureKey{auto: true}
	}
	return measureKey{width: float32(maxWidth)}
}

// unbounded is a size limit nothing reaches; tall is a frame height
// whose line origins keep their precision.
const (
	unbounded = math.MaxFloat32
	tall      = 1e7
)

func (p *prepared) Measure(maxWidth core.Points) core.Size {
	k := keyOf(maxWidth)
	if size, ok := p.measured[k]; ok {
		return size
	}
	size := p.measure(maxWidth)
	if p.measured == nil {
		p.measured = make(map[measureKey]core.Size, 2)
	}
	p.measured[k] = size
	return size
}

// measure is Measure without the cache.
func (p *prepared) measure(maxWidth core.Points) core.Size {
	if p.emptyHeight > 0 {
		return core.Size{Height: core.Points(math.Ceil(p.emptyHeight))}
	}
	w := float64(unbounded)
	if !maxWidth.IsAuto() {
		w = float64(maxWidth)
	}
	if p.maxLines > 0 {
		frame := p.frame(CGSize{Width: w, Height: tall})
		defer Release(frame)
		if lines := ctFrameGetLines(frame); cfArrayGetCount(lines) > p.maxLines {
			size := firstLines(frame, lines, p.maxLines, tall)
			return core.Size{Width: core.Points(math.Ceil(size.Width)), Height: core.Points(math.Ceil(size.Height))}
		}
	}
	size := ctFramesetterSuggestSize(p.fs, cfRange{}, 0, CGSize{Width: w, Height: unbounded}, 0)
	return core.Size{Width: core.Points(math.Ceil(size.Width)), Height: core.Points(math.Ceil(size.Height))}
}

// firstLines returns the size of a frame's first n lines.
func firstLines(frame, lines uintptr, n int, frameHeight float64) CGSize {
	origins := make([]CGPoint, n)
	ctFrameGetLineOrigins(frame, cfRange{Length: n}, &origins[0])
	var width, descent float64
	for i := range n {
		var a, d, l float64
		width = max(width, ctLineGetTypographicBounds(cfArrayGetValueAtIndex(lines, i), &a, &d, &l))
		descent = d
	}
	return CGSize{Width: width, Height: frameHeight - (origins[n-1].Y - descent)}
}

func (p *prepared) frame(size CGSize) uintptr {
	path := cgPathCreateWithRect(Rect(0, 0, size.Width, size.Height), 0)
	defer cgPathRelease(path)
	return ctFramesetterCreateFrame(p.fs, cfRange{}, path, 0)
}

// Layout implements core.PreparedText: the CTFrame the main thread draws.
func (p *prepared) Layout(size core.Size) core.TextLayout {
	l := p.layout(size)
	if len(p.decos) > 0 || len(p.atts) > 0 {
		l.place(p.decos, p.atts, p.fonts, p.toUTF16)
	}
	return l
}

func (p *prepared) layout(size core.Size) *Layout {
	l := &Layout{height: float64(size.Height), text: p.text, toUTF8: p.toUTF8, shadow: p.shadow}
	if p.emptyHeight > 0 {
		return l
	}
	// A pixel of slack keeps the last line, which rounding could drop.
	l.frame = p.frame(CGSize{Width: float64(size.Width) + 0.5, Height: float64(size.Height) + 1})
	if p.maxLines <= 0 {
		return l
	}
	all := p.frame(CGSize{Width: float64(size.Width) + 0.5, Height: tall})
	defer Release(all)
	lines := ctFrameGetLines(all)
	if cfArrayGetCount(lines) <= p.maxLines {
		return l
	}
	// The last line shown takes the rest of the text, truncated.
	r := ctLineGetStringRange(cfArrayGetValueAtIndex(lines, p.maxLines-1))
	rest := cfAttributedStringSubstr(0, p.str, cfRange{Location: r.Location, Length: cfAttributedStringLength(p.str) - r.Location})
	line := ctLineCreateWithAttrString(rest)
	if p.ellipsize == core.EllipsizeClip {
		// The rest on one line, cut where the box ends.
		l.truncated, l.clip = line, float64(size.Width)
	} else {
		kind := uint32(truncateEnd)
		switch p.ellipsize {
		case core.EllipsizeHead:
			kind = truncateStart
		case core.EllipsizeMiddle:
			kind = truncateMiddle
		}
		token := ellipsisToken(rest)
		l.truncated = ctLineCreateTruncatedLine(line, float64(size.Width), kind, token)
		Release(token)
		Release(line)
	}
	Release(rest)
	l.shown, l.restStart = p.maxLines, r.Location
	origins := make([]CGPoint, p.maxLines)
	ctFrameGetLineOrigins(all, cfRange{Length: p.maxLines}, &origins[0])
	// Origins count from the frame's bottom; keep them from its top.
	l.origins = make([]CGPoint, p.maxLines)
	for i, o := range origins {
		l.origins[i] = CGPoint{X: o.X, Y: tall - o.Y}
	}
	l.lines = make([]uintptr, p.maxLines-1)
	for i := range l.lines {
		l.lines[i] = cfRetain(cfArrayGetValueAtIndex(lines, i))
	}
	Release(l.frame)
	l.frame = 0
	return l
}

// CTLineTruncationType.
const (
	truncateStart  = 0
	truncateEnd    = 1
	truncateMiddle = 2
)

// ellipsisToken returns a line of "…" in the attributes str starts with.
func ellipsisToken(str uintptr) uintptr {
	text := CFString("…")
	defer Release(text)
	ellipsis := cfAttributedStringCreate(0, text, cfAttributedStringAttrsAt(str, 0, nil))
	defer Release(ellipsis)
	return ctLineCreateWithAttrString(ellipsis)
}

func (p *prepared) Release() {
	Release(p.fs)
	Release(p.str)
	p.fs, p.str = 0, 0
}

// Layout is text CoreText laid out on the engine goroutine, drawn on the
// main thread.
type Layout struct {
	frame  uintptr
	height float64
	// With a line limit: the lines shown before the last, the last one
	// truncated, and their baselines from the top.
	lines     []uintptr
	truncated uintptr
	origins   []CGPoint
	shown     int
	restStart int // the truncated line's first UTF-16 offset
	// clip is the width a clipped last line is cut at, 0 for none.
	clip float64
	// shadow is drawn behind the glyphs and their lines.
	shadow core.TextShadow

	text   string
	toUTF8 []int

	// under are span backgrounds, over strikethroughs, both from the
	// box's top left.
	under, over []box
	attachments []attachmentBox
}

// Release implements core.TextLayout.
func (l *Layout) Release() {
	if l == nil {
		return
	}
	Release(l.frame)
	for _, line := range l.lines {
		Release(line)
	}
	Release(l.truncated)
	*l = Layout{}
}

// placed returns l's lines, their baselines from the box's top and the
// UTF-16 offset each line's string range starts from: 0, or the rest's
// start for the truncated last line.
func (l *Layout) placed() (lines []uintptr, baselines []CGPoint, bases []int) {
	if l.frame != 0 {
		arr := ctFrameGetLines(l.frame)
		n := cfArrayGetCount(arr)
		if n == 0 {
			return nil, nil, nil
		}
		origins := make([]CGPoint, n)
		ctFrameGetLineOrigins(l.frame, cfRange{Length: n}, &origins[0])
		// Draw puts the frame's bottom at the box's top plus height+1.
		for i := range n {
			lines = append(lines, cfArrayGetValueAtIndex(arr, i))
			baselines = append(baselines, CGPoint{X: origins[i].X, Y: l.height + 1 - origins[i].Y})
		}
		return lines, baselines, make([]int, n)
	}
	lines = append(append(lines, l.lines...), l.truncated)
	bases = make([]int, len(lines))
	bases[len(lines)-1] = l.restStart
	return lines, l.origins[:l.shown], bases
}

// TextLines implements core.LineLayout: each line's range, box and its
// first run's font metrics.
func (l *Layout) TextLines() []core.TextLine {
	if l == nil || (l.frame == 0 && l.truncated == 0) {
		return nil
	}
	lines, baselines, bases := l.placed()
	out := make([]core.TextLine, 0, len(lines))
	for i, line := range lines {
		var ascent, descent, leading float64
		width := ctLineGetTypographicBounds(line, &ascent, &descent, &leading)
		if line == l.truncated && l.clip > 0 {
			width = min(width, l.clip)
		}
		r := ctLineGetStringRange(line)
		start, end := r.Location+bases[i], r.Location+r.Length+bases[i]
		if line == l.truncated {
			end = len(l.toUTF8) - 1 // the rest, which the line stands for
		}
		tl := core.TextLine{Ascender: core.Points(ascent), Descender: core.Points(descent),
			Bounds: core.Bounds{Origin: core.Point{X: core.Points(baselines[i].X), Y: core.Points(baselines[i].Y - ascent - leading/2)},
				Size: core.Size{Width: core.Points(width), Height: core.Points(ascent + descent + leading)}}}
		if n := len(l.toUTF8) - 1; n >= 0 {
			tl.Start, tl.End = l.toUTF8[min(max(start, 0), n)], l.toUTF8[min(max(end, 0), n)]
		}
		if runs := ctLineGetGlyphRuns(line); cfArrayGetCount(runs) > 0 {
			if font := cfDictionaryGetValue(ctRunGetAttributes(cfArrayGetValueAtIndex(runs, 0)), attrFont); font != 0 {
				tl.CapHeight, tl.XHeight = core.Points(ctFontGetCapHeight(font)), core.Points(ctFontGetXHeight(font))
			}
		}
		out = append(out, tl)
	}
	return out
}

// IndexAt returns the UTF-8 byte offset of the character at p, a point of
// the view Draw drew l in at box, or -1 when no character is there.
func (l *Layout) IndexAt(p CGPoint, box CGRect) int {
	if l == nil || (l.frame == 0 && l.truncated == 0) {
		return -1
	}
	x, y := p.X-box.Origin.X, p.Y-box.Origin.Y // from the box's top left
	lines, baselines, bases := l.placed()
	for i, line := range lines {
		var ascent, descent, leading float64
		width := ctLineGetTypographicBounds(line, &ascent, &descent, &leading)
		o := baselines[i]
		if y < o.Y-ascent-leading/2 || y > o.Y+descent+leading/2 {
			continue
		}
		lx := x - o.X
		if lx < 0 || lx > width {
			return -1
		}
		r := ctLineGetStringRange(line)
		idx := ctLineIndexForPosition(line, CGPoint{X: lx})
		// The index is the caret's: step back when it is past the point.
		if idx > r.Location && ctLineOffsetForIndex(line, idx, nil) > lx {
			idx--
		}
		idx = min(max(idx, r.Location), r.Location+r.Length-1) + bases[i]
		if idx < 0 || idx >= len(l.toUTF8)-1 {
			return -1
		}
		return l.toUTF8[idx]
	}
	return -1
}

// OffsetAt returns the UTF-16 offset of the caret nearest p, a point of
// the view Draw drew l in at box: on the line nearest p, before the
// line's break.
func (l *Layout) OffsetAt(p CGPoint, box CGRect) int {
	if l == nil {
		return 0
	}
	lines, baselines, bases := l.placed()
	if len(lines) == 0 {
		return 0
	}
	y := p.Y - box.Origin.Y
	i := len(lines) - 1
	for k, line := range lines {
		var ascent, descent, leading float64
		ctLineGetTypographicBounds(line, &ascent, &descent, &leading)
		if y <= baselines[k].Y+descent+leading/2 {
			i = k
			break
		}
	}
	line := lines[i]
	r := ctLineGetStringRange(line)
	idx := ctLineIndexForPosition(line, CGPoint{X: p.X - box.Origin.X - baselines[i].X})
	end := r.Location + r.Length
	if end > r.Location && l.breaksAt(end-1+bases[i]) {
		end--
	}
	return min(max(idx, r.Location), end) + bases[i]
}

// breaksAt reports whether UTF-16 offset i is a line break.
func (l *Layout) breaksAt(i int) bool {
	if i < 0 || i >= len(l.toUTF8)-1 {
		return false
	}
	c := l.text[l.toUTF8[i]]
	return c == '\n' || c == '\r'
}

// SelectionRects returns the boxes of the UTF-16 range from start to end
// in the view Draw draws l in at box, a box per line it covers. A range
// that goes on past a line covers the rest of it.
func (l *Layout) SelectionRects(start, end int, box CGRect) []CGRect {
	if l == nil || end <= start {
		return nil
	}
	var out []CGRect
	lines, baselines, bases := l.placed()
	for i, line := range lines {
		r := ctLineGetStringRange(line)
		from, to := r.Location+bases[i], r.Location+r.Length+bases[i]
		a, b := max(start, from), min(end, to)
		if a >= b {
			continue
		}
		var ascent, descent, leading float64
		width := ctLineGetTypographicBounds(line, &ascent, &descent, &leading)
		x0 := ctLineOffsetForIndex(line, a-bases[i], nil)
		x1 := ctLineOffsetForIndex(line, b-bases[i], nil)
		if end >= to {
			x1 = width
		}
		if x1 < x0 {
			x0, x1 = x1, x0
		}
		o := baselines[i]
		top := o.Y - ascent - leading/2
		out = append(out, Rect(box.Origin.X+o.X+x0, box.Origin.Y+top, x1-x0, ascent+descent+leading))
	}
	return out
}

// FillRects fills rects with color, a CGColor.
func FillRects(ctx uintptr, rects []CGRect, color uintptr) {
	if ctx == 0 || len(rects) == 0 {
		return
	}
	cgContextSaveGState(ctx)
	cgContextSetFillColor(ctx, color)
	for _, r := range rects {
		cgContextAddRect(ctx, r)
	}
	cgContextFillPath(ctx)
	cgContextRestoreGState(ctx)
}

// Draw draws l in box of a flipped context of a view viewHeight tall.
func (l *Layout) Draw(ctx uintptr, viewHeight float64, box CGRect) {
	if l == nil || ctx == 0 || (l.frame == 0 && l.truncated == 0) {
		return
	}
	cgContextSaveGState(ctx)
	defer cgContextRestoreGState(ctx)
	if s := l.shadow; s.Color != 0 {
		// Shadow offsets are in the base space, y up under the flipped
		// context: a positive offset must still go down.
		c := CGColor(s.Color)
		cgContextSetShadowWithColor(ctx, CGSize{Width: float64(s.Offset.X), Height: -float64(s.Offset.Y)},
			float64(s.Radius), c)
		Release(c)
	}
	l.drawGlyphs(ctx, viewHeight, box)
	if len(l.over) > 0 {
		drawLines(ctx, l.over, box)
	}
}

func (l *Layout) drawGlyphs(ctx uintptr, viewHeight float64, box CGRect) {
	cgContextSaveGState(ctx)
	defer cgContextRestoreGState(ctx)
	// CoreText draws bottom-up; flip the view's top-down coordinates.
	cgContextTranslateCTM(ctx, 0, viewHeight)
	cgContextScaleCTM(ctx, 1, -1)
	if l.frame != 0 {
		cgContextTranslateCTM(ctx, box.Origin.X, viewHeight-box.Origin.Y-l.height-1)
		ctFrameDraw(l.frame, ctx)
		return
	}
	top := viewHeight - box.Origin.Y
	for i := range l.shown {
		line := l.truncated
		if i < len(l.lines) {
			line = l.lines[i]
		}
		o := l.origins[i]
		if line == l.truncated && l.clip > 0 {
			cgContextSaveGState(ctx)
			cgContextClipToRect(ctx, Rect(box.Origin.X, -tall, l.clip, 2*tall))
		}
		cgContextSetTextPosition(ctx, box.Origin.X+o.X, top-o.Y)
		ctLineDraw(line, ctx)
		if line == l.truncated && l.clip > 0 {
			cgContextRestoreGState(ctx)
		}
	}
}
