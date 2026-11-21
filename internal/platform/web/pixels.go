//go:build js && wasm

package web

import (
	goimage "image"
	"slices"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// pixelCanvas keeps the backing canvas and upload buffers across versions of
// an image. A canvas drawing uses the same upload path for its cached frames.
type pixelCanvas struct {
	el, ctx, data js.Value
	straight      []byte
	frame         *goimage.RGBA
	tint          driver.Color
}

func newPixelCanvas(el js.Value) *pixelCanvas {
	return &pixelCanvas{el: el, ctx: el.Call("getContext", "2d")}
}

// put writes the core's dense, premultiplied copy as straight ImageData.
func (c *pixelCanvas) put(p *goimage.RGBA, tint driver.Color) {
	if c.frame == p && c.tint == tint {
		return
	}
	w, h := p.Rect.Dx(), p.Rect.Dy()
	if c.frame == nil || c.frame.Rect.Size() != p.Rect.Size() {
		c.el.Set("width", w)
		c.el.Set("height", h)
		if w > 0 && h > 0 {
			c.data = c.ctx.Call("createImageData", w, h)
			c.straight = make([]byte, w*h*4)
		}
	}
	c.frame, c.tint = p, tint
	if w <= 0 || h <= 0 {
		return
	}
	straight := c.straight
	if tint == 0 {
		copy(straight, p.Pix)
		for i := 0; i < len(straight); i += 4 {
			a := uint32(straight[i+3])
			if a == 0 || a == 255 {
				continue
			}
			for channel := range 3 {
				straight[i+channel] = byte(min(uint32(straight[i+channel])*255/a, 255))
			}
		}
	} else {
		r, g, b, a := byte(tint>>24), byte(tint>>16), byte(tint>>8), uint16(byte(tint))
		for i := 0; i < len(straight); i += 4 {
			straight[i], straight[i+1], straight[i+2] = r, g, b
			straight[i+3] = byte((uint16(p.Pix[i+3])*a + 127) / 255)
		}
	}
	js.CopyBytesToJS(c.data.Get("data"), straight)
	c.ctx.Call("putImageData", c.data, 0, 0)
}

// maxCanvasFrames and maxCanvasFramePixels bound the canvases a surface keeps of the pixels its canvases draw, by count and by their pixels together (8 Mi, four 1920x1080 frames): a stream's older versions are never drawn again.
const (
	maxCanvasFrames      = 8
	maxCanvasFramePixels = 1 << 23
)

func pixelCount(p *goimage.RGBA) int { return p.Rect.Dx() * p.Rect.Dy() }

type canvasFrame struct {
	pixels *goimage.RGBA
	image  *canvasImage
}

// canvasFrame returns src's pixels in a canvas of their own, made at once. The surface keeps the latest few, by the core's copy, the most recently drawn last: a canvas that draws again at the same versions makes none anew, and the rest go, as each version is a copy of its own.
func (s *surface) canvasFrame(src driver.ImageSource) *canvasImage {
	for i, f := range s.canvasFrames {
		if f.pixels == src.Pixels {
			s.canvasFrames = append(append(s.canvasFrames[:i:i], s.canvasFrames[i+1:]...), f)
			return f.image
		}
	}
	el := s.doc.Call("createElement", "canvas")
	newPixelCanvas(el).put(src.Pixels, 0)
	im := &canvasImage{img: el, density: 1, loaded: true}
	for len(s.canvasFrames) > 0 && (len(s.canvasFrames) >= maxCanvasFrames || s.canvasFramePixels+pixelCount(src.Pixels) > maxCanvasFramePixels) {
		s.canvasFramePixels -= pixelCount(s.canvasFrames[0].pixels)
		s.canvasFrames = slices.Delete(s.canvasFrames, 0, 1)
	}
	s.canvasFramePixels += pixelCount(src.Pixels)
	s.canvasFrames = append(s.canvasFrames, canvasFrame{src.Pixels, im})
	return im
}
