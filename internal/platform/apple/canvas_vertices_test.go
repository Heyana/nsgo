//go:build darwin

package apple

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"syscall"
	"testing"
	"time"

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

// TestCanvasVerticesMatchAnImageAtAnyOffset draws a quad as a fan of four triangles, which takes the rasterizer, at a fractional offset, and checks its pixels inside the edges against DrawImageFrom's nearest samples, flipped and not.
func TestCanvasVerticesMatchAnImageAtAnyOffset(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			pixels.SetRGBA(x, y, color.RGBA{R: uint8(60 * x), G: uint8(60 * y), B: 200, A: 255})
		}
	}
	src := driver.ImageSource{Pixels: pixels}
	vertex := func(x, y driver.Points) driver.CanvasVertex {
		return driver.CanvasVertex{Position: driver.Point{X: 3.3 + x, Y: 5.7 + y}, TexCoord: driver.Point{X: x / 16, Y: y / 16}}
	}
	a, b, c, e, mid := vertex(0, 0), vertex(64, 0), vertex(64, 64), vertex(0, 64), vertex(32, 32)
	mesh := driver.NewDrawing(nil).SetImageSmoothing(false).DrawVertices(src, []driver.CanvasVertex{a, b, mid, b, c, mid, c, e, mid, e, a, mid})
	crop := driver.NewDrawing(nil).SetImageSmoothing(false).DrawImageFrom(src, 0, 0, 4, 4, 3.3, 5.7, 64, 64)
	for _, flipped := range []bool{false, true} {
		got, want := play(t, "vertices-offset", mesh, 72, 72, flipped), play(t, "vertices-offset-image", crop, 72, 72, flipped)
		for y := 6*canvastest.Scale + 2; y < 69*canvastest.Scale-2; y++ {
			for x := 3*canvastest.Scale + 2; x < 67*canvastest.Scale-2; x++ {
				if got.RGBAAt(x, y) != want.RGBAAt(x, y) {
					t.Fatalf("flipped=%v: pixel %d,%d = %v, want %v", flipped, x, y, got.RGBAAt(x, y), want.RGBAAt(x, y))
				}
			}
		}
	}
}

// BenchmarkPlayDeformingMesh plays the #1102 canvas workload's mesh form: 1,000 triangles sampling a 1024 × 1024 atlas, deforming every frame over a 1200 × 800 point canvas at canvastest.Scale, as the throughput criterion of #1211 measures it.
func BenchmarkPlayDeformingMesh(b *testing.B) {
	const w, h, columns, rows = 1200, 800, 25, 20
	atlas := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	for y := range 1024 {
		for x := range 1024 {
			atlas.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x ^ y), A: 255})
		}
	}
	src := driver.ImageSource{Pixels: atlas}
	for _, tinted := range []bool{false, true} {
		b.Run(fmt.Sprintf("tinted=%v", tinted), func(b *testing.B) {
			// A BGRA context drawn top down, as a layer's backing store is.
			bitmapsLoaded.Do(loadBitmaps)
			const scale, premultipliedFirst, byteOrder32Little = canvastest.Scale, 2, 2 << 12
			ctx := cgBitmapContextCreate(0, w*scale, h*scale, 8, w*scale*4, srgb, premultipliedFirst|byteOrder32Little)
			defer Release(ctx)
			cgContextScaleCTM(ctx, scale, scale)
			vertices := make([]driver.CanvasVertex, 0, columns*rows*6)
			var frame float64
			cpu := processCPU()
			for b.Loop() {
				frame += .05
				vertices = vertices[:0]
				at := func(col, row int) driver.CanvasVertex {
					x, y := float64(col*w/columns), float64(row*h/rows)
					var tint driver.Color
					if tinted {
						tint = driver.RGB(0xddeeff)
					}
					return driver.CanvasVertex{
						Position: driver.Point{X: driver.Points(x + 8*math.Sin(frame+y/60)), Y: driver.Points(y + 8*math.Cos(frame+x/80))},
						TexCoord: driver.Point{X: driver.Points(col * 1024 / columns), Y: driver.Points(row * 1024 / rows)},
						Color:    tint,
					}
				}
				for row := range rows {
					for col := range columns {
						a, c := at(col, row), at(col+1, row+1)
						vertices = append(vertices, a, at(col+1, row), c, a, c, at(col, row+1))
					}
				}
				d := driver.NewDrawing(nil).DrawVertices(src, vertices)
				DrawTopDown(ctx, h, true, func(baseUp bool) {
					Play(ctx, d, d.Ops, CanvasHost{Ratio: scale, BaseUp: baseUp})
				})
			}
			// The CPU time of every thread, which a loaded machine inflates less than the wall time.
			b.ReportMetric(float64(processCPU()-cpu)/float64(b.N), "cpu-ns/op")
			b.ReportMetric(float64(len(vertices)/3), "triangles")
		})
	}
}

func processCPU() time.Duration {
	var u syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &u)
	return time.Duration(u.Utime.Nano() + u.Stime.Nano())
}
