package canvasmesh

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func vertexAt(x, y, u, v driver.Points, tint driver.Color) driver.CanvasVertex {
	return driver.CanvasVertex{Position: driver.Point{X: x, Y: y}, TexCoord: driver.Point{X: u, Y: v}, Color: tint}
}

func TestRasterSharesTheDiagonal(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 1, 1))
	halfRed := color.RGBA{R: 128, A: 128}
	pixels.SetRGBA(0, 0, halfRed)
	a, b, c, e := vertexAt(0, 0, 0, 0, 0), vertexAt(4, 0, 1, 0, 0), vertexAt(4, 4, 1, 1, 0), vertexAt(0, 4, 0, 1, 0)
	src := driver.ImageSource{Pixels: pixels}
	// Opposite winding exercises the coverage tie on the same diagonal.
	op := driver.NewDrawing(nil).DrawVertices(src, []driver.CanvasVertex{a, b, c, a, e, c}).Ops[0]
	out := Raster(&op, RGBA(pixels, driver.Size{}), 2)
	for y := range out.Rect.Dy() {
		for x := range out.Rect.Dx() {
			if c := out.RGBAAt(x, y); c != halfRed {
				t.Errorf("pixel %d,%d = %v, want %v", x, y, c, halfRed)
			}
		}
	}
}

func TestRasterMultipliesInterpolatedVertexColors(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixels.SetRGBA(0, 0, color.RGBA{R: 128, G: 192, B: 255, A: 255})
	vertices := []driver.CanvasVertex{
		vertexAt(0, 0, 0, 0, driver.RGB(0xff0000)),
		vertexAt(8, 0, 1, 0, driver.RGB(0x00ff00)),
		vertexAt(0, 8, 0, 1, driver.RGB(0x0000ff)),
	}
	op := driver.NewDrawing(nil).DrawVertices(driver.ImageSource{Pixels: pixels}, vertices).Ops[0]
	out := Raster(&op, RGBA(pixels, driver.Size{}), 1)
	if c, want := out.RGBAAt(0, 0), (color.RGBA{R: 112, G: 12, B: 16, A: 255}); c != want {
		t.Errorf("interpolated tint = %v, want %v", c, want)
	}
	if c := out.RGBAAt(7, 7); c != (color.RGBA{}) {
		t.Errorf("pixel outside the triangle = %v", c)
	}
}

func BenchmarkDeformingMesh(b *testing.B) {
	pixels := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			pixels.SetRGBA(x, y, color.RGBA{R: uint8(32 * x), B: uint8(32 * y), A: 255})
		}
	}
	tex := RGBA(pixels, driver.Size{})
	src := driver.ImageSource{Pixels: pixels}
	const columns, rows = 25, 20
	vertices := make([]driver.CanvasVertex, 0, columns*rows*6)
	for _, ratio := range []float64{1, 2} {
		b.Run(fmt.Sprintf("%gx", ratio), func(b *testing.B) {
			var frame float64
			for b.Loop() {
				frame += .01
				vertices = vertices[:0]
				for y := range rows {
					base := driver.Points(y * 14)
					dx := driver.Points(6 * math.Sin(frame+float64(y)/5))
					dxNext := driver.Points(6 * math.Sin(frame+float64(y+1)/5))
					for x := range columns {
						left := driver.Points(x * 20)
						a := vertexAt(left+dx, base, 0, 0, 0)
						c := vertexAt(left+20+dxNext, base+14, 8, 8, driver.RGB(0xddeeff))
						vertices = append(vertices, a, vertexAt(left+20+dx, base, 8, 0, 0), c,
							a, c, vertexAt(left+dxNext, base+14, 0, 8, 0))
					}
				}
				op := driver.NewDrawing(nil).DrawVertices(src, vertices).Ops[0]
				if Raster(&op, tex, ratio) == nil {
					b.Fatal("no raster")
				}
			}
		})
	}
}
