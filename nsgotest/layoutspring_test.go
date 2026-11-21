package nsgotest_test

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestLayoutSpringPhysicsDampingRegimes(t *testing.T) {
	run := func(dampingRatio float64) (peak core.Points, settle time.Duration, monotonic bool) {
		var move *core.Signal[bool]
		var cx0 *core.Scope
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			cx0 = cx
			move = core.NewSignal(cx, false)
			return core.View{Style: core.Style{Width: core.Pt(400), Height: core.Pt(200)}, Children: []core.Node{
				core.View{TestID: "box", StyleFunc: func() core.Style {
					left := core.Pt(0)
					if move.Get() {
						left = core.Pt(200)
					}
					return core.Style{Position: core.Absolute, Left: left, Top: core.Pt(0), Width: core.Pt(40), Height: core.Pt(40)}
				}},
			}}
		}, nsgotest.ManualFrames())

		cx0.AnimateLayout(core.LayoutAnimation{
			Update: &core.LayoutTransition{
				Curve:        core.LayoutSpring,
				Response:     250 * time.Millisecond,
				DampingRatio: dampingRatio,
				// Ignored by physics springs: verifies it runs until settled.
				Duration: 40 * time.Millisecond,
			},
		})
		move.Set(true)
		app.Settle()

		box := app.Find("id:box")
		if box.Frame.Origin.X != 200 || box.ShownFrame.Origin.X != 0 {
			t.Fatalf("at start: Frame=%v ShownFrame=%v", box.Frame, box.ShownFrame)
		}

		monotonic = true
		prev := box.ShownFrame.Origin.X
		const step = 10 * time.Millisecond
		for elapsed := step; elapsed <= 3*time.Second; elapsed += step {
			app.Advance(step)
			cur := app.Find("id:box").ShownFrame.Origin.X
			if cur > peak {
				peak = cur
			}
			if cur < prev-1e-3 {
				monotonic = false
			}
			prev = cur
			if cur == 200 {
				settle = elapsed
				break
			}
		}
		if settle == 0 {
			t.Fatalf("dampingRatio %v did not settle within 3s (last X=%v)", dampingRatio, prev)
		}
		return peak, settle, monotonic
	}

	underPeak, underSettle, _ := run(0.4)
	if underPeak <= 240 {
		t.Fatalf("underdamped spring peak = %v, want overshoot > 240", underPeak)
	}
	if underSettle <= 400*time.Millisecond {
		t.Fatalf("underdamped spring settled too early at %v (ignored Duration=40ms?)", underSettle)
	}

	critPeak, critSettle, critMono := run(1.0)
	if !critMono || critPeak > 200.01 {
		t.Fatalf("critically damped overshot or reversed: peak=%v monotonic=%v", critPeak, critMono)
	}

	overPeak, overSettle, overMono := run(2.0)
	if !overMono || overPeak > 200.01 {
		t.Fatalf("overdamped overshot or reversed: peak=%v monotonic=%v", overPeak, overMono)
	}
	if overSettle <= critSettle {
		t.Fatalf("overdamped settle %v <= critically damped settle %v", overSettle, critSettle)
	}
}

