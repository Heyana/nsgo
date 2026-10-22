//go:build windows

package windows

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/oneo-me/nsgo/core"
)

// GDI has no antialiasing at all, so a canvas's curves and the strokes of
// its paths step a whole pixel at a time. GDI+ antialiases both, but it
// draws through a DC of its own: it ignores the clipping GDI set on the
// target, and its own clipping steps a whole pixel too. So the painter has
// GDI+ draw into a bitmap of its own and blends that back over the target
// with AlphaBlend, which respects the clipping.

var (
	gdiplus = syscall.NewLazyDLL("gdiplus.dll")

	gdiplusStartup         = gdiplus.NewProc("GdiplusStartup")
	gdipCreateFromHDC      = gdiplus.NewProc("GdipCreateFromHDC")
	gdipDeleteGraphics     = gdiplus.NewProc("GdipDeleteGraphics")
	gdipFlush              = gdiplus.NewProc("GdipFlush")
	gdipSetSmoothingMode   = gdiplus.NewProc("GdipSetSmoothingMode")
	gdipSetPixelOffsetMode = gdiplus.NewProc("GdipSetPixelOffsetMode")
	gdipCreateMatrix2      = gdiplus.NewProc("GdipCreateMatrix2")
	gdipDeleteMatrix       = gdiplus.NewProc("GdipDeleteMatrix")
	gdipSetWorldTransform  = gdiplus.NewProc("GdipSetWorldTransform")
	gdipCreatePath         = gdiplus.NewProc("GdipCreatePath")
	gdipDeletePath         = gdiplus.NewProc("GdipDeletePath")
	gdipStartPathFigure    = gdiplus.NewProc("GdipStartPathFigure")
	gdipAddPathLine        = gdiplus.NewProc("GdipAddPathLine")
	gdipAddPathBezier      = gdiplus.NewProc("GdipAddPathBezier")
	gdipClosePathFigure    = gdiplus.NewProc("GdipClosePathFigure")
	gdipCreateSolidFill    = gdiplus.NewProc("GdipCreateSolidFill")
	gdipDeleteBrush        = gdiplus.NewProc("GdipDeleteBrush")
	gdipFillPath           = gdiplus.NewProc("GdipFillPath")
	gdipCreatePen1         = gdiplus.NewProc("GdipCreatePen1")
	gdipDeletePen          = gdiplus.NewProc("GdipDeletePen")
	gdipSetPenLineCap      = gdiplus.NewProc("GdipSetPenLineCap197819")
	gdipSetPenLineJoin     = gdiplus.NewProc("GdipSetPenLineJoin")
	gdipDrawPath           = gdiplus.NewProc("GdipDrawPath")
)

const (
	smoothingAntiAlias = 4 // SmoothingModeAntiAlias
	pixelOffsetHalf    = 4 // PixelOffsetModeHighQuality: edges land on whole pixels
	flushSync          = 1 // FlushIntentionSync
	unitWorld          = 0
	windingFill        = 1 // FillModeWinding, as a canvas fills its paths
	capFlat            = 0
	capSquare          = 1
	capRound           = 2
	joinMiter          = 0
	joinBevel          = 1
	joinRound          = 2
)

// gdiplusInput is GdiplusStartupInput.
type gdiplusInput struct {
	Version                  uint32
	_                        uint32
	Callback                 uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs   int32
}

// startGDIPlus starts GDI+ once per process and reports whether it runs.
var startGDIPlus = sync.OnceValue(func() bool {
	var token uintptr
	in := gdiplusInput{Version: 1}
	r, _, _ := gdiplusStartup.Call(uintptr(unsafe.Pointer(&token)), uintptr(unsafe.Pointer(&in)), 0)
	return r == 0
})

// smoothing is a bitmap GDI+ draws smooth shapes into, and the graphics
// drawing into it.
type smoothing struct {
	bmp *bitmap
	g   uintptr
}

