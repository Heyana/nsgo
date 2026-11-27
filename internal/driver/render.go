package driver

import (
	"errors"
	"image"
	"math"
)

// NoDrawingRenderer is how a platform without offscreen drawing rasterization reports it.
const NoDrawingRenderer = "this platform cannot render drawings off the screen"

// MaxRenderPixels bounds one offscreen rendered drawing (16 Mi pixels = 64 MiB).
const MaxRenderPixels = 1 << 24

// RenderDrawingRequest is an ItemRenderDrawing's parameters.
type RenderDrawingRequest struct {
	Drawing *Drawing
	Size    Size
	Scale   float64
}

// RenderPixelSize checks d, size and scale for offscreen drawing rendering and
// returns the pixel dimensions.
func RenderPixelSize(d *Drawing, size Size, scale float64) (width, height int, err error) {
	if d == nil {
		return 0, 0, errors.New("no drawing")
	}
	if len(d.Slots) != 0 || len(d.ViewPaints) != 0 {
		return 0, 0, errors.New("drawing still depends on a view's animations or paints")
	}
	w, h := math.Ceil(float64(size.Width)*scale), math.Ceil(float64(size.Height)*scale)
	if !(scale > 0) || math.IsInf(scale, 0) || !(w > 0) || !(h > 0) || w*h > MaxRenderPixels {
		return 0, 0, errors.New("render size and scale must be finite, positive and at most 16 Mi pixels")
	}
	return int(w), int(h), nil
}

// RenderDrawingResult answers an ItemRenderDrawing request with the rendered image or an error.
func RenderDrawingResult(count uint32, img *image.RGBA, err error) Event {
	if err != nil {
		return Event{Kind: EventFiles, Count: count, Text: err.Error()}
	}
	return Event{Kind: EventFiles, Count: count, Data: img, Finished: true}
}

// BGRAtoRGBA copies premultiplied BGRA rows with the given byte stride into a
// new owned premultiplied *image.RGBA.
func BGRAtoRGBA(rows []byte, width, height, stride int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		src := rows[y*stride : y*stride+width*4]
		dst := out.Pix[y*out.Stride : y*out.Stride+width*4]
		for x := 0; x < width*4; x += 4 {
			dst[x+0] = src[x+2]
			dst[x+1] = src[x+1]
			dst[x+2] = src[x+0]
			dst[x+3] = src[x+3]
		}
	}
	return out
}
