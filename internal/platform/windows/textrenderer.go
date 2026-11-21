//go:build windows

package windows

import (
	"math"
	"sync"
	"syscall"
	"unsafe"
)

// hiddenEffect is an immortal IUnknown sentinel set as the drawing effect on hidden spans: DirectWrite splits runs at its edges, and textRenderer skips every glyph run carrying it so glyphs, color emoji and shadows are never drawn.
type hiddenEffect struct{ vtbl *[3]uintptr }

var (
	hiddenEffectVtbl [3]uintptr
	hiddenEffectObj  = hiddenEffect{vtbl: &hiddenEffectVtbl}
)

func (h *hiddenEffect) ptr() uintptr { return uintptr(unsafe.Pointer(h)) }

func hiddenEffectQueryInterface(h *hiddenEffect, iid *guid, out *unsafe.Pointer) uintptr {
	if *iid == iidUnknown {
		*out = unsafe.Pointer(h)
		return sOK
	}
	*out = nil
	return eNoInterface
}

func hiddenEffectKeep(*hiddenEffect) uintptr { return 1 }

// textRange is a UTF-16 range on an IDWriteTextLayout.
type textRange struct{ start, length uint32 }

// textDrawContext is passed as clientDrawingContext to IDWriteTextLayout::Draw.
type textDrawContext struct {
	dc           *comObject
	defaultBrush *comObject
	scale        float32
	colorFont    bool
	shadow       bool
}

// textRenderer is an immortal IDWriteTextRenderer used for layouts with hidden spans: it skips any glyph run carrying hiddenEffect and draws the rest (translating color glyph runs when colorFont is set).
type textRenderer struct{ vtbl *[10]uintptr }

var (
	textRendererVtbl [10]uintptr
	textRendererObj  = textRenderer{vtbl: &textRendererVtbl}

	theDWriteFactory2 = sync.OnceValue(func() *comObject {
		f := dwriteFactory()
		if f == nil {
			return nil
		}
		return f.query(&iidDWriteFactory2)
	})
)

func (r *textRenderer) ptr() uintptr { return uintptr(unsafe.Pointer(r)) }

func init() {
	hiddenEffectVtbl = [3]uintptr{
		syscall.NewCallback(hiddenEffectQueryInterface),
		syscall.NewCallback(hiddenEffectKeep),
		syscall.NewCallback(hiddenEffectKeep),
	}
	textRendererVtbl = [10]uintptr{
		syscall.NewCallback(textRendererQueryInterface),
		syscall.NewCallback(textRendererKeep),
		syscall.NewCallback(textRendererKeep),
		syscall.NewCallback(textRendererPixelSnappingDisabled),
		syscall.NewCallback(textRendererCurrentTransform),
		syscall.NewCallback(textRendererPixelsPerDip),
		textDrawGlyphRunSlot(),
		syscall.NewCallback(textRendererDrawLineNoop),
		syscall.NewCallback(textRendererDrawLineNoop),
		syscall.NewCallback(textRendererDrawInlineNoop),
	}
}

func textRendererQueryInterface(r *textRenderer, iid *guid, out *unsafe.Pointer) uintptr {
	if *iid == iidUnknown || *iid == iidDWritePixelSnap || *iid == iidDWriteTextRenderer {
		*out = unsafe.Pointer(r)
		return sOK
	}
	*out = nil
	return eNoInterface
}

func textRendererKeep(*textRenderer) uintptr { return 1 }

func textRendererPixelSnappingDisabled(r *textRenderer, ctx uintptr, disabled *int32) uintptr {
	*disabled = 0
	return sOK
}

func textRendererCurrentTransform(r *textRenderer, ctx uintptr, transform *matrix3x2) uintptr {
	*transform = matrix3x2{m11: 1, m22: 1}
	return sOK
}

func textRendererPixelsPerDip(r *textRenderer, ctx *textDrawContext, pixelsPerDip *float32) uintptr {
	*pixelsPerDip = ctx.scale
	return sOK
}

// DrawUnderline and DrawStrikethrough: decorations are drawn by painter.
func textRendererDrawLineNoop(r *textRenderer, ctx, x, y, deco, effect uintptr) uintptr { return sOK }

// DrawInlineObject: inline attachment views are drawn as child views.
func textRendererDrawInlineNoop(r *textRenderer, ctx, x, y, obj, sideways, rtl, effect uintptr) uintptr {
	return sOK
}

// textDrawGlyphRunWords is IDWriteTextRenderer::DrawGlyphRun, receiving baselineOriginX and baselineOriginY as float32 bit patterns (moved from float registers by textrenderer_*.s on amd64 and arm64).
func textDrawGlyphRunWords(r *textRenderer, ctx *textDrawContext, xBits, yBits, measuringMode, glyphRunPtr, descPtr uintptr, effect *comObject) uintptr {
	if shouldSkipGlyphRun(effect.ptr()) {
		return sOK
	}
	x, y := math.Float32frombits(uint32(xBits)), math.Float32frombits(uint32(yBits))
	brush := ctx.defaultBrush
	if !ctx.shadow && effect != nil {
		brush = effect
	}
	if brush == nil {
		return sOK
	}
	if ctx.colorFont && !ctx.shadow && drawColorGlyphRun(ctx.dc, x, y, measuringMode, glyphRunPtr, descPtr, brush) {
		return sOK
	}
	ctx.dc.callPoint(rtDrawGlyphRun, x, y, glyphRunPtr, brush.ptr(), measuringMode)
	return sOK
}

// shouldSkipGlyphRun reports whether a glyph run's drawing effect marks a hidden span.
func shouldSkipGlyphRun(effectPtr uintptr) bool {
	return effectPtr == hiddenEffectObj.ptr()
}

// drawColorGlyphRun translates a color font run (such as COLR emoji) into colored layers and draws each one; it returns false for monochrome runs.
func drawColorGlyphRun(dc *comObject, x, y float32, measuringMode, glyphRunPtr, descPtr uintptr, defaultBrush *comObject) bool {
	f2 := theDWriteFactory2()
	if f2 == nil {
		return false
	}
	var enum *comObject
	hr := f2.callF(dw2TranslateColorGlyphRun, 0b11, f32(x), f32(y), glyphRunPtr, descPtr,
		measuringMode, 0, 0, uintptr(unsafe.Pointer(&enum)))
	if failed(hr) || enum == nil {
		return false
	}
	defer enum.release()
	var layerBrush *comObject
	defer releaseAll(&layerBrush)
	for {
		var hasRun int32
		if failed(enum.call(colorEnumMoveNext, uintptr(unsafe.Pointer(&hasRun)))) || hasRun == 0 {
			break
		}
		var run *colorGlyphRun
		if failed(enum.call(colorEnumGetCurrentRun, uintptr(unsafe.Pointer(&run)))) || run == nil {
			break
		}
		brush := defaultBrush
		if run.paletteIndex != 0xffff {
			if layerBrush == nil {
				dc.call(rtCreateSolidColorBrush, uintptr(unsafe.Pointer(&run.runColor)), 0, uintptr(unsafe.Pointer(&layerBrush)))
			} else {
				layerBrush.call(brushSetColor, uintptr(unsafe.Pointer(&run.runColor)))
			}
			if layerBrush != nil {
				brush = layerBrush
			}
		}
		dc.callPoint(rtDrawGlyphRun, run.baselineOriginX, run.baselineOriginY,
			uintptr(unsafe.Pointer(&run.glyphRun)), brush.ptr(), measuringMode)
	}
	return true
}
