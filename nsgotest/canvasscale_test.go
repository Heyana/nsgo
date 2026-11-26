package nsgotest_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestCanvasResolutionScaleReplaysWithoutDrawing(t *testing.T) {
	var (
		scale    *core.Signal[float32]
		sampling *core.Signal[core.ImageSampling]
		zoom     *core.Signal[float32]
	)
	draws, presses := 0, 0
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scale = core.NewSignal(cx, float32(0))
		sampling = core.NewSignal(cx, core.ImageSamplingSmooth)
		zoom = core.NewSignal(cx, float32(1))
		return core.View{StyleFunc: func() core.Style {
			return core.Style{Transform: core.Transform{Scale: zoom.Get()}}
		}, Children: []core.Node{core.Canvas{
			TestID:              "scene",
			Style:               core.Style{Width: core.Pt(100), Height: core.Pt(50)},
			ResolutionScaleFunc: func() float32 { return scale.Get() },
			SamplingFunc:        func() core.ImageSampling { return sampling.Get() },
			OnPress:             func(core.PressEvent) { presses++ },
			Draw: func(d *core.Drawing, size core.Size) {
				draws++
				d.FillRect(0, 0, float32(size.Width), float32(size.Height))
			},
		}}}
	}, nsgotest.Size(200, 200), nsgotest.Environment(driver.Environment{OS: driver.OSMacOS, Idiom: driver.IdiomDesktop, PixelRatio: 3}))

	if got := app.CanvasScale("scene"); got != 3 {
		t.Fatalf("CanvasScale = %v, want the screen's 3", got)
	}
	v := app.Find("scene")
	if v.ResolutionScale != 1 || v.Drawing.Resolution != 0 {
		t.Fatalf("unset scale = %v (drawing %v), want 1 (0)", v.ResolutionScale, v.Drawing.Resolution)
	}
	ops := v.Drawing.Ops

	app.Post(func() { scale.Set(0.5); sampling.Set(core.ImageSamplingNearest) })
	app.Settle()
	v = app.Find("scene")
	if draws != 1 {
		t.Fatalf("Draw ran %d times, want once: a new scale replays the drawing", draws)
	}
	if v.ResolutionScale != 0.5 || v.CanvasSampling != core.ImageSamplingNearest || &v.Drawing.Ops[0] != &ops[0] {
		t.Fatalf("scale %v, sampling %v, want 0.5 nearest over the same ops", v.ResolutionScale, v.CanvasSampling)
	}
	if got := app.CanvasScale("scene"); got != 1.5 {
		t.Fatalf("CanvasScale = %v, want 3 × 0.5", got)
	}
	if v.Frame.Size != (core.Size{Width: 100, Height: 50}) {
		t.Fatalf("frame = %v, want the size in points", v.Frame.Size)
	}

	// The factor multiplies the zoom's raster step rather than replacing it.
	app.Post(func() { zoom.Set(2) })
	app.Settle()
	if got := app.CanvasScale("scene"); got != 3 {
		t.Fatalf("zoomed CanvasScale = %v, want 3 × 2 × 0.5", got)
	}

	for _, r := range []float32{1, 2, -1} {
		app.Post(func() { scale.Set(r) })
		app.Settle()
		if v := app.Find("scene"); v.ResolutionScale != 1 {
			t.Fatalf("ResolutionScale %v plays at %v, want 1", r, v.ResolutionScale)
		}
	}
	if draws != 1 {
		t.Fatalf("Draw ran %d times, want once", draws)
	}

	app.Post(func() { scale.Set(0.25) })
	app.Settle()
	app.Press("scene")
	if presses != 1 {
		t.Fatalf("presses = %d, want 1 at a quarter's resolution", presses)
	}
}
