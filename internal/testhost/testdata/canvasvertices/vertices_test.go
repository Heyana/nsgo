package canvasvertices

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestCanvasVerticesMatchImageCrop(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 4, 4))
	pixels.SetRGBA(1, 1, color.RGBA{R: 255, A: 255})
	pixels.SetRGBA(2, 1, color.RGBA{G: 255, A: 255})
	pixels.SetRGBA(1, 2, color.RGBA{B: 255, A: 255})
	pixels.SetRGBA(2, 2, color.RGBA{R: 255, G: 255, A: 255})
	src := asSource(t, pixels)
	var useMesh *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		useMesh = core.NewSignal(cx, false)
		return core.Canvas{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}, Draw: func(d *core.Drawing, size core.Size) {
			d.SetFillColor(core.RGB(0xffffff)).FillRect(0, 0, float32(size.Width), float32(size.Height))
			if !useMesh.Get() {
				d.DrawImageFrom(src, 1, 1, 2, 2, 16, 16, 64, 64)
				return
			}
			a := vert(16, 16, 1, 1, 0)
			b := vert(80, 16, 3, 1, 0)
			c := vert(80, 80, 3, 3, 0)
			e := vert(16, 80, 1, 3, 0)
			d.DrawVertices(src, []core.CanvasVertex{a, b, c, a, c, e})
		}}
	}, nsgotest.Size(128, 128))
	ref := awaitPixels(t, app, func(r, g, b uint32) bool { return r > 0xe000 && g < 0x2000 && b < 0x2000 })
	app.Post(func() { useMesh.Set(true) })
	got := awaitPixels(t, app, func(r, g, b uint32) bool { return r > 0xe000 && g < 0x2000 && b < 0x2000 })
	if got.Bounds() != ref.Bounds() {
		t.Fatalf("mesh capture has bounds %v, want %v", got.Bounds(), ref.Bounds())
	}
	for y := got.Bounds().Min.Y; y < got.Bounds().Max.Y; y++ {
		for x := got.Bounds().Min.X; x < got.Bounds().Max.X; x++ {
			if got.At(x, y) != ref.At(x, y) {
				t.Fatalf("mesh differs from DrawImageFrom at (%d, %d): %v vs %v", x, y, got.At(x, y), ref.At(x, y))
			}
		}
	}
}

func TestCanvasVerticesInterpolateOpaqueColors(t *testing.T) {
	// A light-blue texture changes the expected red and green channels,
	// so a flat fill and an untinted texture both fail this probe.
	pixels := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		for x := range 2 {
			pixels.SetRGBA(x, y, color.RGBA{R: 128, G: 192, B: 255, A: 255})
		}
	}
	src := asSource(t, pixels)
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.Canvas{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100)}, Draw: func(d *core.Drawing, size core.Size) {
			d.SetFillColor(core.RGB(0xffffff)).FillRect(0, 0, float32(size.Width), float32(size.Height))
			d.Save().Translate(10, 10).DrawVertices(src, []core.CanvasVertex{
				vert(0, 0, 0, 0, core.RGB(0xff0000)),
				vert(72, 0, 2, 0, core.RGB(0x00ff00)),
				vert(0, 72, 0, 2, core.RGB(0x0000ff)),
			}).Restore()
		}}
	}, nsgotest.Size(128, 128))
	awaitPixels(t, app, func(r, g, b uint32) bool {
		return r > 0x2000 && r < 0x3500 && g > 0x3200 && g < 0x5000 && b > 0x4400 && b < 0x6700
	})
}

func vert(x, y, u, v core.Points, tint core.Color) core.CanvasVertex {
	return core.CanvasVertex{Position: core.Point{X: x, Y: y}, TexCoord: core.Point{X: u, Y: v}, Color: tint}
}

func asSource(t *testing.T, pixels *image.RGBA) core.ImageSource {
	t.Helper()
	var data bytes.Buffer
	if err := png.Encode(&data, pixels); err != nil {
		t.Fatal(err)
	}
	return core.ImageSource{Data: data.Bytes()}
}

func awaitPixels(t *testing.T, app *nsgotest.App, match func(uint32, uint32, uint32) bool) image.Image {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; {
		img := app.Capture()
		b := img.Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				r, g, b, a := img.At(x, y).RGBA()
				if a >= 0xff00 && match(r, g, b) {
					return img
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no expected pixels in %v", b)
		}
		app.Advance(20 * time.Millisecond)
	}
}
