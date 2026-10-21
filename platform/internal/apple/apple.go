//go:build darwin

// Package apple holds what the AppKit and UIKit backends share, called
// through purego without cgo: CoreFoundation strings, CoreGraphics colors
// and CoreText text, measured on the engine goroutine and drawn on the
// main thread.
package apple

import (
	"math"
	"structs"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/core"
)

// CGPoint, CGSize and CGRect match CoreGraphics' layouts on 64-bit Apple
// platforms.
type CGPoint struct {
	_    structs.HostLayout
	X, Y float64
}

type CGSize struct {
	_             structs.HostLayout
	Width, Height float64
}

type CGRect struct {
	_      structs.HostLayout
	Origin CGPoint
	Size   CGSize
}

// Rect returns a CGRect.
func Rect(x, y, w, h float64) CGRect {
	return CGRect{Origin: CGPoint{X: x, Y: y}, Size: CGSize{Width: w, Height: h}}
}

type cfRange struct {
	_                structs.HostLayout
	Location, Length int
}

const utf8Encoding = 0x08000100

var (
	cfRelease                 func(uintptr)
	cfStringCreateWithBytes   func(alloc uintptr, bytes *byte, n int, encoding uint32, bom bool) uintptr
	cfDictionaryCreate        func(alloc uintptr, keys, values *uintptr, n int, keyCB, valueCB uintptr) uintptr
	cfAttributedStringCreate  func(alloc, str, attrs uintptr) uintptr
	cfAttributedStringLength  func(uintptr) int
	cgColorCreateSRGB         func(r, g, b, a float64) uintptr
	cgPathCreateWithRect      func(rect CGRect, transform uintptr) uintptr
	cgPathRelease             func(uintptr)
	cgContextSaveGState       func(uintptr)
	cgContextRestoreGState    func(uintptr)
	cgContextTranslateCTM     func(ctx uintptr, x, y float64)
	cgContextScaleCTM         func(ctx uintptr, x, y float64)
	ctFontCreateUIFont        func(kind uint32, size float64, lang uintptr) uintptr
	ctFontGetAscent           func(uintptr) float64
	ctFontGetDescent          func(uintptr) float64
	ctFontGetLeading          func(uintptr) float64
	ctFramesetterCreate       func(str uintptr) uintptr
	ctFramesetterSuggestSize  func(fs uintptr, r cfRange, attrs uintptr, limit CGSize, fit uintptr) CGSize
	ctFramesetterCreateFrame  func(fs uintptr, r cfRange, path, attrs uintptr) uintptr
	ctFrameDraw               func(frame, ctx uintptr)
	keyCallBacks, valueCBs    uintptr
	fontAttr, foregroundColor uintptr
)

func load(path string) uintptr {
	lib, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	return lib
}

func symbol(lib uintptr, name string) uintptr {
	addr, err := purego.Dlsym(lib, name)
	if err != nil {
		panic(err)
	}
	return addr
}

// constant reads a global CFStringRef constant of lib.
func constant(lib uintptr, name string) uintptr {
	addr := symbol(lib, name)
	return **(**uintptr)(unsafe.Pointer(&addr))
}

