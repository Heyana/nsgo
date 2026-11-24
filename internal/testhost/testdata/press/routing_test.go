package press_test

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestPressRoutingDoesNotDisableSiblings(t *testing.T) {
	var earlier, later int
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		button := func(id string, x core.Points, press func()) core.Node {
			return core.View{TestID: id, Style: core.Style{Position: core.Absolute,
				Left: core.Pt(x), Top: core.Pt(20), Width: core.Pt(100), Height: core.Pt(60)},
				OnPress: func(core.PressEvent) { press() }, Children: []core.Node{core.Text{Content: id}}}
		}
		return core.View{TestID: "root", Style: core.Style{Width: core.Pt(300), Height: core.Pt(200)},
			Children: []core.Node{
				button("earlier", 20, func() { earlier++ }),
				button("later", 170, func() { later++ }),
			}}
	}, nsgotest.Size(300, 200))
	for i := 1; i <= 3; i++ {
		app.PressAt("id:root", core.Point{X: 70, Y: 50})
		app.PressAt("id:root", core.Point{X: 220, Y: 50})
		if earlier != i || later != i {
			t.Fatalf("alternating press %d: earlier=%d, later=%d", i, earlier, later)
		}
	}
}

func TestClickableTextDoesNotStealSiblingTouches(t *testing.T) {
	var buttonPresses, linkPresses int
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(300), Height: core.Pt(200)}, Children: []core.Node{
			core.View{Style: core.Style{Padding: core.All(12), Gap: 8}, Children: []core.Node{
				core.Input{TestID: "field", Style: core.Style{Height: core.Pt(36)}},
				core.View{TestID: "button", Style: core.Style{Height: core.Pt(36)},
					OnPress: func(core.PressEvent) { buttonPresses++ }},
			}},
			core.View{Style: core.Style{Padding: core.All(12)}, Children: []core.Node{
				core.Text{TestID: "link", Content: "Link", OnPress: func(core.PressEvent) { linkPresses++ }},
			}},
		}}
	}, nsgotest.Size(300, 200))
	app.Press("id:field")
	if !app.Find("id:field").Focused || linkPresses != 0 {
		t.Fatalf("tap on field: focused=%v, link presses=%d", app.Find("id:field").Focused, linkPresses)
	}
	app.Press("id:button")
	if buttonPresses != 1 || linkPresses != 0 {
		t.Fatalf("tap on button: button presses=%d, link presses=%d", buttonPresses, linkPresses)
	}
	app.Press("id:link")
	if linkPresses != 1 {
		t.Fatalf("tap on link: link presses=%d, want 1", linkPresses)
	}
}

// A sibling probed for a touch outside it, which declines it, must not take the touch's pan from the views it lands on.
func TestDeclinedRoutingLeavesPansToTheTouchedViews(t *testing.T) {
	var outer, sibling, presses int
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{TestID: "outer", Style: core.Style{Width: core.Pt(300), Height: core.Pt(240)},
			OnPan: func(core.PanEvent) { outer++ }, Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(300), Height: core.Pt(240)}, Children: []core.Node{
					core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(20), Top: core.Pt(20),
						Width: core.Pt(100), Height: core.Pt(60)}, OnPress: func(core.PressEvent) { presses++ }},
					core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(170), Top: core.Pt(20),
						Width: core.Pt(100), Height: core.Pt(60)}, OnPan: func(core.PanEvent) { sibling++ }},
				}},
			}}
	}, nsgotest.Size(300, 240))
	app.DragAt("id:outer", core.Point{X: 70, Y: 50}, core.Point{X: 0, Y: 40}, core.Point{X: 0, Y: 120})
	if outer == 0 || sibling != 0 || presses != 0 {
		t.Fatalf("drag from the earlier view: outer pans=%d, sibling pans=%d, presses=%d", outer, sibling, presses)
	}
	app.PressAt("id:outer", core.Point{X: 70, Y: 50})
	if presses != 1 || sibling != 0 {
		t.Fatalf("press after the drag: presses=%d, sibling pans=%d", presses, sibling)
	}
}

func TestOverflowPressLeavesItsContainersPressable(t *testing.T) {
	var child, parent, backdrop int
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		clipped := core.NewSignal(cx, false)
		return core.View{TestID: "root", Style: core.Style{Width: core.Pt(300), Height: core.Pt(250)},
			OnPress: func(core.PressEvent) { backdrop++ }, Children: []core.Node{
				core.View{StyleFunc: func() core.Style {
					overflow := core.Visible
					if clipped.Get() {
						overflow = core.Hidden
					}
					return core.Style{Position: core.Absolute, Left: core.Pt(150), Top: core.Pt(100),
						Width: core.Pt(80), Height: core.Pt(80), Overflow: overflow}
				}, OnPress: func(core.PressEvent) { parent++ }, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(20), Height: core.Pt(20)}, Children: []core.Node{
						core.View{Style: core.Style{Width: core.Pt(20), Height: core.Pt(20),
							Transform: core.Transform{TranslateX: -100}}, OnPress: func(core.PressEvent) { child++ }},
					}},
				}},
				core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(240), Top: core.Pt(200),
					Width: core.Pt(50), Height: core.Pt(40)}, OnPress: func(core.PressEvent) { clipped.Set(true) }},
			}}
	}, nsgotest.Size(300, 250))
	for i := 1; i <= 3; i++ {
		app.PressAt("id:root", core.Point{X: 60, Y: 110})
		app.PressAt("id:root", core.Point{X: 210, Y: 150})
		if child != i || parent != i || backdrop != 0 {
			t.Fatalf("overflow press %d: child=%d, parent=%d, backdrop=%d", i, child, parent, backdrop)
		}
	}
	app.PressAt("id:root", core.Point{X: 265, Y: 220})
	app.PressAt("id:root", core.Point{X: 60, Y: 110})
	if child != 3 || parent != 3 || backdrop != 1 {
		t.Fatalf("clipped press: child=%d, parent=%d, backdrop=%d", child, parent, backdrop)
	}
}

func TestPressRoutingUsesNestedTransforms(t *testing.T) {
	var presses []core.PressEvent
	var backdrop int
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return core.View{TestID: "root", Style: core.Style{Width: core.Pt(300), Height: core.Pt(300)},
			OnPress: func(core.PressEvent) { backdrop++ }, Children: []core.Node{
				core.View{PointerEvents: core.PointerEventsBoxNone, Style: core.Style{Position: core.Absolute,
					Left: core.Pt(100), Top: core.Pt(100), Width: core.Pt(40), Height: core.Pt(40),
					Transform: core.Transform{Rotate: 90, Scale: 2}}, Children: []core.Node{
					core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(5), Top: core.Pt(5),
						Width: core.Pt(20), Height: core.Pt(20), Transform: core.Transform{TranslateY: -60}},
						OnPress: func(e core.PressEvent) { presses = append(presses, e) }},
				}},
			}}
	}, nsgotest.Size(300, 300))
	app.PressAt("id:root", core.Point{X: 250, Y: 110})
	if len(presses) != 1 || math.Abs(float64(presses[0].Position.X-10)) > .5 || math.Abs(float64(presses[0].Position.Y-10)) > .5 {
		t.Fatalf("transformed press = %+v, want local (10,10)", presses)
	}
	app.PressAt("id:root", core.Point{X: 115, Y: 115})
	if len(presses) != 1 || backdrop != 1 {
		t.Fatalf("old position: presses=%+v, backdrop=%d", presses, backdrop)
	}
}
