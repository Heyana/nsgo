//go:build linux && !android

package linux

import (
	"math"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvasmesh"
)

func (p *canvasPlayer) vertices(op *driver.DrawOp) {
	imgSrc := p.drawing.Image(op)
	if imgSrc == nil {
		return
	}
	if draw, ok := op.ImageRect(); ok {
		p.image(&draw)
		return
	}
	var texture canvasmesh.Texture
	switch src := *imgSrc; {
	case src.Pixels != nil:
		texture = canvasmesh.RGBA(src.Pixels, src.Size)
	case src.Offscreen != nil:
		e := p.w.offscreenSurface(p.v, src.Offscreen)
		if e == nil || e.surface == 0 {
			return
		}
		data, stride := cairoImageSurfaceGetData(e.surface), int(cairoImageSurfaceGetStride(e.surface))
		if data == 0 || stride < e.width*4 {
			return
		}
		texture = canvasmesh.Texture{Pix: unsafe.Slice((*byte)(cPointer(data)), stride*e.height), Width: e.width, Height: e.height,
			Stride: stride, Points: offscreenPoints(src, e), BGRA: true}
	default:
		slot := p.w.canvasImage(src, p.v)
		if slot == nil || slot.pic == nil {
			p.waited = p.waited || (slot != nil && slot.load != nil)
			return
		}
		pic := slot.pic
		texture = canvasmesh.Texture{Pix: pic.pix, Width: pic.width, Height: pic.height,
			Stride: pic.width * 4, Points: natural(slot), BGRA: true}
	}
	var m cairoMatrix
	cairoGetMatrix(p.cr, &m)
	pix := canvasmesh.Raster(op, texture, max(math.Hypot(m.xx, m.yx), math.Hypot(m.xy, m.yy)))
	if pix == nil {
		return
	}
	var slot imageSlot
	slot.showPixels(driver.ImageSource{Pixels: pix})
	defer slot.reset(driver.ImageSource{})
	p.drawImage(op, slot.surfaceOf(0), slot.pic.width, driver.Size{Width: driver.Points(op.W), Height: driver.Points(op.H)})
}
