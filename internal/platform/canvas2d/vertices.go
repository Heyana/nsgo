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
	// Without rotation or skew, the raster's pixels are the canvas's, so compositing it copies them rather than resampling them.
	dst, aligned := canvasmesh.Bounds(op, tex, p.Ratio), false
	if p.Ctx.Get("getTransform").Type() == js.TypeFunction {
		m := p.Ctx.Call("getTransform")
		a, b, c, d := m.Get("a").Float(), m.Get("b").Float(), m.Get("c").Float(), m.Get("d").Float()
		dst = canvasmesh.Bounds(op, tex, max(math.Hypot(a, b), math.Hypot(c, d)))
		if b == 0 && c == 0 {
			dst, aligned = canvasmesh.Device(op, tex, a, d, m.Get("e").Float(), m.Get("f").Float())
		}
	}
	if dst.Width == 0 {
		return
	}
	pix := image.NewRGBA(image.Rect(0, 0, dst.Width, dst.Height))
	dst.Pix, dst.Stride = pix.Pix, pix.Stride
	canvasmesh.Draw(dst, op, tex)
	c := p.MeshCanvas()
	if !WritePixels(c, pix) {
		return
	}
	p.Ctx.Set("imageSmoothingEnabled", !aligned && op.Sampling != driver.ImageSamplingNearest)
	p.Ctx.Call("drawImage", c, dst.X, dst.Y, float64(dst.Width)/dst.ScaleX, float64(dst.Height)/dst.ScaleY)
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
// the straight alpha of ImageData. It reports false for a context
// without ImageData, as some mini program runtimes' are.
func WritePixels(canvas js.Value, pix *image.RGBA) bool {
	w, h := pix.Rect.Dx(), pix.Rect.Dy()
	canvas.Set("width", w)
	canvas.Set("height", h)
	ctx := canvas.Call("getContext", "2d")
	if w == 0 || h == 0 || ctx.Get("createImageData").Type() != js.TypeFunction || ctx.Get("putImageData").Type() != js.TypeFunction {
		return false
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
	data := ctx.Call("createImageData", w, h)
	js.CopyBytesToJS(data.Get("data"), straight)
	ctx.Call("putImageData", data, 0, 0)
	return true
}
