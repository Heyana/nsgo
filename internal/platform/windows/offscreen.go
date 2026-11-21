//go:build windows

package windows

import (
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const maxOffscreenPixels = 1 << 24

// offscreenEntry is a cached GPU-resident Direct2D bitmap for an
// OffscreenCanvas keyed by (ID, Version, gfxGeneration).
type offscreenEntry struct {
	version    uint64
	generation uint64
	bitmap     *comObject
	scale      float32
	points     driver.Size
}

func (e *offscreenEntry) release() {
	if e != nil {
		releaseAll(&e.bitmap)
	}
}

func (w *window) releaseOffscreen(id uint64) {
	if e := w.offscreen[id]; e != nil {
		e.release()
		delete(w.offscreen, id)
	}
}

func (w *window) closeOffscreen() {
	for id, e := range w.offscreen {
		e.release()
		delete(w.offscreen, id)
	}
	clear(w.offscreenRendering)
}

// offscreenBitmap returns the cached GPU Direct2D bitmap for off, rendering
// off.Drawing into a bitmap render target only on first use, after a version
// change, or after a graphics device reset.
func (p *painter) offscreenBitmap(v *view, off *driver.OffscreenCanvas) *offscreenEntry {
	w := p.w
	if w.canvasClosed || off == nil {
		return nil
	}
	if e := w.offscreen[off.ID]; e != nil && e.version == off.Version && e.generation == gfxGeneration && e.bitmap != nil {
		return e
	}
	if w.offscreenRendering[off.ID] {
		return nil
	}
	pw, ph := off.PixelSize()
	if pw <= 0 || ph <= 0 || pw*ph > maxOffscreenPixels {
		return nil
	}
	scale := off.EffectiveScale()
	props := bitmapProperties1{
		format:    dxgiFormatBGRA,
		alphaMode: d2dAlphaPremultiplied,
		dpiX:      96 * scale,
		dpiY:      96 * scale,
		options:   d2dBitmapTarget,
	}
	var bmp *comObject
	if failed(p.dc.callSize(dcCreateBitmap, uint32(pw), uint32(ph), 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&bmp)))) {
		return nil
	}
	if w.offscreenRendering == nil {
		w.offscreenRendering = map[uint64]bool{}
	}
	w.offscreenRendering[off.ID] = true
	defer delete(w.offscreenRendering, off.ID)

	world := p.m
	oldScale := p.scale
	waited := false
	p.flatten(func() {
		var target *comObject
		p.dc.call(dcGetTarget, uintptr(unsafe.Pointer(&target)))
		p.dc.call(dcSetTarget, bmp.ptr())
		p.dc.callF(rtSetDpi, 0b11, f32(96*scale), f32(96*scale))
		pushed, split := p.pushed, p.split
		p.pushed, p.split = nil, nil
		p.scale = scale
		p.setTransform(identity)
		clearCol := colorF{}
		p.dc.call(rtClear, uintptr(unsafe.Pointer(&clearCol)))
		c := p.newCanvasPlayer(v, identity, float32(off.Size.Width), float32(off.Size.Height))
		c.off = bmp
		if off.Drawing != nil {
			c.playAll(off.Drawing.Ops)
		}
		waited = c.waited
		for len(p.pushed) > 0 {
			p.pop()
		}
		p.pushed, p.split = pushed, split
		p.scale = oldScale
		p.dc.call(dcSetTarget, target.ptr())
		p.dc.callF(rtSetDpi, 0b11, f32(96*oldScale), f32(96*oldScale))
		target.release()
	})
	p.setTransform(world)

	if w.offscreen == nil {
		w.offscreen = map[uint64]*offscreenEntry{}
	}
	if old := w.offscreen[off.ID]; old != nil {
		old.release()
	}
	version := off.Version
	if waited {
		// Keep the bitmap alive through this frame, but re-render on the next
		// pass once async images inside the offscreen drawing have loaded.
		version = 0
	}
	e := &offscreenEntry{
		version:    version,
		generation: gfxGeneration,
		bitmap:     bmp,
		scale:      scale,
		points:     off.Size,
	}
	w.offscreen[off.ID] = e
	return e
}

// drawOffscreenImage draws an Image view whose source is an OffscreenCanvas.
func (p *painter) drawOffscreenImage(v *view, s *imageSlot, r rectF) {
	e := p.offscreenBitmap(v, s.src.Offscreen)
	if e == nil || e.bitmap == nil {
		return
	}
	nat := e.points
	if s.src.Size.Width > 0 && s.src.Size.Height > 0 {
		nat = s.src.Size
	}
	box := driver.Size{Width: driver.Points(r.right - r.left), Height: driver.Points(r.bottom - r.top)}
	fitted := v.image.ResizeMode.Fit(nat, box, v.image.Position)
	d := rectF{
		left:   r.left + float32(fitted.Origin.X),
		top:    r.top + float32(fitted.Origin.Y),
		right:  r.left + float32(fitted.Origin.X+fitted.Size.Width),
		bottom: r.top + float32(fitted.Origin.Y+fitted.Size.Height),
	}
	if d.empty() {
		return
	}
	src := rectF{0, 0, float32(e.points.Width), float32(e.points.Height)}
	p.dc.callF(rtDrawBitmap, 0b100, e.bitmap.ptr(), uintptr(unsafe.Pointer(&d)), f32(1), imageInterpolation(v.image.Sampling),
		uintptr(unsafe.Pointer(&src)))
}
