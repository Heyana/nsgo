//go:build darwin

package apple

import (
	"image/color"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
)

// TestCanvasVerticesSampleAnOffscreenImage draws a triangle, which takes the rasterizer rather than the image op, from an offscreen drawing whose top half is red and bottom half blue, and checks both halves land upright.
func TestCanvasVerticesSampleAnOffscreenImage(t *testing.T) {
	const id uint64 = 987654322
	defer ReleaseOffscreen(id)
	off := &driver.OffscreenCanvas{ID: id, Version: 1, Size: driver.Size{Width: 40, Height: 20}, Scale: 2,
		Drawing: driver.NewDrawing(nil).
			SetFillColor(driver.RGB(0xff0000)).FillRect(0, 0, 40, 10).
			SetFillColor(driver.RGB(0x0000ff)).FillRect(0, 10, 40, 10)}
	vertex := func(x, y, u, v driver.Points) driver.CanvasVertex {
		return driver.CanvasVertex{Position: driver.Point{X: x, Y: y}, TexCoord: driver.Point{X: u, Y: v}}
	}
	d := driver.NewDrawing(nil).DrawVertices(driver.ImageSource{Offscreen: off}, []driver.CanvasVertex{
		vertex(0, 0, 0, 0), vertex(40, 0, 40, 0), vertex(0, 40, 0, 20),
	})
	for _, flipped := range []bool{false, true} {
		img := play(t, "vertices-offscreen", d, 40, 40, flipped)
		at := func(x, y int) color.RGBA { return img.RGBAAt(x*canvastest.Scale, y*canvastest.Scale) }
		if got := at(5, 5); got != (color.RGBA{R: 0xff, A: 0xff}) {
			t.Errorf("flipped=%v: top of the triangle = %v, want red", flipped, got)
		}
		if got := at(5, 30); got != (color.RGBA{B: 0xff, A: 0xff}) {
			t.Errorf("flipped=%v: bottom of the triangle = %v, want blue", flipped, got)
		}
		if got := at(35, 35); got != (color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}) {
			t.Errorf("flipped=%v: outside the triangle = %v, want the white background", flipped, got)
		}
	}
}
