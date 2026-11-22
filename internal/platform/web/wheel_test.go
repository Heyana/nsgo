//go:build js && wasm

package web

import (
	"math"
	"strconv"
	"sync"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func nearPt(a, b core.Points, tol float64) bool {
	return math.Abs(float64(a-b)) <= tol
}

func mountTestApp(t *testing.T, id string, width, height int, app func(*core.Scope) core.Node) (*surface, js.Value) {
	t.Helper()
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("run with nsgo test -target web")
	}
	mount := doc.Call("createElement", "div")
	mount.Set("id", id)
	setStyle(mount, map[string]string{
		"position": "fixed",
		"left":     "0px",
		"top":      "0px",
		"width":    strconv.Itoa(width) + "px",
		"height":   strconv.Itoa(height) + "px",
	})
	doc.Get("body").Call("appendChild", mount)
	t.Cleanup(func() { mount.Call("remove") })
	s := open(app, Options{Mount: "#" + id}, nil)
	t.Cleanup(func() { s.engine.Close() })
	return s, mount
}

func waitViewByTestID(t *testing.T, s *surface, mount js.Value, testID string) *view {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		el := mount.Call("querySelector", "[data-testid="+testID+"]")
		if el.Truthy() {
			r := el.Call("getBoundingClientRect")
			if r.Get("width").Float() > 0 && r.Get("height").Float() > 0 {
				if v := s.viewOf(el); v != nil {
					return v
				}
				for _, v := range s.views {
					if v != nil && ((v.el.Truthy() && v.el.Equal(el)) || (v.inner.Truthy() && v.inner.Equal(el))) {
						return v
					}
				}
			}
		}
	}
	t.Fatalf("view %q never mounted", testID)
	return nil
}

func dispatchWheel(target js.Value, init map[string]any) js.Value {
	init["bubbles"] = true
	init["cancelable"] = true
	ev := js.Global().Get("WheelEvent").New("wheel", init)
	target.Call("dispatchEvent", ev)
	return ev
}

