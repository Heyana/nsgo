//go:build windows

package windows

import (
	"math"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvasmesh"
)

const (
	bitmapMap        = 14
	bitmapUnmap      = 15
	d2dBitmapCPURead = 4
	d2dMapRead       = 1
)

func (c *canvasPlayer) vertices(op *driver.DrawOp) {
	if op.Image == nil {
		return
	}
	if draw, ok := op.ImageRect(); ok {
		c.image(&draw)
		return
	}
	var texture canvasmesh.Texture
	switch src := *op.Image; {
	case src.Pixels != nil:
		texture = canvasmesh.RGBA(src.Pixels, src.Size)
	case src.Offscreen != nil:
		e := c.p.offscreenBitmap(c.v, src.Offscreen)
		if e == nil || e.bitmap == nil {
			return
		}
		texture = c.p.offscreenTexture(e, src.Offscreen)
		if src.Size.Width > 0 && src.Size.Height > 0 {
			texture.Points = src.Size
		}
	default:
		slot := c.p.w.canvasImage(src)
		if slot == nil || slot.pic == nil || len(slot.pic.frames) == 0 {
			c.waited = c.waited || (slot != nil && slot.load != nil)
			return
		}
		pic := slot.pic
		texture = canvasmesh.Texture{Pix: pic.frames[0].Pix, Width: pic.width, Height: pic.height,
			Stride: pic.width * 4, Points: natural(slot), BGRA: true}
	}
	if len(texture.Pix) == 0 {
		return
	}
	m := c.st.ctm.then(c.world)
	scale := float64(c.p.scale) * max(math.Hypot(m.a, m.b), math.Hypot(m.c, m.d))
	pix := canvasmesh.Raster(op, texture, scale)
	if pix == nil {
		return
	}
	var slot imageSlot
	slot.showPixels(driver.ImageSource{Pixels: pix})
	defer slot.reset(driver.ImageSource{})
	c.drawImage(op, &slot)
}

// offscreenTexture returns e's pixels, which live on the GPU, copied once per version of off through a bitmap the CPU can read.
func (p *painter) offscreenTexture(e *offscreenEntry, off *driver.OffscreenCanvas) canvasmesh.Texture {
	pw, ph := off.PixelSize()
	if e.meshPixels == nil && pw > 0 && ph > 0 {
		props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied, dpiX: 96 * e.scale,
			dpiY: 96 * e.scale, options: d2dBitmapCPURead | d2dBitmapCannotDraw}
		var readable *comObject
		if failed(p.dc.callSize(dcCreateBitmap, uint32(pw), uint32(ph), 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&readable)))) {
			return canvasmesh.Texture{}
		}
		defer readable.release()
		p.dc.call(rtFlush, 0, 0)
		readable.call(bitmapCopyFromBitmap, 0, e.bitmap.ptr(), 0)
		var mapped struct {
			pitch uint32
			bits  *byte
		}
		if failed(readable.call(bitmapMap, d2dMapRead, uintptr(unsafe.Pointer(&mapped)))) {
			return canvasmesh.Texture{}
		}
		rows := unsafe.Slice(mapped.bits, int(mapped.pitch)*ph)
		pix := make([]byte, pw*ph*4)
		for y := range ph {
			copy(pix[y*pw*4:(y+1)*pw*4], rows[y*int(mapped.pitch):])
		}
		readable.call(bitmapUnmap)
		e.meshPixels = pix
	}
	return canvasmesh.Texture{Pix: e.meshPixels, Width: pw, Height: ph, Stride: pw * 4, Points: e.points, BGRA: true}
}
