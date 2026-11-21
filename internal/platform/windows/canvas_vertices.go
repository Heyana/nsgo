//go:build windows

package windows

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvasmesh"
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
	if src := *op.Image; src.Pixels != nil {
		texture = canvasmesh.RGBA(src.Pixels, src.Size)
	} else {
		slot := c.p.w.canvasImage(src)
		if slot == nil || slot.pic == nil || len(slot.pic.frames) == 0 {
			return
		}
		pic := slot.pic
		texture = canvasmesh.Texture{Pix: pic.frames[0].Pix, Width: pic.width, Height: pic.height,
			Stride: pic.width * 4, Points: natural(slot), BGRA: true}
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