// smoothing returns the window's smoothing bitmap, cleared, for a box of
// width by height pixels.
func (w *window) smoothing(width, height int32) *smoothing {
	if !startGDIPlus() {
		return nil
	}
	if s := w.aa; s != nil && s.bmp.width >= width && s.bmp.height >= height {
		// The bitmap may be wider than the box, so clear it row by row.
		pix := unsafe.Slice((*uint32)(s.bmp.bits), int(s.bmp.width)*int(s.bmp.height))
		for y := int32(0); y < height; y++ {
			row := pix[int(y)*int(s.bmp.width):]
			clear(row[:width])
		}
		return s
	}
	if w.aa != nil {
		gdipDeleteGraphics.Call(w.aa.g)
		w.aa.bmp.free()
		w.aa = nil
	}
	bmp, ok := newBitmap(0, width, height)
	if !ok {
		return nil
	}
	var g uintptr
	if r, _, _ := gdipCreateFromHDC.Call(bmp.dc, uintptr(unsafe.Pointer(&g))); r != 0 || g == 0 {
		bmp.free()
		return nil
	}
	gdipSetSmoothingMode.Call(g, smoothingAntiAlias)
	gdipSetPixelOffsetMode.Call(g, pixelOffsetHalf)
	w.aa = &smoothing{bmp: bmp, g: g}
	return w.aa
}

// drawAA has fn draw into the smoothing bitmap at the target's pixels and
// blends the result over box. It reports whether it drew.
func (p *painter) drawAA(box rect, fn func(g uintptr) bool) bool {
	if !startGDIPlus() {
		return false
	}
	gdiFlush.Call() // the bitmap may still be read by a batched blend
	box = box.intersect(p.clipBox())
	if box.empty() {
		return false
	}
	s := p.w.smoothing(box.Right-box.Left, box.Bottom-box.Top)
	if s == nil {
		return false
	}
	// GDI+ draws with the box at the bitmap's origin, in the target's
	// pixels.
	var m uintptr
	gdipCreateMatrix2.Call(real32(1), real32(0), real32(0), real32(1),
		real32(-float64(box.Left)), real32(-float64(box.Top)), uintptr(unsafe.Pointer(&m)))
	gdipSetWorldTransform.Call(s.g, m)
	gdipDeleteMatrix.Call(m)
	if !fn(s.g) {
		return false
	}
	gdipFlush.Call(s.g, flushSync)
	width, height := uintptr(box.Right-box.Left), uintptr(box.Bottom-box.Top)
	p.device(func() {
		alphaBlend.Call(p.dc, uintptr(box.Left), uintptr(box.Top), width, height,
			s.bmp.dc, 0, 0, width, height, blendPremultiplied)
	})
	return true
}

// argb returns c as GDI+ takes it, 0xAARRGGBB.
func argb(c core.Color) uint32 {
	return uint32(c>>8) | uint32(c&0xff)<<24
}

// real32 returns v as GDI+'s REAL, the bits of a float32.
func real32(v float64) uintptr {
	f := float32(v)
	return uintptr(*(*uint32)(unsafe.Pointer(&f)))
}

// solidBrush returns a GDI+ brush of c, or 0.
func solidBrush(c core.Color) uintptr {
	var brush uintptr
	if r, _, _ := gdipCreateSolidFill.Call(uintptr(argb(c)), uintptr(unsafe.Pointer(&brush))); r != 0 {
		return 0
	}
	return brush
}

// pen returns a GDI+ pen of the given width in the target's pixels, with
// the canvas's line ends and joins, or 0.
func pen(c core.Color, width float64, lineCap core.LineCap, join core.LineJoin) uintptr {
	var p uintptr
	if r, _, _ := gdipCreatePen1.Call(uintptr(argb(c)), real32(width), unitWorld, uintptr(unsafe.Pointer(&p))); r != 0 {
		return 0
	}
	endCap := uintptr(capFlat)
	switch lineCap {
	case core.CapRound:
		endCap = capRound
	case core.CapSquare:
		endCap = capSquare
	}
	gdipSetPenLineCap.Call(p, endCap, endCap, endCap)
	kind := uintptr(joinMiter)
	switch join {
	case core.JoinRound:
		kind = joinRound
	case core.JoinBevel:
		kind = joinBevel
	}
	gdipSetPenLineJoin.Call(p, kind)
	return p
}
