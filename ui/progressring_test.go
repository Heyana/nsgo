package ui

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestProgressRing reads its value as Progress does, and the spinners
// turn while work goes on.
func TestProgressRing(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		app, _ := start(t, e, func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Direction: core.Row, Gap: 16}, Children: []core.Node{
				ProgressRing{Label: "Upload", TestID: "upload", Value: func() float32 { return 40 }},
				Spinner{Label: "Sync", TestID: "ring"},
				Spinner{Label: "Load", TestID: "spokes", Variant: SpinnerSpokes, Size: 20},
			}}
		}, nsgotest.ManualFrames())
		if v := app.Find("upload").Props.Value; v.Now != 40 || v.Max != 100 {
			t.Fatalf("value = %v of %v, want 40 of 100", v.Now, v.Max)
		}
		if f := app.Find("spokes").Frame.Size; f.Width != 20 || f.Height != 20 {
			t.Fatalf("spokes are %v, want 20 pt across", f)
		}
		turn := func(id string) float32 { return app.Find(id).Animated.Rotate }
		before := []float32{turn("ring"), turn("spokes")}
		app.Advance(350 * time.Millisecond)
		if turn("ring") == before[0] || turn("spokes") == before[1] {
			t.Fatal("the spinners stand still")
		}
	})
}

// TestProgressRingTint draws the arc in Tint and the track in a quarter of it, and the theme's line and accent without one.
func TestProgressRingTint(t *testing.T) {
	want := core.RGB(0x123456)
	app, p := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			ProgressRing{TestID: "tinted", Value: func() float32 { return 50 }, Tint: func() core.Color { return want }},
			ProgressRing{TestID: "plain", Value: func() float32 { return 50 }},
			Spinner{TestID: "spinner", Tint: func() core.Color { return want }},
		}}
	})
	for id, strokes := range map[string][]core.Color{"tinted": {Alpha(want, 0.25), want}, "plain": {p.Line, p.Accent}, "spinner": {Alpha(want, 0.25), want}} {
		if got := canvasStrokes(app.Find(id)); !slices.Equal(got, strokes) {
			t.Errorf("%s strokes %x, want %x", id, got, strokes)
		}
	}
}

func TestProgressRingLineWidth(t *testing.T) {
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		return ProgressRing{TestID: "ring", Size: 34, LineWidth: 4, Value: func() float32 { return 50 }}
	})
	var widths []float32
	for _, child := range views(app.Find("ring")) {
		if child.Kind != nsgotest.KindCanvas {
			continue
		}
		for _, op := range child.Ops {
			if op.Kind == driver.DrawLineWidth {
				widths = append(widths, op.W)
			}
		}
	}
	if len(widths) != 1 || widths[0] != 4 {
		t.Fatalf("line widths = %v, want [4]", widths)
	}
}

// canvasStrokes are the stroke colors of the first canvas from v down, in order.
func canvasStrokes(v *nsgotest.View) []core.Color {
	for _, child := range views(v) {
		if child.Kind != nsgotest.KindCanvas {
			continue
		}
		var out []core.Color
		for _, op := range child.Ops {
			if op.Kind == driver.DrawStrokeColor {
				out = append(out, op.Color)
			}
		}
		return out
	}
	return nil
}

// TestProgressRingStartAngle starts the arc at StartAngle and runs it the way CounterClockwise says.
func TestProgressRingStartAngle(t *testing.T) {
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			ProgressRing{TestID: "plain", Value: func() float32 { return 25 }},
			ProgressRing{TestID: "turned", Value: func() float32 { return 25 }, StartAngle: 90, CounterClockwise: true},
		}}
	})
	arc := func(id string) driver.DrawOp {
		var last driver.DrawOp
		for _, child := range views(app.Find(id)) {
			for _, op := range child.Ops {
				if op.Kind == driver.DrawArc {
					last = op
				}
			}
		}
		return last
	}
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }
	if a := arc("plain"); !near(a.A0, -math.Pi/2) || !near(a.A1, 0) || a.CCW {
		t.Errorf("plain arc %v to %v ccw %v, want twelve to three clockwise", a.A0, a.A1, a.CCW)
	}
	if a := arc("turned"); !near(a.A0, 0) || !near(a.A1, -math.Pi/2) || !a.CCW {
		t.Errorf("turned arc %v to %v ccw %v, want three to twelve counterclockwise", a.A0, a.A1, a.CCW)
	}
}