func init() {
	cf := load("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
	cg := load("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
	ct := load("/System/Library/Frameworks/CoreText.framework/CoreText")

	purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
	purego.RegisterLibFunc(&cfStringCreateWithBytes, cf, "CFStringCreateWithBytes")
	purego.RegisterLibFunc(&cfDictionaryCreate, cf, "CFDictionaryCreate")
	purego.RegisterLibFunc(&cfAttributedStringCreate, cf, "CFAttributedStringCreate")
	purego.RegisterLibFunc(&cfAttributedStringLength, cf, "CFAttributedStringGetLength")
	keyCallBacks = symbol(cf, "kCFTypeDictionaryKeyCallBacks")
	valueCBs = symbol(cf, "kCFTypeDictionaryValueCallBacks")

	purego.RegisterLibFunc(&cgColorCreateSRGB, cg, "CGColorCreateSRGB")
	purego.RegisterLibFunc(&cgPathCreateWithRect, cg, "CGPathCreateWithRect")
	purego.RegisterLibFunc(&cgPathRelease, cg, "CGPathRelease")
	purego.RegisterLibFunc(&cgContextSaveGState, cg, "CGContextSaveGState")
	purego.RegisterLibFunc(&cgContextRestoreGState, cg, "CGContextRestoreGState")
	purego.RegisterLibFunc(&cgContextTranslateCTM, cg, "CGContextTranslateCTM")
	purego.RegisterLibFunc(&cgContextScaleCTM, cg, "CGContextScaleCTM")

	purego.RegisterLibFunc(&ctFontCreateUIFont, ct, "CTFontCreateUIFontForLanguage")
	purego.RegisterLibFunc(&ctFontGetAscent, ct, "CTFontGetAscent")
	purego.RegisterLibFunc(&ctFontGetDescent, ct, "CTFontGetDescent")
	purego.RegisterLibFunc(&ctFontGetLeading, ct, "CTFontGetLeading")
	purego.RegisterLibFunc(&ctFramesetterCreate, ct, "CTFramesetterCreateWithAttributedString")
	purego.RegisterLibFunc(&ctFramesetterSuggestSize, ct, "CTFramesetterSuggestFrameSizeWithConstraints")
	purego.RegisterLibFunc(&ctFramesetterCreateFrame, ct, "CTFramesetterCreateFrame")
	purego.RegisterLibFunc(&ctFrameDraw, ct, "CTFrameDraw")
	fontAttr = constant(ct, "kCTFontAttributeName")
	foregroundColor = constant(ct, "kCTForegroundColorAttributeName")
}

// Release calls CFRelease on a CoreFoundation object.
func Release(ref uintptr) {
	if ref != 0 {
		cfRelease(ref)
	}
}

// CFString returns a new CFString (an NSString) holding s. The caller
// releases it.
func CFString(s string) uintptr {
	b := []byte(s)
	if len(b) == 0 {
		b = []byte{0}
		return cfStringCreateWithBytes(0, &b[0], 0, utf8Encoding, false)
	}
	return cfStringCreateWithBytes(0, &b[0], len(b), utf8Encoding, false)
}

// NSString returns an autoreleased NSString holding s.
func NSString(s string) objc.ID {
	ref := CFString(s)
	return objc.ID(ref).Send(objc.RegisterName("autorelease"))
}

// GoString returns the Go string of an NSString.
func GoString(ns objc.ID) string {
	if ns == 0 {
		return ""
	}
	p := objc.Send[*byte](ns, objc.RegisterName("UTF8String"))
	if p == nil {
		return ""
	}
	n := 0
	for *(*byte)(unsafe.Add(unsafe.Pointer(p), n)) != 0 {
		n++
	}
	return string(unsafe.Slice(p, n))
}

// CGColor returns a new CGColor of c. The caller releases it.
func CGColor(c core.Color) uintptr {
	r, g, b, a := c.Components()
	return cgColorCreateSRGB(r, g, b, a)
}

// CoreText's UI font kinds.
const (
	uiFontSystem           = 2
	uiFontEmphasizedSystem = 3
)

// Font returns a new CTFont (a UIFont or NSFont) of the system face. The
// caller releases it.
func Font(f core.Font) uintptr {
	kind := uint32(uiFontSystem)
	if f.Weight >= 600 {
		kind = uiFontEmphasizedSystem
	}
	return ctFontCreateUIFont(kind, float64(f.Size), 0)
}

// Text is shaped text a view draws: an attributed string, owned.
type Text struct{ str uintptr }

// NewText returns text in font and color.
func NewText(s string, font core.Font, color core.Color) *Text {
	f := Font(font)
	c := CGColor(color)
	keys := []uintptr{fontAttr, foregroundColor}
	values := []uintptr{f, c}
	attrs := cfDictionaryCreate(0, &keys[0], &values[0], 2, keyCallBacks, valueCBs)
	str := CFString(s)
	t := &Text{str: cfAttributedStringCreate(0, str, attrs)}
	Release(str)
	Release(attrs)
	Release(c)
	Release(f)
	return t
}

// Free releases the text.
func (t *Text) Free() {
	if t != nil {
		Release(t.str)
		t.str = 0
	}
}

// Measure implements core.TextEngine with CoreText. It is safe to call
// off the main thread.
func Measure(text string, font core.Font, maxWidth core.Points) core.Size {
	t := NewText(text, font, 0)
	defer t.Free()
	limit := CGSize{Width: math.MaxFloat32, Height: math.MaxFloat32}
	if !maxWidth.IsAuto() {
		limit.Width = float64(maxWidth)
	}
	fs := ctFramesetterCreate(t.str)
	defer Release(fs)
	size := ctFramesetterSuggestSize(fs, cfRange{}, 0, limit, 0)
	if cfAttributedStringLength(t.str) == 0 {
		f := Font(font)
		size.Height = ctFontGetAscent(f) + ctFontGetDescent(f) + ctFontGetLeading(f)
		Release(f)
	}
	return core.Size{Width: core.Points(math.Ceil(size.Width)), Height: core.Points(math.Ceil(size.Height))}
}

// Draw draws t into a flipped context of a view of height viewHeight, in
// box (top-left origin).
func (t *Text) Draw(ctx uintptr, viewHeight float64, box CGRect) {
	if t == nil || t.str == 0 || ctx == 0 {
		return
	}
	cgContextSaveGState(ctx)
	defer cgContextRestoreGState(ctx)
	// CoreText draws bottom-up; flip the view's top-down coordinates.
	cgContextTranslateCTM(ctx, 0, viewHeight)
	cgContextScaleCTM(ctx, 1, -1)
	box.Origin.Y = viewHeight - box.Origin.Y - box.Size.Height
	fs := ctFramesetterCreate(t.str)
	path := cgPathCreateWithRect(box, 0)
	frame := ctFramesetterCreateFrame(fs, cfRange{}, path, 0)
	ctFrameDraw(frame, ctx)
	Release(frame)
	cgPathRelease(path)
	Release(fs)
}