func TestLayoutSpringRetargetPreservesVelocity(t *testing.T) {
	type pos struct{ x, y core.Points }
	var target *core.Signal[pos]
	var cx0 *core.Scope
	spring := core.LayoutTransition{Curve: core.LayoutSpring, Response: 300 * time.Millisecond, DampingRatio: 1}

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx0 = cx
		target = core.NewSignal(cx, pos{0, 0})
		return core.View{Style: core.Style{Width: core.Pt(500), Height: core.Pt(300)}, Children: []core.Node{
			core.View{TestID: "box", StyleFunc: func() core.Style {
				p := target.Get()
				return core.Style{Position: core.Absolute, Left: core.Pt(p.x), Top: core.Pt(p.y), Width: core.Pt(50), Height: core.Pt(50)}
			}},
		}}
	}, nsgotest.ManualFrames())

	// Launch rightward toward X = 300.
	cx0.AnimateLayout(core.LayoutAnimation{Update: &spring})
	target.Set(pos{300, 0})
	app.Settle()

	// Advance 48 ms so the box is mid-flight with high positive X velocity.
	app.Advance(48 * time.Millisecond)
	mid := app.Find("id:box").ShownFrame
	if mid.Origin.X <= 40 || mid.Origin.X >= 250 {
		t.Fatalf("expected mid-flight X between 40 and 250, got %v", mid.Origin.X)
	}

	// Retarget back to X = 0 and down to Y = 150 while moving fast in +X.
	cx0.AnimateLayout(core.LayoutAnimation{Update: &spring})
	target.Set(pos{0, 150})
	app.Settle()

	// Must not snap away from its last frame on retarget.
	atRetarget := app.Find("id:box").ShownFrame
	if atRetarget != mid {
		t.Fatalf("retarget snapped from %v to %v", mid, atRetarget)
	}

	// One short step later, preserved +X velocity must carry X briefly past
	// mid.Origin.X before curving smoothly back toward 0, while Y moves from 0
	// toward 150.
	app.Advance(8 * time.Millisecond)
	after := app.Find("id:box").ShownFrame
	if after.Origin.X <= mid.Origin.X {
		t.Fatalf("retarget dropped velocity: X moved from %v to %v instead of continuing briefly forward",
			mid.Origin.X, after.Origin.X)
	}
	if after.Origin.Y <= 0 || after.Origin.Y >= 150 {
		t.Fatalf("Y did not start moving toward 150: %v", after.Origin.Y)
	}

	// Advance until settled at the new target.
	app.Advance(2 * time.Second)
	final := app.Find("id:box").ShownFrame
	if final.Origin.X != 0 || final.Origin.Y != 150 {
		t.Fatalf("did not settle at (0, 150): got %v", final.Origin)
	}
}

func TestLayoutSpringSettlesOnPressAndSnapsWithReduceMotion(t *testing.T) {
	spring := core.LayoutTransition{Curve: core.LayoutSpring, Response: 200 * time.Millisecond, DampingRatio: 0.85}

	// Default (non-ManualFrames): Press advances until the spring settles.
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		moved := core.NewSignal(cx, false)
		return core.View{Style: core.Style{Width: core.Pt(300), Height: core.Pt(100)}, Children: []core.Node{
			core.View{TestID: "go", OnPress: func(core.PressEvent) {
				cx.AnimateLayout(core.LayoutAnimation{Update: &spring})
				moved.Set(true)
			}, StyleFunc: func() core.Style {
				x := core.Pt(0)
				if moved.Get() {
					x = core.Pt(120)
				}
				return core.Style{Position: core.Absolute, Left: x, Width: core.Pt(40), Height: core.Pt(40)}
			}},
		}}
	})
	app.Press("id:go")
	if v := app.Find("id:go"); v.ShownFrame.Origin.X != 120 {
		t.Fatalf("ShownFrame after Press = %v, want settled at 120", v.ShownFrame)
	}

	// Reduced motion snaps immediately even under ManualFrames.
	reduced := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		moved := core.NewSignal(cx, false)
		return core.View{Style: core.Style{Width: core.Pt(300), Height: core.Pt(100)}, Children: []core.Node{
			core.View{TestID: "go", OnPress: func(core.PressEvent) {
				cx.AnimateLayout(core.LayoutAnimation{Update: &spring})
				moved.Set(true)
			}, StyleFunc: func() core.Style {
				x := core.Pt(0)
				if moved.Get() {
					x = core.Pt(120)
				}
				return core.Style{Position: core.Absolute, Left: x, Width: core.Pt(40), Height: core.Pt(40)}
			}},
		}}
	}, nsgotest.ManualFrames(), nsgotest.Environment(core.Environment{ReduceMotion: true}))
	reduced.Press("id:go")
	if v := reduced.Find("id:go"); v.ShownFrame.Origin.X != 120 {
		t.Fatalf("ShownFrame with ReduceMotion = %v, want immediate snap to 120", v.ShownFrame)
	}
}
