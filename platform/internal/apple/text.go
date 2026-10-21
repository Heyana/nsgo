//go:build darwin

package apple

import (
	"math"
	"structs"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/oneo-me/nsgo/core"
)

var (
	cfAttributedStringCreate   func(alloc, str, attrs uintptr) uintptr
	cfAttributedStringLength   func(uintptr) int
	cfAttributedStringSubstr   func(alloc, str uintptr, r cfRange) uintptr
	cfAttributedStringAttrsAt  func(str uintptr, loc int, effective *cfRange) uintptr
	cfRetain                   func(uintptr) uintptr
	cfArrayGetCount            func(uintptr) int
	cfArrayGetValueAtIndex     func(arr uintptr, i int) uintptr
	cgPathCreateWithRect       func(rect CGRect, transform uintptr) uintptr
	cgPathRelease              func(uintptr)
	cgContextSaveGState        func(uintptr)
	cgContextRestoreGState     func(uintptr)
	cgContextTranslateCTM      func(ctx uintptr, x, y float64)
	cgContextScaleCTM          func(ctx uintptr, x, y float64)
	cgContextSetTextPosition   func(ctx uintptr, x, y float64)
	ctFontCreateUIFont         func(kind uint32, size float64, lang uintptr) uintptr
	ctFontCreateWithName       func(name uintptr, size float64, matrix uintptr) uintptr
	ctFontCopyFontDescriptor   func(font uintptr) uintptr
	ctFontDescriptorCopyAttrs  func(desc, attrs uintptr) uintptr
	ctFontCreateWithDescriptor func(desc uintptr, size float64, matrix uintptr) uintptr
	ctFontGetAscent            func(uintptr) float64
	ctFontGetDescent           func(uintptr) float64
	ctFontGetLeading           func(uintptr) float64
	ctParagraphStyleCreate     func(settings unsafe.Pointer, n uintptr) uintptr
	ctFramesetterCreate        func(str uintptr) uintptr
	ctFramesetterSuggestSize   func(fs uintptr, r cfRange, attrs uintptr, limit CGSize, fit uintptr) CGSize
	ctFramesetterCreateFrame   func(fs uintptr, r cfRange, path, attrs uintptr) uintptr
	ctFrameGetLines            func(frame uintptr) uintptr
	ctFrameGetLineOrigins      func(frame uintptr, r cfRange, origins *CGPoint)
	ctFrameDraw                func(frame, ctx uintptr)
	ctLineGetTypographicBounds func(line uintptr, ascent, descent, leading *float64) float64
	ctLineGetStringRange       func(line uintptr) cfRange
	ctLineCreateWithAttrString func(str uintptr) uintptr
	ctLineCreateTruncatedLine  func(line uintptr, width float64, kind uint32, token uintptr) uintptr
	ctLineDraw                 func(line, ctx uintptr)

	attrFont, attrColor, attrKern, attrUnderline, attrParagraph uintptr
	attrTraits, attrWeight, attrSymbolic                        uintptr
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
	attrFont = constant(ct, "kCTFontAttributeName")
	attrColor = constant(ct, "kCTForegroundColorAttributeName")
	attrKern = constant(ct, "kCTKernAttributeName")
	attrUnderline = constant(ct, "kCTUnderlineStyleAttributeName")
	attrParagraph = constant(ct, "kCTParagraphStyleAttributeName")
	attrTraits = constant(ct, "kCTFontTraitsAttribute")
	attrWeight = constant(ct, "kCTFontWeightTrait")
	attrSymbolic = constant(ct, "kCTFontSymbolicTrait")
}

// CoreText weights of CSS's 100 to 900, from UIFontWeight.
var weights = [...]float64{-0.8, -0.6, -0.4, 0, 0.23, 0.3, 0.4, 0.56, 0.62}

// Font returns a new CTFont (an NSFont or UIFont) for f: the system face
// unless f names a family. The caller releases it.
func Font(f core.Font) uintptr {
	var base uintptr
	if f.Family != "" {
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
	specAlignment                      = 0
	specMaxLineHeight                  = 8
	specMinLineHeight                  = 9
)

// attributed returns a new CFAttributedString of t. The caller releases
// it.
func attributed(t *core.AttributedText) uintptr {
	st := t.Style
	font := Font(st.Font)
	color := CGColor(st.Color)
	keys := []uintptr{attrFont, attrColor}
	values := []uintptr{font, color}
	owned := []uintptr{font, color}

	align := uint8(alignLeft)
	switch st.Align {
	case core.TextAlignCenter:
		align = alignCenter
	case core.TextAlignEnd:
		align = alignRight
	}
	lineHeight := float64(st.LineHeight)
	settings := []paragraphSetting{{spec: specAlignment, size: 1, value: unsafe.Pointer(&align)}}
	if lineHeight > 0 {
		settings = append(settings,
			paragraphSetting{spec: specMinLineHeight, size: 8, value: unsafe.Pointer(&lineHeight)},
			paragraphSetting{spec: specMaxLineHeight, size: 8, value: unsafe.Pointer(&lineHeight)})
	}
	para := ctParagraphStyleCreate(unsafe.Pointer(&settings[0]), uintptr(len(settings)))
	keys, values, owned = append(keys, attrParagraph), append(values, para), append(owned, para)
	if st.LetterSpacing != 0 {
		kern := float64Number(float64(st.LetterSpacing))
		keys, values, owned = append(keys, attrKern), append(values, kern), append(owned, kern)
	}
	if st.Decoration&core.TextDecorationUnderline != 0 {
		const single = 1
		u := int32Number(single)
		keys, values, owned = append(keys, attrUnderline), append(values, u), append(owned, u)
	}
	attrs := dictionary(keys, values)
	str := CFString(t.Text)
	out := cfAttributedStringCreate(0, str, attrs)
	Release(str)
	Release(attrs)
	for _, ref := range owned {
		Release(ref)
	}
	return out
}

// TextEngine implements core.TextEngine with CoreText. Strikethrough is
// not drawn: CoreText has no attribute for it.
type TextEngine struct{}

// Prepare implements core.TextEngine. It is safe off the main thread.
func (TextEngine) Prepare(t *core.AttributedText) core.PreparedText {
	str := attributed(t)
	p := &prepared{str: str, fs: ctFramesetterCreate(str), maxLines: t.MaxLines}
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
	emptyHeight float64
}

// unbounded is a size limit nothing reaches; tall is a frame height
// whose line origins keep their precision.
const (
	unbounded = math.MaxFloat32
	tall      = 1e7
)

func (p *prepared) Measure(maxWidth core.Points) core.Size {
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
	l := &Layout{height: float64(size.Height)}
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
	token := ellipsisToken(rest)
	l.truncated = ctLineCreateTruncatedLine(line, float64(size.Width), truncateEnd, token)
	Release(token)
	Release(line)
	Release(rest)
	l.shown = p.maxLines
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

const truncateEnd = 1

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

// Draw draws l in box of a flipped context of a view viewHeight tall.
func (l *Layout) Draw(ctx uintptr, viewHeight float64, box CGRect) {
	if l == nil || ctx == 0 || (l.frame == 0 && l.truncated == 0) {
		return
	}
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
		cgContextSetTextPosition(ctx, box.Origin.X+o.X, top-o.Y)
		ctLineDraw(line, ctx)
	}
}
