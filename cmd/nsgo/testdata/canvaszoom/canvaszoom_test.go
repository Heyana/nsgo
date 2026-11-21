package canvaszoom

import (
	"math"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func lineCanvas(draws *int) core.Node {
	return core.Canvas{Style: core.Style{Width: core.Pt(300), Height: core.Pt(300)}, Draw: func(d *core.Drawing, _ core.Size) {
		*draws++
		d.SetFillColor(core.RGB(0xffffff)).FillRect(0, 0, 300, 300)
		d.SetFillColor(core.RGB(0x000000)).FillRect(50, 0, 1, 300)
	}}
}

func zoomScroll(ref *core.Ref, children ...core.Node) core.Scroll {
	return core.Scroll{Ref: ref, HideIndicator: true, MinZoomScale: .5, MaxZoomScale: 4,
		Style: core.Style{Width: core.Pt(300), Height: core.Pt(300), Background: core.RGB(0xffffff)}, Children: children}
}

func requireWeb(t *testing.T) {
	t.Helper()
	if nsgotest.Target() != "web" {
		t.Skip("exercises browser canvas rasterization")
	}
}

func checkLine(t *testing.T, a *nsgotest.App, zoom float64) {
	t.Helper()
	a.Advance(500 * time.Millisecond)
	img := a.Capture()
	b := img.Bounds()
	// Stay inside a nested viewport even when an outer scroll zooms out.
	y := b.Min.Y + b.Dy()/4
	black, gray := 0, 0
	for x := b.Min.X; x < b.Max.X; x++ {
		r, _, _, _ := img.At(x, y).RGBA()
		switch {
		case r < 0x2000:
			black++
		case r < 0xe000:
			gray++
		}
	}
	want := int(math.Round(zoom * float64(b.Dx()) / 300))
	if black != want || gray != 0 {
		t.Errorf("zoom %g, capture %dx%d: line has %d black and %d gray pixels, want %d black and no gray", zoom, b.Dx(), b.Dy(), black, gray, want)
	}
}

func TestZoomedCanvasIsSharp(t *testing.T) {
	requireWeb(t)
	var ref core.Ref
	draws := 0
	a := nsgotest.Start(t, func(*core.Scope) core.Node { return zoomScroll(&ref, lineCanvas(&draws)) }, nsgotest.Size(300, 300))
	before := draws
	for _, step := range []struct {
		zoom     float32
		animated bool
	}{{3, false}, {1, false}, {3, true}} {
		a.Post(func() { ref.ZoomTo(step.zoom, &core.Point{}, step.animated) })
		checkLine(t, a, float64(step.zoom))
	}
	// A new target takes over an animation instead of snapping back to it.
	a.Post(func() { ref.ZoomTo(4, &core.Point{}, true) })
	a.Advance(40 * time.Millisecond)
	a.Post(func() { ref.ZoomTo(2, &core.Point{}, false) })
	checkLine(t, a, 2)
	if draws != before {
		t.Errorf("zoom reran Draw %d times; only the native recording should be replayed", draws-before)
	}
}

func TestCanvasInitialZoomIsSharp(t *testing.T) {
	requireWeb(t)
	var ref core.Ref
	draws := 0
	a := nsgotest.Start(t, func(*core.Scope) core.Node {
		scroll := zoomScroll(&ref, lineCanvas(&draws))
		scroll.ZoomScale = 3
		return scroll
	}, nsgotest.Size(300, 300))
	checkLine(t, a, 3)
}

func TestNestedZoomedCanvasIsSharp(t *testing.T) {
	requireWeb(t)
	var outer, inner core.Ref
	draws := 0
	a := nsgotest.Start(t, func(*core.Scope) core.Node {
		return zoomScroll(&outer, zoomScroll(&inner, lineCanvas(&draws)))
	}, nsgotest.Size(300, 300))
	for _, scales := range [][2]float32{{2, 1.5}, {1.5, 2}, {.5, 2}} {
		a.Post(func() {
			outer.ZoomTo(scales[0], &core.Point{}, false)
			inner.ZoomTo(scales[1], &core.Point{}, false)
		})
		checkLine(t, a, float64(scales[0]*scales[1]))
	}
}

func TestCanvasMountedAfterZoomIsSharp(t *testing.T) {
	requireWeb(t)
	var ref core.Ref
	var shown *core.Signal[bool]
	draws := 0
	a := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		shown = core.NewSignal(cx, false)
		return zoomScroll(&ref, core.Show{When: shown.Get, Then: func(*core.Scope) core.Node { return lineCanvas(&draws) }})
	}, nsgotest.Size(300, 300))
	a.Post(func() { ref.ZoomTo(3, &core.Point{}, false) })
	// Past the zoom's replay, so only the mount can sharpen the new canvas.
	a.Advance(500 * time.Millisecond)
	a.Post(func() { shown.Set(true) })
	checkLine(t, a, 3)
}

