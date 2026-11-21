//go:build linux && !android

package linux

import (
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const maxOffscreenPixels = 1 << 24

// offscreenEntry is a cached Cairo ARGB32 image surface for an
// OffscreenCanvas keyed by (ID, Version).
type offscreenEntry struct {
	version       uint64
	surface       uintptr
	width, height int
	points        driver.Size
}

func (e *offscreenEntry) destroy() {
	if e != nil && e.surface != 0 {
		cairoSurfaceDestroy(e.surface)
		e.surface = 0
	}
}

func (w *window) releaseOffscreen(id uint64) {
	if e := w.offscreen[id]; e != nil {
		e.destroy()
		delete(w.offscreen, id)
	}
}

func (w *window) closeOffscreen() {
	for id, e := range w.offscreen {
		e.destroy()
		delete(w.offscreen, id)
	}
	clear(w.offscreenRendering)
}

// offscreenSurface returns the cached Cairo image surface for off, rendering
// off.Drawing only on first use or when off.Version changes.
func (w *window) offscreenSurface(v *view, off *driver.OffscreenCanvas) *offscreenEntry {
	if w.canvasClosed || off == nil {
		return nil
	}
	if e := w.offscreen[off.ID]; e != nil && e.version == off.Version && e.surface != 0 {
		return e
	}
	if w.offscreenRendering[off.ID] {
		return nil
	}
	pw, ph := off.PixelSize()
	if pw <= 0 || ph <= 0 || pw*ph > maxOffscreenPixels {
		return nil
	}
	surf := cairoImageSurfaceCreate(0, int32(pw), int32(ph))
	if surf == 0 {
		return nil
	}
	if w.offscreenRendering == nil {
		w.offscreenRendering = map[uint64]bool{}
	}
	w.offscreenRendering[off.ID] = true
	defer delete(w.offscreenRendering, off.ID)

	cr := cairoCreate(surf)
	scale := float64(off.EffectiveScale())
	cairoScale(cr, scale, scale)
	var ops []driver.DrawOp
	if off.Drawing != nil {
		ops = off.Drawing.Ops
	}
	waited := w.playCanvasOps(v, cr, rect{w: float64(off.Size.Width), h: float64(off.Size.Height)}, ops, driver.CanvasSplit{}, nil)
	cairoDestroy(cr)
	cairoSurfaceFlush(surf)

	if w.offscreen == nil {
		w.offscreen = map[uint64]*offscreenEntry{}
	}
	if old := w.offscreen[off.ID]; old != nil {
		old.destroy()
	}
	version := off.Version
	if waited {
		// Keep the surface alive through this paint, but re-render on the next
		// pass once async images inside the offscreen drawing have loaded.
		version = 0
	}
	e := &offscreenEntry{
		version: version,
		surface: surf,
		width:   pw,
		height:  ph,
		points:  off.Size,
	}
	w.offscreen[off.ID] = e
	return e
}

// showOffscreen renders src's OffscreenCanvas (or reuses the cached surface)
// into slot s for an Image view.
func (w *window) showOffscreen(v *view, s *imageSlot, src driver.ImageSource) {
	s.reset(src)
	e := w.offscreenSurface(v, src.Offscreen)
	if e == nil || e.surface == 0 {
		return
	}
	cairoSurfaceFlush(e.surface)
	data := cairoImageSurfaceGetData(e.surface)
	stride := int(cairoImageSurfaceGetStride(e.surface))
	if data == 0 || stride < e.width*4 {
		return
	}
	rows := unsafe.Slice((*byte)(cPointer(data)), stride*e.height)
	pix := make([]byte, e.width*e.height*4)
	rowLen := e.width * 4
	for y := range e.height {
		copy(pix[y*rowLen:(y+1)*rowLen], rows[y*stride:y*stride+rowLen])
	}
	s.pic = &picture{width: e.width, height: e.height, pix: pix, size: e.points}
}
