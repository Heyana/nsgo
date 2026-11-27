//go:build windows

package windows

import (
	"math"

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
	imgSrc := c.drawing.Image(op)
	if imgSrc == nil {
		return
	}
	if draw, ok := op.ImageRect(); ok {
		c.image(&draw)
		return
	}
	var texture canvasmesh.Texture
	switch src := *imgSrc; {
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
		slot := c.p.canvasImage(src)
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
		e.meshPixels = bitmapPixels(p.dc, e.bitmap, uint32(pw), uint32(ph), e.scale)
	}
	return canvasmesh.Texture{Pix: e.meshPixels, Width: pw, Height: ph, Stride: pw * 4, Points: e.points, BGRA: true}
}
