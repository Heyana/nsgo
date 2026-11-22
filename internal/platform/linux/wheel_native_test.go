//go:build linux && !android

package linux

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestWheelAndPinchPositionNative exercises real GTK4 window views for OnWheel
// (mouse notches, touchpad Began/Changed/Ended + deceleration velocity, scroll
// nesting, transformed-view unprojection, Ctrl+wheel on OnWheel-only views) and
// pinch Position across Began, Changed and Ended for both Ctrl+wheel and
// two-finger GtkGestureZoom (#1607).
func TestWheelAndPinchPositionNative(t *testing.T) {
	var mu sync.Mutex
	var wheels []core.WheelEvent
	var scaledWheels []core.WheelEvent
	var pinches []core.GestureEvent

	const title = "nsgo-wheel-pinch-linux"
	app := func(cx *core.Scope) core.Node {
		return core.Scroll{
			TestID: "outer-scroll",
			Style:  core.Style{Width: core.Pt(360), Height: core.Pt(360)},
			Children: []core.Node{
				core.View{
					Style: core.Style{Width: core.Pt(360), Height: core.Pt(720)},
					Children: []core.Node{
						core.View{
							TestID: "wheel-box",
							Style:  core.Style{Position: core.Absolute, Left: core.Pt(30), Top: core.Pt(40), Width: core.Pt(180), Height: core.Pt(140)},
							OnWheel: func(e core.WheelEvent) {
								mu.Lock()
								wheels = append(wheels, e)
								mu.Unlock()
							},
							Children: []core.Node{
								core.Scroll{
									TestID: "inner-scroll",
									Style:  core.Style{Position: core.Absolute, Left: core.Pt(10), Top: core.Pt(10), Width: core.Pt(60), Height: core.Pt(50)},
									Children: []core.Node{
										core.View{Style: core.Style{Width: core.Pt(60), Height: core.Pt(200)}},
									},
								},
							},
						},
						core.View{
							TestID: "pinch-box",
							Style:  core.Style{Position: core.Absolute, Left: core.Pt(220), Top: core.Pt(40), Width: core.Pt(120), Height: core.Pt(120)},
							OnPinch: func(e core.GestureEvent) {
								mu.Lock()
								pinches = append(pinches, e)
								mu.Unlock()
							},
						},
						core.View{
							TestID: "scaled-wheel",
							Style: core.Style{
								Position:  core.Absolute,
								Left:      core.Pt(40),
								Top:       core.Pt(220),
								Width:     core.Pt(80),
								Height:    core.Pt(80),
								Transform: core.Transform{ScaleX: 2, ScaleY: 2},
							},
							OnWheel: func(e core.WheelEvent) {
								mu.Lock()
								scaledWheels = append(scaledWheels, e)
								mu.Unlock()
							},
						},
					},
				},
			},
		}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 360, Height: 360})

	// Wait until the views are mounted and laid out.
	deadline := time.Now().Add(5 * time.Second)
	for {
		var ready bool
		w.onLoop(t, func() {
			for _, v := range w.views {
				if v != nil && !v.deleted && v.props.TestID == "scaled-wheel" && v.frame.Size.Width > 0 {
					ready = true
					break
				}
			}
		})
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("views never laid out")
		}
		time.Sleep(20 * time.Millisecond)
	}

	waitWheel := func(list *[]core.WheelEvent, want int) core.WheelEvent {
		t.Helper()
		for dl := time.Now().Add(5 * time.Second); time.Now().Before(dl); time.Sleep(15 * time.Millisecond) {
			mu.Lock()
			if len(*list) >= want {
				ev := (*list)[want-1]
				mu.Unlock()
				return ev
			}
			mu.Unlock()
		}
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("got %d wheel events, want %d", len(*list), want)
		return core.WheelEvent{}
	}

	// 1. Mouse wheel notch down (+1) over wheel-box at (130, 120) -> local (100, 80), Delta.Y == -99.
	w.onLoop(t, func() {
		w.wheelEventKnown(0, driver.Point{X: 130, Y: 120}, 0, 1, false, true)
	})
	ev := waitWheel(&wheels, 1)
	if math.Abs(float64(ev.Position.X-100)) > 1.5 || math.Abs(float64(ev.Position.Y-80)) > 1.5 {
		t.Errorf("mouse wheel Position = %+v, want (100, 80)", ev.Position)
	}
	if math.Abs(float64(ev.Delta.Y+99)) > 1 || ev.Touchpad || ev.Phase != core.GestureChanged {
		t.Errorf("mouse wheel = %+v, want Delta.Y=-99 Touchpad=false GestureChanged", ev)
	}

	// 2. Touchpad scroll: Began -> Changed -> Ended with Velocity (sign inverted to content motion).
	w.onLoop(t, func() {
		w.wheelTaker = nil
		w.lastPointer = driver.Point{X: 130, Y: 120}
		w.wheelEventKnown(0, driver.Point{X: 130, Y: 120}, -6, 10, true, true)
	})
	ev = waitWheel(&wheels, 2)
	if !ev.Touchpad || ev.Phase != core.GestureBegan || math.Abs(float64(ev.Delta.X-6)) > 0.5 || math.Abs(float64(ev.Delta.Y+10)) > 0.5 {
		t.Errorf("touchpad began = %+v, want Touchpad=true GestureBegan Delta=(6, -10)", ev)
	}
	w.onLoop(t, func() {
		w.wheelEventKnown(0, driver.Point{X: 130, Y: 120}, -4, 8, true, true)
	})
	ev = waitWheel(&wheels, 3)
	if !ev.Touchpad || ev.Phase != core.GestureChanged {
		t.Errorf("touchpad changed = %+v, want Touchpad=true GestureChanged", ev)
	}
	w.onLoop(t, func() {
		if v := w.wheelTaker; v != nil && !v.deleted {
			w.wheelTaker = nil
			endEv := w.wheelEventAt(v, w.lastPointer, 0)
			endEv.Phase, endEv.Precise, endEv.Velocity = driver.GestureEnded, true, driver.Point{X: 180, Y: -320}
			w.engine.Dispatch(endEv)
		}
	})
	ev = waitWheel(&wheels, 4)
	if !ev.Touchpad || ev.Phase != core.GestureEnded || math.Abs(float64(ev.Velocity.X-180)) > 1 || math.Abs(float64(ev.Velocity.Y+320)) > 1 {
		t.Errorf("touchpad ended = %+v, want GestureEnded Velocity=(180, -320)", ev)
	}

	// 3. Ctrl+wheel on OnWheel-only view (no OnPinch) reaches OnWheel with Control modifier.
	w.onLoop(t, func() {
		w.wheelEventKnown(gdkControl, driver.Point{X: 130, Y: 120}, 0, -1, false, true)
	})
	ev = waitWheel(&wheels, 5)
	if !ev.Modifiers.Control || math.Abs(float64(ev.Delta.Y-99)) > 1 {
		t.Errorf("Ctrl+wheel on OnWheel-only view = %+v, want Control + Delta.Y=99", ev)
	}

	// 4. Inner Scroll inside OnWheel takes its own wheel first; OnWheel must not fire.
	w.onLoop(t, func() {
		w.wheelEventKnown(0, driver.Point{X: 55, Y: 65}, 0, 1, false, true)
	})
	time.Sleep(80 * time.Millisecond)
	mu.Lock()
	afterInner := len(wheels)
	mu.Unlock()
	if afterInner != 5 {
		t.Errorf("OnWheel fired %d times when wheel landed on inner Scroll, want 5", afterInner)
	}

	// 5. Transformed 2x-scaled view at (40, 220) size 80x80: surface (40, 220) -> local (20, 20).
	w.onLoop(t, func() {
		w.wheelEventKnown(0, driver.Point{X: 40, Y: 220}, 0, -1, false, true)
	})
	ev = waitWheel(&scaledWheels, 1)
	if math.Abs(float64(ev.Position.X-20)) > 1.5 || math.Abs(float64(ev.Position.Y-20)) > 1.5 {
		t.Errorf("scaled wheel Position = %+v, want (20, 20)", ev.Position)
	}

	waitPinch := func(start int, phase core.GesturePhase) core.GestureEvent {
		t.Helper()
		for dl := time.Now().Add(5 * time.Second); time.Now().Before(dl); time.Sleep(15 * time.Millisecond) {
			mu.Lock()
			for i := start; i < len(pinches); i++ {
				if pinches[i].Phase == phase {
					g := pinches[i]
					mu.Unlock()
					return g
				}
			}
			mu.Unlock()
		}
		t.Fatalf("never received pinch phase %v", phase)
		return core.GestureEvent{}
	}

	// 6. Ctrl+wheel pinch over pinch-box at (260, 95) -> local (40, 55) on Began, Changed, and Ended.
	w.onLoop(t, func() {
		w.wheelEventKnown(gdkControl, driver.Point{X: 260, Y: 95}, 0, -5, false, true)
	})
	began := waitPinch(0, core.GestureBegan)
	changed := waitPinch(0, core.GestureChanged)
	ended := waitPinch(0, core.GestureEnded)
	for name, g := range map[string]core.GestureEvent{"began": began, "changed": changed, "ended": ended} {
		if math.Abs(float64(g.Position.X-40)) > 1.5 || math.Abs(float64(g.Position.Y-55)) > 1.5 {
			t.Errorf("Ctrl+wheel pinch %s Position = %+v, want (40, 55)", name, g.Position)
		}
	}
	if changed.Value <= 1 {
		t.Errorf("Ctrl+wheel pinch zoom-in Value = %v, want > 1", changed.Value)
	}

	// 7. Two-finger touchscreen pinch (GtkGestureZoom path) at (290, 110) -> local (70, 70) on Began, Changed, and Ended.
	mu.Lock()
	base := len(pinches)
	mu.Unlock()
	w.onLoop(t, func() {
		w.reportTwoFinger(driver.EventPinch, 1.4, driver.Point{X: 290, Y: 110})
		gestureEndCallback(0, 0, w.handle)
	})
	began = waitPinch(base, core.GestureBegan)
	changed = waitPinch(base, core.GestureChanged)
	ended = waitPinch(base, core.GestureEnded)
	for name, g := range map[string]core.GestureEvent{"began": began, "changed": changed, "ended": ended} {
		if math.Abs(float64(g.Position.X-70)) > 1.5 || math.Abs(float64(g.Position.Y-70)) > 1.5 {
			t.Errorf("two-finger pinch %s Position = %+v, want (70, 70)", name, g.Position)
		}
	}
	if math.Abs(float64(changed.Value-1.4)) > 0.01 {
		t.Errorf("two-finger pinch Value = %v, want 1.4", changed.Value)
	}
}
