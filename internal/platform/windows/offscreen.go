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
	// meshPixels is bitmap's BGRA pixels, read back the first time a mesh samples this version.
	meshPixels []byte
	// width and height are bitmap's size in pixels.
	width, height uint32
	// canvasTint is bitmap with its channels multiplied by canvasTintColor, for a tinted canvas draw.
	canvasTint      *comObject
	canvasTintColor driver.Color
}

func (e *offscreenEntry) release() {
	if e != nil {
		releaseAll(&e.bitmap, &e.canvasTint)
	}
}

// tinted returns the bitmap op draws: the entry's own, or one copy of it multiplied by op's tint, read back once through a CPU-readable bitmap.
func (e *offscreenEntry) tinted(p *painter, op *driver.DrawOp) *comObject {
	if !op.Tinted || op.Color == 0xffffffff {
		return e.bitmap
	}
	if e.canvasTint != nil && e.canvasTintColor == op.Color {
		return e.canvasTint
	}
	releaseAll(&e.canvasTint)
	w, h := e.width, e.height
	props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied, dpiX: 96 * e.scale, dpiY: 96 * e.scale,
		options: d2dBitmapCPURead | d2dBitmapCannotDraw}
	var readable *comObject
	if failed(p.dc.callSize(dcCreateBitmap, w, h, 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&readable)))) {
		return nil
	}
	defer readable.release()
	p.dc.call(rtFlush, 0, 0)
	if failed(readable.call(bitmapCopyFromBitmap, 0, e.bitmap.ptr(), 0)) {
		return nil
	}
	var mapped struct {
		pitch uint32
		bits  *byte
	}
	if failed(readable.call(bitmapMap, d2dMapRead, uintptr(unsafe.Pointer(&mapped)))) {
		return nil
	}
	row := int(w) * 4
	pix := make([]byte, row*int(h))
	rows := unsafe.Slice(mapped.bits, int(mapped.pitch)*int(h))
	for y := range int(h) {
		at := y * int(mapped.pitch)
		driver.TintImagePixels(pix[y*row:(y+1)*row], rows[at:at+row], op.Color, true, true)
	}
	readable.call(bitmapUnmap)
	props.options = 0
	var bmp *comObject
	if failed(p.dc.callSize(dcCreateBitmap, w, h, uintptr(unsafe.Pointer(&pix[0])), uintptr(row), uintptr(unsafe.Pointer(&props)),
		uintptr(unsafe.Pointer(&bmp)))) {
		return nil
	}
	e.canvasTint, e.canvasTintColor = bmp, op.Color
	return bmp
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

	waited := false
	var dc *comObject
	if failed(p.g.device.call(d2dDeviceCreateDeviceContext, 0, uintptr(unsafe.Pointer(&dc)))) {
		bmp.release()
		return nil
	}
	defer dc.release()
	dc.call(dcSetTarget, bmp.ptr())
	dc.callF(rtSetDpi, 0b11, f32(96*scale), f32(96*scale))
	dc.call(rtBeginDraw)
	op := &painter{w: p.w, g: p.g, dc: dc, scale: scale, m: identity, alpha: 1, brush: p.brush}
	op.setTransform(identity)
	clearCol := colorF{}
	dc.call(rtClear, uintptr(unsafe.Pointer(&clearCol)))
	c := op.newCanvasPlayer(v, identity, float32(off.Size.Width), float32(off.Size.Height))
	c.off = bmp
	if off.Drawing != nil {
		c.playAll(off.Drawing.Ops)
	}
	waited = c.waited
	for len(op.pushed) > 0 {
		op.pop()
	}
	hr := dc.call(rtEndDraw, 0, 0)
	dc.call(dcSetTarget, 0)
	if failed(hr) {
		bmp.release()
		return nil
	}

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
		width:      uint32(pw),
		height:     uint32(ph),
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