func TestWheelOnWheelAndScrollNesting(t *testing.T) {
	var mu sync.Mutex
	var events []core.WheelEvent
	var scaledEvents []core.WheelEvent

	s, mount := mountTestApp(t, "nsgo-wheel-test", 400, 400, func(*core.Scope) core.Node {
		return core.Scroll{
			TestID: "outer-scroll",
			Style:  core.Style{Width: core.Pt(400), Height: core.Pt(400)},
			Children: []core.Node{
				core.View{
					Style: core.Style{Width: core.Pt(400), Height: core.Pt(800)},
					Children: []core.Node{
						core.View{
							TestID: "canvas",
							Style:  core.Style{Position: core.Absolute, Left: core.Pt(40), Top: core.Pt(50), Width: core.Pt(200), Height: core.Pt(160)},
							OnWheel: func(e core.WheelEvent) {
								mu.Lock()
								events = append(events, e)
								mu.Unlock()
							},
							Children: []core.Node{
								core.Scroll{
									TestID: "inner-scroll",
									Style:  core.Style{Position: core.Absolute, Left: core.Pt(10), Top: core.Pt(10), Width: core.Pt(80), Height: core.Pt(60)},
									Children: []core.Node{
										core.View{TestID: "inner-content", Style: core.Style{Width: core.Pt(80), Height: core.Pt(240)}},
									},
								},
							},
						},
						core.View{
							TestID: "scaled-canvas",
							Style: core.Style{
								Position:  core.Absolute,
								Left:      core.Pt(40),
								Top:       core.Pt(240),
								Width:     core.Pt(100),
								Height:    core.Pt(100),
								Transform: core.Transform{ScaleX: 2, ScaleY: 2},
							},
							OnWheel: func(e core.WheelEvent) {
								mu.Lock()
								scaledEvents = append(scaledEvents, e)
								mu.Unlock()
							},
						},
					},
				},
			},
		}
	})

	outer := waitViewByTestID(t, s, mount, "outer-scroll")
	canvas := waitViewByTestID(t, s, mount, "canvas")
	innerContent := waitViewByTestID(t, s, mount, "inner-content")
	scaled := waitViewByTestID(t, s, mount, "scaled-canvas")

	waitCount := func(list *[]core.WheelEvent, want int) core.WheelEvent {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			mu.Lock()
			n := len(*list)
			if n >= want {
				got := (*list)[want-1]
				mu.Unlock()
				return got
			}
			mu.Unlock()
		}
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("got %d wheel events, want at least %d", len(*list), want)
		return core.WheelEvent{}
	}

	// 1. Mouse wheel notch (pixel mode): deltaY +100 moves content up (Delta.Y == -100).
	r := canvas.el.Call("getBoundingClientRect")
	cx, cy := r.Get("left").Float()+120, r.Get("top").Float()+90
	ev := dispatchWheel(canvas.el, map[string]any{
		"clientX": cx, "clientY": cy, "deltaX": 0, "deltaY": 100, "deltaMode": 0,
	})
	if !ev.Get("defaultPrevented").Bool() {
		t.Fatal("OnWheel did not preventDefault on wheel event")
	}
	got := waitCount(&events, 1)
	if !nearPt(got.Position.X, 120, 1) || !nearPt(got.Position.Y, 90, 1) {
		t.Errorf("mouse wheel Position = %+v, want (120, 90)", got.Position)
	}
	if !nearPt(got.Delta.X, 0, 0.5) || !nearPt(got.Delta.Y, -100, 0.5) {
		t.Errorf("mouse wheel Delta = %+v, want (0, -100)", got.Delta)
	}
	if got.Touchpad || got.Phase != core.GestureChanged {
		t.Errorf("mouse wheel Touchpad=%v Phase=%v, want false, GestureChanged", got.Touchpad, got.Phase)
	}
	if outer.offset().Y != 0 {
		t.Errorf("outer scroll moved to %v when OnWheel took the wheel", outer.offset())
	}

	// 2. Line mode (deltaMode == 1, 3 lines * 40pt = 120pt) + horizontal + modifiers.
	dispatchWheel(canvas.el, map[string]any{
		"clientX": cx, "clientY": cy, "deltaX": 2, "deltaY": -3, "deltaMode": 1, "shiftKey": true, "altKey": true,
	})
	got = waitCount(&events, 2)
	if !nearPt(got.Delta.X, -80, 0.5) || !nearPt(got.Delta.Y, 120, 0.5) {
		t.Errorf("line mode Delta = %+v, want (-80, 120)", got.Delta)
	}
	if got.Touchpad {
		t.Errorf("line mode Touchpad = true, want false")
	}
	if !got.Modifiers.Shift || !got.Modifiers.Alt {
		t.Errorf("line mode Modifiers = %+v, want Shift+Alt", got.Modifiers)
	}

	// 3. Touchpad scroll (small pixel deltas < 40).
	dispatchWheel(canvas.el, map[string]any{
		"clientX": cx, "clientY": cy, "deltaX": -10, "deltaY": 12, "deltaMode": 0,
	})
	got = waitCount(&events, 3)
	if !got.Touchpad {
		t.Errorf("touchpad scroll Touchpad = false, want true")
	}
	if !nearPt(got.Delta.X, 10, 0.5) || !nearPt(got.Delta.Y, -12, 0.5) {
		t.Errorf("touchpad scroll Delta = %+v, want (10, -12)", got.Delta)
	}

	// 4. Ctrl+wheel on an OnWheel-only view (no OnPinch) reaches OnWheel with Control modifier and prevents browser zoom.
	ev = dispatchWheel(canvas.el, map[string]any{
		"clientX": cx, "clientY": cy, "deltaX": 0, "deltaY": -100, "deltaMode": 0, "ctrlKey": true,
	})
	if !ev.Get("defaultPrevented").Bool() {
		t.Fatal("Ctrl+wheel on OnWheel-only view did not preventDefault")
	}
	got = waitCount(&events, 4)
	if !got.Modifiers.Control || !nearPt(got.Delta.Y, 100, 0.5) {
		t.Errorf("Ctrl+wheel on OnWheel-only view = %+v, want Control + Delta.Y 100", got)
	}

	// 5. Inner Scroll inside OnWheel takes its own wheel first; OnWheel must not fire.
	ir := innerContent.el.Call("getBoundingClientRect")
	dispatchWheel(innerContent.el, map[string]any{
		"clientX": ir.Get("left").Float() + 20, "clientY": ir.Get("top").Float() + 20,
		"deltaX": 0, "deltaY": 60, "deltaMode": 0,
	})
	time.Sleep(80 * time.Millisecond)
	mu.Lock()
	afterInner := len(events)
	mu.Unlock()
	if afterInner != 4 {
		t.Errorf("OnWheel fired %d times when wheel landed on inner Scroll, want 4", afterInner)
	}

	// 6. Transformed view (2x scale around center (50, 50)): top-left in client coords maps to local (0, 0), center maps to (50, 50).
	sr := scaled.el.Call("getBoundingClientRect")
	// Client point 25% into the scaled 200x200 rect is (25, 25) in untransformed 100x100 local space.
	dispatchWheel(scaled.el, map[string]any{
		"clientX": sr.Get("left").Float() + 50, "clientY": sr.Get("top").Float() + 50,
		"deltaX": 0, "deltaY": 100, "deltaMode": 0,
	})
	got = waitCount(&scaledEvents, 1)
	if !nearPt(got.Position.X, 25, 1.5) || !nearPt(got.Position.Y, 25, 1.5) {
		t.Errorf("transformed view wheel Position = %+v, want (25, 25)", got.Position)
	}
}

