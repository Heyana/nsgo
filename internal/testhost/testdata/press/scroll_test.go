package press_test

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The same point distance must reach a wheel handler and a native scroll view, including in a browser with an emulated high-density display.
func TestTouchpadScrollUsesPointDistances(t *testing.T) {
	desktopPointer(t)
	var moved core.Point
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Gap: 20}, Children: []core.Node{
			core.View{TestID: "wheel", Style: core.Style{Width: core.Pt(200), Height: core.Pt(100)},
				OnWheel: func(e core.WheelEvent) { moved.X += e.Delta.X; moved.Y += e.Delta.Y }},
			core.Scroll{TestID: "scroll", Style: core.Style{Width: core.Pt(200), Height: core.Pt(100)},
				Children: []core.Node{core.View{Style: core.Style{Height: core.Pt(1000)}}}},
		}}
	}, nsgotest.Size(300, 300))
	app.TouchpadScroll("wheel", core.Point{}, core.Point{X: 40, Y: 60})
	if math.Abs(float64(moved.X-40)) > 1 || math.Abs(float64(moved.Y-60)) > 1 {
		t.Fatalf("wheel handler moved %v, want {40 60}", moved)
	}
	app.Scroll("scroll", 80)
	if got := app.Find("scroll").Offset.Y; math.Abs(float64(got-80)) > 1 {
		t.Fatalf("native scroll offset = %v, want 80", got)
	}
}
