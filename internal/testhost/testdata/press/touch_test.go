package press_test

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/base/camera"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestTouchCoordinatesFollowTheCamera uses native pointer input on a target: the camera's bound values move the content without changing its layout.
func TestTouchCoordinatesFollowTheCamera(t *testing.T) {
	var state camera.State
	type sample struct {
		kind     string
		at, page core.Point
	}
	var got []sample
	var touches []core.TouchEvent
	note := func(kind string) func(core.ResponderEvent) {
		return func(e core.ResponderEvent) { got = append(got, sample{kind, e.Position, e.PagePosition}) }
	}
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return camera.View{TestID: "camera", State: &state,
			Style:    core.Style{Width: core.Pt(300), Height: core.Pt(300)},
			Children: []core.Node{core.View{TestID: "zoom", Style: core.Style{Position: core.Absolute, Right: core.Pt(0), Top: core.Pt(0), Width: core.Pt(30), Height: core.Pt(30)}, OnPress: func(core.PressEvent) { state.ZoomIn() }}},
			Content: core.View{Style: core.Style{Width: core.Pt(600), Height: core.Pt(500)}, Children: []core.Node{
				core.View{TestID: "pad", Style: core.Style{Position: core.Absolute,
					Left: core.Pt(300), Top: core.Pt(200), Width: core.Pt(80), Height: core.Pt(60)},
					OnTouchCapture: func(e core.TouchEvent) {
						p := e.Touches[0]
						got = append(got, sample{"capture", p.Position, p.PagePosition})
					},
					OnTouch: func(e core.TouchEvent) { touches = append(touches, e) },
					Responder: &core.Responder{Claims: []core.TouchClaim{{}},
						OnGrant: note("grant"), OnStart: note("start"), OnMove: note("move"),
						OnEnd: note("end"), OnRelease: note("release")}},
			}}}
	}, nsgotest.Size(300, 300), nsgotest.Environment(core.Environment{ReduceMotion: true}))
	near := func(a, b core.Point) bool {
		// Native input rounds to device pixels, amplified by a camera below 1×.
		return math.Abs(float64(a.X-b.X)) < 3 && math.Abs(float64(a.Y-b.Y)) < 3
	}
	check := func() {
		t.Helper()
		got, touches = nil, nil
		start := state.ToView(core.Point{X: 320, Y: 220})
		app.PressAt("camera", start)
		if len(touches) < 2 || touches[0].Phase != core.TouchStart || touches[len(touches)-1].Phase != core.TouchEnd {
			t.Fatalf("touch sequence = %+v", touches)
		}
		if p := touches[0].Touches[0]; !near(p.Position, core.Point{X: 20, Y: 20}) || !near(p.PagePosition, start) {
			t.Fatalf("touch start = %+v, want local (20,20) at %v", p, start)
		}
		if p := touches[len(touches)-1].Touches[0]; !near(p.Position, core.Point{X: 20, Y: 20}) || !near(p.PagePosition, start) {
			t.Fatalf("touch end = %+v, want local (20,20) at %v", p, start)
		}
		for _, kind := range []string{"capture", "grant", "start", "end", "release"} {
			found := false
			for _, s := range got {
				if s.kind != kind {
					continue
				}
				found = true
				matched := false
				for _, e := range touches {
					p := e.Touches[0]
					matched = matched || near(s.page, p.PagePosition) && near(s.at, p.Position)
				}
				if !matched {
					t.Fatalf("%s = %+v disagrees with touches %+v", kind, s, touches)
				}
			}
			if !found {
				t.Fatalf("no %s callback: %+v", kind, got)
			}
		}
	}
	check()
	// Pan on empty space so the pad's responder does not take the camera's drag.
	app.DragAt("camera", core.Point{X: 10, Y: 10}, core.Point{X: 40, Y: 30})
	if !state.Moved() {
		t.Fatal("the native drag did not pan the camera")
	}
	app.Press("zoom")
	app.Settle()
	check()
}

// TestTouchMovesFollowNestedBoundTransforms checks the entire responder sequence under a translated/scaled parent and a rotated child. There is no native pan recognizer here to move the content with the finger.
func TestTouchMovesFollowNestedBoundTransforms(t *testing.T) {
	var touches []core.Touch
	var moves []core.ResponderEvent
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		x, y := core.NewAnimatedValue(cx, 100), core.NewAnimatedValue(cx, 40)
		scale, rotate := core.NewAnimatedValue(cx, 1.5), core.NewAnimatedValue(cx, 90)
		return core.View{TestID: "root", Style: core.Style{Width: core.Pt(400), Height: core.Pt(300)}, Children: []core.Node{core.View{
			Style: core.Style{Position: core.Absolute, Width: core.Pt(160), Height: core.Pt(120),
				TransformOrigin: core.ObjectPosition{X: core.PositionStart, Y: core.PositionStart},
				Animated:        &core.AnimatedStyle{TranslateX: x, TranslateY: y, Scale: scale}},
			Children: []core.Node{core.View{Style: core.Style{Position: core.Absolute,
				Left: core.Pt(20), Top: core.Pt(10), Width: core.Pt(80), Height: core.Pt(60),
				TransformOrigin: core.ObjectPosition{X: core.PositionStart, Y: core.PositionStart},
				Animated:        &core.AnimatedStyle{Rotate: rotate}},
				OnTouch: func(e core.TouchEvent) { touches = append(touches, e.Touches[0]) },
				Responder: &core.Responder{Claims: []core.TouchClaim{{}},
					OnMove: func(e core.ResponderEvent) { moves = append(moves, e) }}}},
		}}}
	}, nsgotest.Size(400, 300))
	// Rotation by 90 degrees at the child's origin, then parent scale and translation.
	app.DragAt("root", core.Point{X: 100, Y: 85}, core.Point{X: -15, Y: 45})
	if len(touches) < 3 || len(moves) == 0 {
		t.Fatalf("touches=%+v, responder moves=%+v", touches, moves)
	}
	near := func(a, b core.Point) bool {
		return math.Abs(float64(a.X-b.X)) < 2 && math.Abs(float64(a.Y-b.Y)) < 2
	}
	if !near(touches[0].Position, core.Point{X: 20, Y: 20}) ||
		!near(touches[len(touches)-1].Position, core.Point{X: 50, Y: 30}) {
		t.Fatalf("touch path = %+v, want local (20,20) to (50,30)", touches)
	}
	for _, move := range moves {
		matched := false
		for _, touch := range touches {
			matched = matched || near(move.PagePosition, touch.PagePosition) && near(move.Position, touch.Position)
		}
		if !matched {
			t.Fatalf("responder move %+v disagrees with touches %+v", move, touches)
		}
	}
}
