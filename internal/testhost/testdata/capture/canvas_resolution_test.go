package capture_test

import (
	"image"
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/canvastest"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestCanvasResolutionScale draws canvastest.Resolution on canvases 16 pixels a side at a ResolutionScale of 0.5, smooth and nearest, and checks that the platform stretches each 8-pixel backing store over the canvas's pixels.
func TestCanvasResolutionScale(t *testing.T) {
	if nsgotest.Target() != "android" {
		t.Skip("checks Android's composited capture; the other backends test their stretch themselves")
	}
	samplings := []core.ImageSampling{core.ImageSamplingSmooth, core.ImageSamplingNearest}
	ids := []string{"smooth", "nearest"}
	var ratio float32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		ratio = cx.Env().PixelRatio()
		pt := func(px float32) core.Points { return core.Points(px / ratio) }
		var canvases []core.Node
		for i, s := range samplings {
			canvases = append(canvases, core.Canvas{TestID: ids[i], ResolutionScale: 0.5, Sampling: s,
				Style: core.Style{Width: core.Pt(pt(canvastest.ResolutionSize * canvastest.Scale)), Height: core.Pt(pt(canvastest.ResolutionSize * canvastest.Scale))},
				Draw: func(d *core.Drawing, size core.Size) {
					// One square a backing pixel whatever the screen's density.
					k := float32(size.Width) / canvastest.ResolutionSize
					d.Scale(k, k)
					d.Ops = append(d.Ops, canvastest.Resolution(nil, s).Ops...)
				}})
		}
		return core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), Direction: core.Row, AlignItems: core.AlignStart, Gap: pt(16),
			Padding: core.All(pt(16)), Background: core.RGB(0xff0000)}, Children: canvases}
	}, nsgotest.Size(200, 200))
	origin := func(id string) image.Point {
		b := app.Find("id:" + id).Bounds()
		return image.Pt(int(math.Round(float64(float32(b.Origin.X)*ratio))), int(math.Round(float64(float32(b.Origin.Y)*ratio))))
	}
	var img image.Image
	deadline := time.Now().Add(5 * time.Second)
	for {
		img = app.Capture()
		r, g, b, _ := img.At(origin("nearest").X, origin("nearest").Y).RGBA()
		if r == 0 && g == 0 && b == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("canvases not drawn: %v at %v", img.At(origin("nearest").X, origin("nearest").Y), origin("nearest"))
		}
		time.Sleep(20 * time.Millisecond)
		app.Settle()
	}
	if got := app.CanvasScale("id:nearest"); math.Abs(float64(got-ratio/2)) > 1e-4 {
		t.Errorf("CanvasScale = %v, want half the pixel ratio %v", got, ratio)
	}
	for i, s := range samplings {
		for _, f := range canvastest.CheckResolution(img, origin(ids[i]), s) {
			t.Errorf("%s: %s", ids[i], f)
		}
	}
}
