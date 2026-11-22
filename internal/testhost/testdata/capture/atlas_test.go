package capture_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestCanvasAtlasMatchesDrawImageFrom(t *testing.T) {
	t.Run("smooth", func(t *testing.T) { checkCanvasAtlasMatchesDrawImageFrom(t, true) })
	t.Run("nearest", func(t *testing.T) { checkCanvasAtlasMatchesDrawImageFrom(t, false) })
}

func checkCanvasAtlasMatchesDrawImageFrom(t *testing.T, smoothing bool) {
	if nsgotest.Target() == "" {
		t.Skip("checks the native canvas pixels")
	}
	tile := image.NewNRGBA(image.Rect(0, 0, 48, 24))
	for y := range 24 {
		for x := range 48 {
			ink := color.NRGBA{R: 240, G: 28 + uint8(y*4), B: uint8(x * 3), A: 255}
			if x >= 24 {
				ink = color.NRGBA{R: 30, G: uint8(x * 3), B: 230 - uint8(y*3), A: 220}
			}
			tile.SetNRGBA(x, y, ink)
		}
	}
	var data bytes.Buffer
	if err := png.Encode(&data, tile); err != nil {
		t.Fatal(err)
	}
	t.Run("encoded", func(t *testing.T) {
		src := core.ImageSource{Data: data.Bytes()}
		checkAtlas(t, smoothing, func(*core.Scope) func() core.ImageSource { return func() core.ImageSource { return src } })
	})
	t.Run("offscreen", func(t *testing.T) {
		if target := nsgotest.Target(); strings.HasPrefix(target, "android") || target == "harmonyos" {
			t.Skip("the target's canvases draw no offscreen images")
		}
		checkAtlas(t, smoothing, func(cx *core.Scope) func() core.ImageSource {
			return cx.NewOffscreenImage(core.Size{Width: 48, Height: 24}, 0, func(d *core.Drawing) {
				d.SetFillColor(core.RGB(0xf01c00)).FillRect(0, 0, 24, 24)
				d.SetFillColor(core.RGB(0xf0d040)).FillRect(0, 0, 24, 5)
				d.SetFillColor(core.RGB(0x1e30e6)).FillRect(24, 0, 24, 24)
				d.SetFillColor(core.RGB(0x30e060)).FillRect(40, 0, 8, 24)
			}).Source
		})
	})
}

// checkAtlas compares DrawAtlas with the DrawImageFrom calls it stands for. The function source returns is read on each draw for the image the tiles come from.
func checkAtlas(t *testing.T, smoothing bool, source func(cx *core.Scope) func() core.ImageSource) {
	left := core.Bounds{Size: core.Size{Width: 24, Height: 24}}
	right := left
	right.Origin.X = 24
	rects := []core.Bounds{left, right, left, right, left}
	xforms := []core.RSXform{
		{SCos: 1, TX: 8, TY: 8},
		{SCos: 0.5, TX: 24, TY: 16},
		{SCos: 2, TX: 56, TY: 16},
		{SCos: 0.8, SSin: 0.6, TX: 28, TY: 42},
		{SCos: 0.8, TX: 72.2, TY: 65.2},
	}
	alphas := []float32{1, 0.6, 0.75, 0.8, 0.75}
	var useAtlas *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		useAtlas = core.NewSignal(cx, false)
		read := source(cx)
		return core.Canvas{
			TestID: "atlas",
			Style:  core.Style{Width: core.Pct(100), Height: core.Pct(100)},
			Draw: func(d *core.Drawing, _ core.Size) {
				atlas := useAtlas.Get()
				d.SetImageSmoothing(smoothing)
				src := read()
				d.SetFillColor(core.RGB(0x162328)).FillRect(0, 0, 128, 96)
				d.BeginPath().Rect(8, 72, 24, 16)
				d.SetAlpha(0.8)
				if atlas {
					d.DrawAtlas(src, rects[:3], xforms[:3], alphas[:3])
					d.DrawAtlas(src, rects[3:4], xforms[3:4], alphas[3:4])
					d.DrawAtlas(src, rects[4:], xforms[4:], alphas[4:])
				} else {
					for i, r := range rects {
						x, y, w, h := float32(r.Origin.X), float32(r.Origin.Y), float32(r.Size.Width), float32(r.Size.Height)
						transform := xforms[i]
						c, s := transform.SCos, transform.SSin
						d.Save().SetAlpha(0.8 * alphas[i])
						if s == 0 {
							d.DrawImageFrom(src, x, y, w, h, transform.TX, transform.TY, w*c, h*c)
						} else {
							d.Transform(c, s, -s, c, transform.TX, transform.TY)
							d.DrawImageFrom(src, x, y, w, h, 0, 0, w, h)
						}
						d.Restore()
					}
				}
				d.SetAlpha(1).SetFillColor(core.RGB(0x40dc08)).Fill()
				d.SetFillColor(core.RGB(0xffffff))
				if !atlas {
					d.SetFillColor(core.RGB(0))
				}
				d.FillRect(112, 8, 16, 16)
			},
		}
	}, nsgotest.Size(144, 112))
	size := app.Find("id:atlas").Bounds().Size
	point := func(img image.Image, x, y float64) image.Point {
		b := img.Bounds()
		return image.Pt(b.Min.X+int(x*float64(b.Dx())/float64(size.Width)),
			b.Min.Y+int(y*float64(b.Dy())/float64(size.Height)))
	}
	var want, got image.Image
	app.WaitUntil(5*time.Second, func() bool {
		want = app.Capture()
		p := point(want, 16, 16)
		r, g, b, _ := want.At(p.X, p.Y).RGBA()
		return r>>8 >= 150 && g>>8 < 100 && b>>8 < 100
	})
	app.Post(func() { useAtlas.Set(true) })
	app.WaitUntil(5*time.Second, func() bool {
		got = app.Capture()
		p := point(got, 120, 16)
		r, g, b, _ := got.At(p.X, p.Y).RGBA()
		return min(r, g, b)>>8 >= 230
	})
	p := point(got, 16, 80)
	r, g, b, _ := got.At(p.X, p.Y).RGBA()
	if g>>8 < 180 || g < max(r, b)+(80<<8) {
		t.Errorf("the path after DrawAtlas drew (%d, %d, %d), want green", r>>8, g>>8, b>>8)
	}
	region := image.Rectangle{Min: point(want, 4, 4), Max: point(want, 108, 92)}
	if !region.In(got.Bounds()) {
		t.Fatalf("capture changed dimensions: %v to %v", want.Bounds(), got.Bounds())
	}
	var different int
	var first image.Point
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			a := color.RGBAModel.Convert(want.At(x, y)).(color.RGBA)
			b := color.RGBAModel.Convert(got.At(x, y)).(color.RGBA)
			beyond := func(a, b uint8) bool { return int(max(a, b))-int(min(a, b)) > 5 }
			if beyond(a.R, b.R) || beyond(a.G, b.G) || beyond(a.B, b.B) || beyond(a.A, b.A) {
				if different == 0 {
					first = image.Pt(x, y)
				}
				different++
			}
		}
	}
	if different > region.Dx()*region.Dy()/500 {
		t.Fatalf("%d of %d atlas pixels differ from DrawImageFrom, first at %v (%v, want %v)",
			different, region.Dx()*region.Dy(), first, got.At(first.X, first.Y), want.At(first.X, first.Y))
	}
}