func TestLargeZoomedCanvasIsSharp(t *testing.T) {
	requireWeb(t)
	var ref core.Ref
	draws := 0
	// 2500×2500 at 3x zoom (ratio 2 -> 15000×15000 full bitmap) exceeds the
	// 4096×4096 budget and stays sharp only by drawing its visible slice.
	large := core.Canvas{Style: core.Style{Width: core.Pt(2500), Height: core.Pt(2500)}, Draw: func(d *core.Drawing, _ core.Size) {
		draws++
		d.SetFillColor(core.RGB(0xffffff)).FillRect(0, 0, 2500, 2500)
		d.SetFillColor(core.RGB(0x000000)).FillRect(50, 0, 1, 2500)
		d.SetFillColor(core.RGB(0x000000)).FillRect(1200, 0, 1, 2500)
	}}
	a := nsgotest.Start(t, func(*core.Scope) core.Node { return zoomScroll(&ref, large) }, nsgotest.Size(300, 300))
	before := draws
	a.Post(func() { ref.ZoomTo(3, &core.Point{X: 50, Y: 50}, false) })
	checkLine(t, a, 3)
	// Panning to a line far outside the initial slice repaints the new visible region sharply.
	a.Post(func() { ref.ZoomTo(3, &core.Point{X: 1200, Y: 1200}, false) })
	checkLine(t, a, 3)
	if draws != before {
		t.Errorf("zooming large canvas reran Draw %d times", draws-before)
	}
}

func TestStyleTransformCanvasIsSharp(t *testing.T) {
	requireWeb(t)
	draws := 0
	var scale *core.Signal[float32]
	a := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scale = core.NewSignal[float32](cx, 3)
		return core.View{
			Style: core.Style{Width: core.Pt(300), Height: core.Pt(300), Background: core.RGB(0xffffff), Overflow: core.Hidden},
			Children: []core.Node{
				core.View{
					StyleFunc: func() core.Style {
						return core.Style{
							Width:           core.Pt(300),
							Height:          core.Pt(300),
							TransformOrigin: core.ObjectPosition{X: core.PositionStart, Y: core.PositionStart},
							Transform:       core.Transform{Scale: scale.Get()},
						}
					},
					Children: []core.Node{lineCanvas(&draws)},
				},
			},
		}
	}, nsgotest.Size(300, 300))
	checkLine(t, a, 3)
	a.Post(func() { scale.Set(2) })
	checkLine(t, a, 2)
}

func TestAnimatedStyleTransformCanvasIsSharp(t *testing.T) {
	requireWeb(t)
	draws := 0
	var animScale *core.AnimatedValue
	a := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		animScale = core.NewAnimatedValue(cx, 3)
		return core.View{
			Style: core.Style{Width: core.Pt(300), Height: core.Pt(300), Background: core.RGB(0xffffff), Overflow: core.Hidden},
			Children: []core.Node{
				core.View{
					Style: core.Style{
						Width:           core.Pt(300),
						Height:          core.Pt(300),
						TransformOrigin: core.ObjectPosition{X: core.PositionStart, Y: core.PositionStart},
						Animated:        &core.AnimatedStyle{Scale: animScale},
					},
					Children: []core.Node{lineCanvas(&draws)},
				},
			},
		}
	}, nsgotest.Size(300, 300))
	checkLine(t, a, 3)
	a.Post(func() { animScale.Set(2) })
	checkLine(t, a, 2)
}
