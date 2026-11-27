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

func TestScrollIntoViewPageAndPress(t *testing.T) {
	var pressed int
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.Scroll{
			TestID: "page",
			Style:  core.Style{FlexGrow: 1},
			Children: []core.Node{
				core.View{TestID: "top-item", Style: core.Style{Height: core.Pt(40)}},
				core.View{Style: core.Style{Height: core.Pt(2400)}},
				core.View{
					TestID:  "bottom-button",
					Style:   core.Style{Height: core.Pt(44)},
					OnPress: func(core.PressEvent) { pressed++ },
					Children: []core.Node{
						core.Text{Content: "Confirm"},
					},
				},
				core.View{Style: core.Style{Height: core.Pt(60)}},
			},
		}
	}, nsgotest.Size(320, 400))

	app.ScrollIntoView("id:bottom-button")
	page := app.Find("id:page").WindowFrame()
	btn := app.Find("id:bottom-button").WindowFrame()
	if btn.Origin.Y < page.Origin.Y-1 || btn.Origin.Y+btn.Size.Height > page.Origin.Y+page.Size.Height+1 {
		t.Fatalf("bottom-button WindowFrame %+v not visible inside page %+v", btn, page)
	}
	app.Press("id:bottom-button")
	if pressed != 1 {
		t.Fatalf("pressed = %d, want 1", pressed)
	}
}

func TestScrollIntoViewNestedAndAlreadyVisible(t *testing.T) {
	var outerScrolls int
	var innerPressed int
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.Scroll{
			TestID:   "outer",
			Style:    core.Style{Width: core.Pt(300), Height: core.Pt(320)},
			OnScroll: func(core.ScrollEvent) { outerScrolls++ },
			Children: []core.Node{
				core.View{TestID: "top-item", Style: core.Style{Height: core.Pt(40)}},
				core.Scroll{
					TestID: "inner",
					Style:  core.Style{Width: core.Pt(260), Height: core.Pt(160), MinHeight: core.Pt(160)},
					Children: []core.Node{
						core.View{Style: core.Style{Height: core.Pt(500)}},
						core.View{
							TestID:  "inner-bottom",
							Style:   core.Style{Height: core.Pt(40)},
							OnPress: func(core.PressEvent) { innerPressed++ },
						},
					},
				},
				core.View{Style: core.Style{Height: core.Pt(900)}},
			},
		}
	}, nsgotest.Size(320, 360))

	// A match already in view must not scroll.
	app.ScrollIntoView("id:top-item")
	if got := app.Find("id:outer").Offset.Y; got != 0 || outerScrolls != 0 {
		t.Fatalf("already-visible item scrolled outer: offset=%v events=%d", got, outerScrolls)
	}

	// Nested scroll views scroll only the innermost one enclosing the match.
	app.ScrollIntoView("id:inner-bottom")
	if got := app.Find("id:outer").Offset.Y; got != 0 || outerScrolls != 0 {
		t.Fatalf("nested ScrollIntoView moved outer: offset=%v events=%d", got, outerScrolls)
	}
	inner := app.Find("id:inner").WindowFrame()
	item := app.Find("id:inner-bottom").WindowFrame()
	if item.Origin.Y < inner.Origin.Y-1 || item.Origin.Y+item.Size.Height > inner.Origin.Y+inner.Size.Height+1 {
		t.Fatalf("inner-bottom WindowFrame %+v not visible inside inner %+v", item, inner)
	}
	app.Press("id:inner-bottom")
	if innerPressed != 1 {
		t.Fatalf("innerPressed = %d, want 1", innerPressed)
	}
}