func TestPinchPositionOnWeb(t *testing.T) {
	var mu sync.Mutex
	var pinches []core.GestureEvent
	var scaledPinches []core.GestureEvent

	s, mount := mountTestApp(t, "nsgo-pinch-test", 400, 400, func(*core.Scope) core.Node {
		return core.View{
			Style: core.Style{Width: core.Pt(400), Height: core.Pt(400)},
			Children: []core.Node{
				core.View{
					TestID: "pinch-box",
					Style:  core.Style{Position: core.Absolute, Left: core.Pt(50), Top: core.Pt(60), Width: core.Pt(200), Height: core.Pt(160)},
					OnPinch: func(e core.GestureEvent) {
						mu.Lock()
						pinches = append(pinches, e)
						mu.Unlock()
					},
				},
				core.View{
					TestID: "scaled-pinch",
					Style: core.Style{
						Position:  core.Absolute,
						Left:      core.Pt(50),
						Top:       core.Pt(250),
						Width:     core.Pt(100),
						Height:    core.Pt(100),
						Transform: core.Transform{ScaleX: 2, ScaleY: 2},
					},
					OnPinch: func(e core.GestureEvent) {
						mu.Lock()
						scaledPinches = append(scaledPinches, e)
						mu.Unlock()
					},
				},
			},
		}
	})

	box := waitViewByTestID(t, s, mount, "pinch-box")
	scaled := waitViewByTestID(t, s, mount, "scaled-pinch")

	waitPhase := func(list *[]core.GestureEvent, start int, phase core.GesturePhase) core.GestureEvent {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			mu.Lock()
			for i := start; i < len(*list); i++ {
				if (*list)[i].Phase == phase {
					got := (*list)[i]
					mu.Unlock()
					return got
				}
			}
			mu.Unlock()
		}
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("never received pinch phase %v (have %+v)", phase, (*list)[start:])
		return core.GestureEvent{}
	}

	// 1. Ctrl+wheel trackpad pinch: Began, Changed, AND Ended must all carry the pointer's Position in the view.
	r := box.el.Call("getBoundingClientRect")
	px, py := r.Get("left").Float()+70, r.Get("top").Float()+45
	ev := dispatchWheel(box.el, map[string]any{
		"clientX": px, "clientY": py, "deltaX": 0, "deltaY": -25, "deltaMode": 0, "ctrlKey": true,
	})
	if !ev.Get("defaultPrevented").Bool() {
		t.Fatal("Ctrl+wheel pinch did not preventDefault")
	}
	began := waitPhase(&pinches, 0, core.GestureBegan)
	changed := waitPhase(&pinches, 0, core.GestureChanged)
	ended := waitPhase(&pinches, 0, core.GestureEnded)
	for name, g := range map[string]core.GestureEvent{"began": began, "changed": changed, "ended": ended} {
		if !nearPt(g.Position.X, 70, 1) || !nearPt(g.Position.Y, 45, 1) {
			t.Errorf("Ctrl+wheel pinch %s Position = %+v, want (70, 45)", name, g.Position)
		}
	}
	if changed.Value <= 1 {
		t.Errorf("Ctrl+wheel pinch zoom-in Value = %v, want > 1", changed.Value)
	}

	// 2. Two-finger touch pinch via PointerEvent: Position is the midpoint of the two fingers.
	mu.Lock()
	base := len(pinches)
	mu.Unlock()
	sendPtr := func(typ string, id int, x, y float64) {
		ev := js.Global().Get("PointerEvent").New(typ, map[string]any{
			"bubbles": true, "cancelable": true, "pointerId": id, "button": 0, "pointerType": "touch",
			"clientX": r.Get("left").Float() + x, "clientY": r.Get("top").Float() + y,
		})
		box.el.Call("dispatchEvent", ev)
	}
	sendPtr("pointerdown", 11, 40, 50)
	sendPtr("pointerdown", 12, 120, 90)  // midpoint (80, 70)
	sendPtr("pointermove", 12, 160, 110) // midpoint (100, 80), wider span
	sendPtr("pointerup", 12, 160, 110)
	sendPtr("pointerup", 11, 40, 50)

	began = waitPhase(&pinches, base, core.GestureBegan)
	changed = waitPhase(&pinches, base, core.GestureChanged)
	ended = waitPhase(&pinches, base, core.GestureEnded)
	if !nearPt(began.Position.X, 80, 1) || !nearPt(began.Position.Y, 70, 1) {
		t.Errorf("two-finger pinch began Position = %+v, want (80, 70)", began.Position)
	}
	if !nearPt(changed.Position.X, 100, 1) || !nearPt(changed.Position.Y, 80, 1) {
		t.Errorf("two-finger pinch changed Position = %+v, want (100, 80)", changed.Position)
	}
	if changed.Value <= 1.2 {
		t.Errorf("two-finger pinch spread Value = %v, want > 1.2", changed.Value)
	}
	if !nearPt(ended.Position.X, 100, 1) || !nearPt(ended.Position.Y, 80, 1) {
		t.Errorf("two-finger pinch ended Position = %+v, want (100, 80)", ended.Position)
	}

	// 3. Safari-style gesturestart/gesturechange/gestureend events carry Position on all phases.
	mu.Lock()
	base = len(pinches)
	mu.Unlock()
	sendSafari := func(name string, x, y, scale float64) {
		ev := js.Global().Get("Event").New(name, map[string]any{"bubbles": true, "cancelable": true})
		ev.Set("clientX", r.Get("left").Float()+x)
		ev.Set("clientY", r.Get("top").Float()+y)
		ev.Set("scale", scale)
		ev.Set("rotation", 0)
		box.el.Call("dispatchEvent", ev)
	}
	sendSafari("gesturestart", 60, 40, 1)
	sendSafari("gesturechange", 65, 42, 1.5)
	sendSafari("gestureend", 65, 42, 1.5)

	began = waitPhase(&pinches, base, core.GestureBegan)
	changed = waitPhase(&pinches, base, core.GestureChanged)
	ended = waitPhase(&pinches, base, core.GestureEnded)
	if !nearPt(began.Position.X, 60, 1) || !nearPt(began.Position.Y, 40, 1) {
		t.Errorf("Safari gesturestart Position = %+v, want (60, 40)", began.Position)
	}
	if !nearPt(changed.Position.X, 65, 1) || !nearPt(changed.Position.Y, 42, 1) || math.Abs(float64(changed.Value-1.5)) > 0.01 {
		t.Errorf("Safari gesturechange = %+v, want pos (65, 42) scale 1.5", changed)
	}
	if !nearPt(ended.Position.X, 65, 1) || !nearPt(ended.Position.Y, 42, 1) {
		t.Errorf("Safari gestureend Position = %+v, want (65, 42)", ended.Position)
	}

	// 4. Ctrl+wheel pinch on a 2x-scaled view unprojects Position into untransformed local space.
	sr := scaled.el.Call("getBoundingClientRect")
	dispatchWheel(scaled.el, map[string]any{
		"clientX": sr.Get("left").Float() + 150, "clientY": sr.Get("top").Float() + 100,
		"deltaX": 0, "deltaY": -20, "deltaMode": 0, "ctrlKey": true,
	})
	began = waitPhase(&scaledPinches, 0, core.GestureBegan)
	changed = waitPhase(&scaledPinches, 0, core.GestureChanged)
	ended = waitPhase(&scaledPinches, 0, core.GestureEnded)
	for name, g := range map[string]core.GestureEvent{"began": began, "changed": changed, "ended": ended} {
		if !nearPt(g.Position.X, 75, 1.5) || !nearPt(g.Position.Y, 50, 1.5) {
			t.Errorf("scaled Ctrl+wheel pinch %s Position = %+v, want (75, 50)", name, g.Position)
		}
	}
}

// Keep driver import used if helpers are refactored.
var _ = driver.EventWheel
