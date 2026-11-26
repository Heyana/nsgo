//go:build js && wasm

package web

import (
	goimage "image"
	"image/color"
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

// stretchedCanvas returns v's <canvas> drawn over side × side pixels, smoothly unless its image-rendering is pixelated, as the browser stretches it to its box.
func stretchedCanvas(s *surface, v *view, side int) *goimage.RGBA {
	c := s.doc.Call("createElement", "canvas")
	c.Set("width", side)
	c.Set("height", side)
	ctx := c.Call("getContext", "2d")
	ctx.Set("imageSmoothingEnabled", v.inner.Get("style").Get("imageRendering").String() != "pixelated")
	ctx.Call("drawImage", v.inner, 0, 0, side, side)
	img := goimage.NewRGBA(goimage.Rect(0, 0, side, side))
	data := ctx.Call("getImageData", 0, 0, side, side).Get("data")
	js.CopyBytesToGo(img.Pix, js.Global().Get("Uint8Array").New(data.Get("buffer")))
	return img
}

func TestCanvasResolutionScale(t *testing.T) {
	for _, sampling := range []driver.ImageSampling{driver.ImageSamplingSmooth, driver.ImageSamplingNearest} {
		t.Run(map[driver.ImageSampling]string{driver.ImageSamplingSmooth: "smooth", driver.ImageSamplingNearest: "nearest"}[sampling], func(t *testing.T) {
			s, _ := canvasZoomSurface(t)
			const side = canvastest.ResolutionSize
			v := mountCanvas(s, 6, 7, 1, side, side)
			d := canvastest.Resolution(nil, sampling)
			s.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemCanvas, ID: 7, Drawing: d}}})
			// Half the screen's 2 pixels a point: one pixel a square.
			waitCanvasPixels(t, v, canvastest.ResolutionPixels)
			if css := v.inner.Get("style").Get("width").String(); css != "100%" && css != "" {
				t.Errorf("<canvas> CSS width = %q, want its box's", css)
			}
			backing := canvasPixels(v)
			for y := range side {
				for x := range side {
					want := color.RGBA{A: 255}
					if (x+y)%2 == 1 {
						want = color.RGBA{255, 255, 255, 255}
					}
					if got := backing.RGBAAt(x, y); got != want {
						t.Fatalf("backing pixel (%d, %d) = %v, want %v", x, y, got, want)
					}
				}
			}
			want := ""
			if sampling == driver.ImageSamplingNearest {
				want = "pixelated"
			}
			if got := v.inner.Get("style").Get("imageRendering").String(); got != want {
				t.Errorf("image-rendering = %q, want %q", got, want)
			}
			for _, f := range canvastest.CheckResolution(stretchedCanvas(s, v, side*canvastest.Scale), goimage.Point{}, sampling) {
				t.Error(f)
			}

			// The factor multiplies a transform's raster step rather than replacing it.
			s.mount(&driver.MountBatch{Items: []driver.MountItem{
				{Kind: driver.ItemViewStyle, ID: 6, ViewStyle: &driver.ViewStyle{Opacity: 1, Transform: driver.Transform{Scale: 2}}},
			}})
			waitCanvasPixels(t, v, side*2)
			if got := v.inner.Get("style").Get("imageRendering").String(); got != want {
				t.Errorf("zoomed image-rendering = %q, want %q", got, want)
			}

			// A scale of 1 plays the same ops at the screen's density again, pixel for pixel under the zoom.
			full := *d
			full.Resolution = 1
			s.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemCanvas, ID: 7, Drawing: &full}}})
			waitCanvasPixels(t, v, side*4)
			if got := v.inner.Get("style").Get("imageRendering").String(); got != "pixelated" || !v.canvasExact {
				t.Errorf("full-density zoomed image-rendering = %q (exact %v), want the exact zoom's pixelated", got, v.canvasExact)
			}
			s.mount(&driver.MountBatch{Items: []driver.MountItem{
				{Kind: driver.ItemViewStyle, ID: 6, ViewStyle: &driver.ViewStyle{Opacity: 1}},
			}})
			waitCanvasPixels(t, v, side*2)
			if got := v.inner.Get("style").Get("imageRendering").String(); got != "" {
				t.Errorf("full-density image-rendering = %q, want the default", got)
			}
		})
	}
}
