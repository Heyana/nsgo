//go:build linux && !android

package linux

import (
	"image"
	"image/color"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestCanvasVerticesSampleOffscreenAndPixels draws a triangle, which takes the rasterizer rather than the image op, from an offscreen drawing and from a pixel buffer, and checks the color each one samples.
func TestCanvasVerticesSampleOffscreenAndPixels(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	pixels := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			pixels.SetRGBA(x, y, color.RGBA{B: 255, A: 255})
		}
	}
	offscreen := &driver.OffscreenCanvas{ID: 1, Version: 1, Size: driver.Size{Width: 4, Height: 4}, Scale: 2,
		Drawing: driver.NewDrawing(nil).SetFillColor(driver.RGB(0xff0000)).FillRect(0, 0, 4, 4)}
	for _, tc := range []struct {
		name string
		src  driver.ImageSource
		want color.RGBA
	}{
		{"offscreen", driver.ImageSource{Offscreen: offscreen}, color.RGBA{R: 255, A: 255}},
		{"pixels", driver.ImageSource{Pixels: pixels, Version: 1}, color.RGBA{B: 255, A: 255}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &window{animator: driver.NewAnimator(nil), env: driver.Environment{PixelRatio: 1}}
			vertex := func(x, y, u, v driver.Points) driver.CanvasVertex {
				return driver.CanvasVertex{Position: driver.Point{X: x, Y: y}, TexCoord: driver.Point{X: u, Y: v}}
			}
			d := driver.NewDrawing(nil).DrawVertices(tc.src, []driver.CanvasVertex{
				vertex(0, 0, 0, 0), vertex(32, 0, 4, 0), vertex(0, 32, 0, 4),
			})
			const size = 32
			surface := cairoImageSurfaceCreate(0, size, size)
			defer cairoSurfaceDestroy(surface)
			cr := cairoCreate(surface)
			w.playCanvas(&view{drawing: d}, cr, rect{w: size, h: size})
			cairoDestroy(cr)
			cairoSurfaceFlush(surface)
			data, stride := cairoImageSurfaceGetData(surface), int(cairoImageSurfaceGetStride(surface))
			rows := unsafe.Slice((*byte)(cPointer(data)), stride*size)
			at := func(x, y int) color.RGBA {
				b := rows[y*stride+x*4:]
				return color.RGBA{R: b[2], G: b[1], B: b[0], A: b[3]}
			}
			if got := at(6, 6); got != tc.want {
				t.Errorf("inside the triangle = %v, want %v", got, tc.want)
			}
			if got := at(28, 28); got.A != 0 {
				t.Errorf("outside the triangle = %v, want transparent", got)
			}
		})
	}
}
