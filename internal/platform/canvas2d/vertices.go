//go:build js && wasm

package canvas2d

import (
	"image"
	"math"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvasmesh"
)

func (p *Player) vertices(op *driver.DrawOp) {
	if op.Image == nil {
		return
	}
	if draw, ok := op.ImageRect(); ok {
		p.image(&draw)
		return
	}
	if p.MeshCanvas == nil || p.Texture == nil {
		return
	}
	tex := p.Texture(*op.Image)
	if len(tex.Pix) == 0 {
		return
	}
	scale := p.Ratio
	if p.Ctx.Get("getTransform").Type() == js.TypeFunction {
		m := p.Ctx.Call("getTransform")
		scale = max(math.Hypot(m.Get("a").Float(), m.Get("b").Float()), math.Hypot(m.Get("c").Float(), m.Get("d").Float()))
	}
	pix := canvasmesh.Raster(op, tex, scale)
	if pix != nil {
		c := p.MeshCanvas()
		WritePixels(c, pix)
		p.Ctx.Call("drawImage", c, float64(op.X), float64(op.Y), float64(op.W), float64(op.H))
	}
}

// ReadTexture snapshots a native image's premultiplied pixels. Canvas
// security rules may prevent reading an image loaded from another origin.
func ReadTexture(canvas, img js.Value, density float64) (tex canvasmesh.Texture) {
	defer func() {
		if recover() != nil {
			tex = canvasmesh.Texture{}
		}
	}()
	w, h := img.Get("width").Int(), img.Get("height").Int()
	if n := img.Get("naturalWidth"); n.Type() == js.TypeNumber {
		w, h = n.Int(), img.Get("naturalHeight").Int()
	}
	if w <= 0 || h <= 0 || density <= 0 {
		return tex
	}
	canvas.Set("width", w)
	canvas.Set("height", h)
	ctx := canvas.Call("getContext", "2d")
	ctx.Call("drawImage", img, 0, 0, w, h)
	raw := ctx.Call("getImageData", 0, 0, w, h).Get("data")
	data := make([]byte, w*h*4)
	js.CopyBytesToGo(data, raw)
	for i := 0; i < len(data); i += 4 {
		a := uint32(data[i+3])
		for c := range 3 {
			data[i+c] = byte((uint32(data[i+c])*a + 127) / 255)
		}
	}
	return canvasmesh.Texture{Pix: data, Width: w, Height: h, Stride: w * 4,
		Points: driver.Size{Width: driver.Points(float64(w) / density), Height: driver.Points(float64(h) / density)}}
}

// WritePixels replaces canvas with pix, converting premultiplied RGBA to
// the straight alpha of ImageData.
func WritePixels(canvas js.Value, pix *image.RGBA) {
	w, h := pix.Rect.Dx(), pix.Rect.Dy()
	canvas.Set("width", w)
	canvas.Set("height", h)
	if w == 0 || h == 0 {
		return
	}
	straight := make([]byte, w*h*4)
	for i := 0; i+3 < len(straight) && i+3 < len(pix.Pix); i += 4 {
		a := uint32(pix.Pix[i+3])
		straight[i+3] = byte(a)
		if a == 0 {
			continue
		}
		for c := range 3 {
			straight[i+c] = byte(min(uint32(pix.Pix[i+c])*255/a, 255))
		}
	}
	ctx := canvas.Call("getContext", "2d")
	data := ctx.Call("createImageData", w, h)
	js.CopyBytesToJS(data.Get("data"), straight)
	ctx.Call("putImageData", data, 0, 0)
}
