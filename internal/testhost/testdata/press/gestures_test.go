package press_test

import (
	"math"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestPanEndsAtTheLastOffset(t *testing.T) {
	if target := nsgotest.Target(); target != "ios_simulator" && target != "ios" && target != "android" {
		desktopPointer(t)
	}
	var end core.PanEvent
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.All(90)}, Children: []core.Node{
			core.View{TestID: "pan", Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)},
				OnPan: func(e core.PanEvent) { end = e }},
		}}
	}, nsgotest.Size(400, 400))
	last := core.Point{X: 36, Y: 24}
	app.Pan("id:pan", core.Point{X: 20, Y: 10}, last)
	dx, dy := end.Translation.X-last.X, end.Translation.Y-last.Y
	if end.Phase != core.GestureEnded || math.Hypot(float64(dx), float64(dy)) > 1 {
		t.Fatalf("last pan = %+v, want an end at %v", end, last)
	}
}

func TestPinchReportsScaleAndPhase(t *testing.T) {
	if target := nsgotest.Target(); target != "" && target != "macos" && target != "web" && target != "ios_simulator" && target != "ios" && target != "android" {
		t.Skipf("native pinch input is unavailable on %s", target)
	}
	type sample struct {
		phase core.GesturePhase
		scale float32
		at    core.Point
	}
	var samples []sample
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.All(90)}, Children: []core.Node{
			core.View{TestID: "pinch", Style: core.Style{Width: core.Pt(80), Height: core.Pt(60)},
				OnPinch: func(e core.GestureEvent) { samples = append(samples, sample{e.Phase, e.Value, e.Position}) }},
		}}
	}, nsgotest.Size(400, 400))
	at := core.Point{X: 40, Y: 30}
	app.Pinch("id:pinch", 1, 1.5, 2)
	if target := nsgotest.Target(); target == "ios_simulator" || target == "ios" || target == "android" {
		// Phone recognizers begin only after their movement threshold.
		if len(samples) < 3 || samples[0].phase != core.GestureBegan {
			t.Fatalf("pinch did not begin and move: %v", samples)
		}
		last, end := samples[len(samples)-2], samples[len(samples)-1]
		dx, dy := last.at.X-at.X, last.at.Y-at.Y
		if last.phase != core.GestureChanged || math.Hypot(float64(dx), float64(dy)) > 1 || last.scale <= samples[0].scale || end.phase != core.GestureEnded || end.scale != last.scale {
			t.Fatalf("pinch did not expand around %v and end: %v", at, samples)
		}
		if target == "android" && math.Abs(float64(end.scale)-2) > 0.02 {
			t.Fatalf("pinch ended at %v, want scale 2", end.scale)
		}
		return
	}
	// The engine can coalesce consecutive moves. The last move must
	// still arrive before the end, at the requested center and scale.
	began := sample{core.GestureBegan, 1, at}
	changed := sample{core.GestureChanged, 2, at}
	ended := sample{core.GestureEnded, 2, at}
	if len(samples) < 3 || samples[0] != began || samples[len(samples)-2] != changed || samples[len(samples)-1] != ended {
		t.Fatalf("pinch events = %v, want begin at %v and move and end at %v around %v", samples, began.scale, ended.scale, at)
	}
	if nsgotest.Target() != "web" {
		samples = nil
		app.Pinch("id:pinch", 0.75)
		want := []sample{{core.GestureBegan, 0.75, at}, {core.GestureEnded, 0.75, at}}
		if !slices.Equal(samples, want) {
			t.Fatalf("one-sample pinch = %v, want %v", samples, want)
		}
	}
}
