//go:build windows

package windows

import (
	"math"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestWheelAndPinchPositionNative posts real WM_MOUSEWHEEL and WM_MOUSEHWHEEL
// messages to a live Win32 window and verifies OnWheel deltas, signs,
// precision-touchpad detection, scroll nesting, transformed-view coordinates,
// and Ctrl+wheel pinch Position across Began, Changed and Ended (#1607).
func TestWheelAndPinchPositionNative(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	var mu sync.Mutex
	var wheels []core.WheelEvent
	var scaledWheels []core.WheelEvent
	var pinches []core.GestureEvent

	title := "nsgo-wheel-pinch-native"
	app := func(cx *core.Scope) core.Node {
		return core.Scroll{
			TestID: "outer-scroll",
			Style:  core.Style{Width: driver.Pt(360), Height: driver.Pt(360)},
			Children: []core.Node{
				core.View{
					Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(720)},
					Children: []core.Node{
						core.View{
							TestID: "wheel-box",
							Style:  core.Style{Position: core.Absolute, Left: driver.Pt(30), Top: driver.Pt(40), Width: driver.Pt(180), Height: driver.Pt(140)},
							OnWheel: func(e core.WheelEvent) {
								mu.Lock()
								wheels = append(wheels, e)
								mu.Unlock()
							},
							Children: []core.Node{
								core.Scroll{
									TestID: "inner-scroll",
									Style:  core.Style{Position: core.Absolute, Left: driver.Pt(10), Top: driver.Pt(10), Width: driver.Pt(60), Height: driver.Pt(50)},
									Children: []core.Node{
										core.View{Style: core.Style{Width: driver.Pt(60), Height: driver.Pt(200)}},
									},
								},
							},
						},
						core.View{
							TestID: "pinch-box",
							Style:  core.Style{Position: core.Absolute, Left: driver.Pt(220), Top: driver.Pt(40), Width: driver.Pt(120), Height: driver.Pt(120)},
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
								Left:      driver.Pt(40),
								Top:       driver.Pt(220),
								Width:     driver.Pt(80),
								Height:    driver.Pt(80),
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
	hwnd := startApp(t, title, app, 400, 400)

	// Wait until the views are mounted and laid out.
	var scale float64
	deadline := time.Now().Add(5 * time.Second)
	for {
		var ready bool
		onMain(t, hwnd, func(w *window) {
			scale = w.scale
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
	px := func(pt float64) int32 { return int32(math.Round(pt * scale)) }
	postWheel := func(msg uintptr, x, y float64, delta int16, keys uintptr) {
		at := point{px(x), px(y)}
		clientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&at)))
		wParam := uintptr(uint32(uint16(delta))<<16) | keys
		lParam := uintptr(uint32(uint16(at.Y))<<16 | uint32(uint16(at.X)))
		postMessageW.Call(hwnd, msg, wParam, lParam)
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

	// 1. Vertical mouse wheel notch (-120) over wheel-box at (130, 120) -> local (100, 80).
	postWheel(wmMouseWheel, 130, 120, -120, 0)
	ev := waitWheel(&wheels, 1)
	if math.Abs(float64(ev.Position.X-100)) > 2 || math.Abs(float64(ev.Position.Y-80)) > 2 {
		t.Errorf("vertical wheel Position = %+v, want (100, 80)", ev.Position)
	}
	if math.Abs(float64(ev.Delta.Y+99)) > 1 || ev.Touchpad || ev.Phase != core.GestureChanged {
		t.Errorf("vertical wheel = %+v, want Delta.Y=-99 Touchpad=false GestureChanged", ev)
	}

	// 2. Horizontal wheel tilt right (+120) -> Delta.X == -99.
	postWheel(wmMouseHWheel, 130, 120, 120, 0)
	ev = waitWheel(&wheels, 2)
	if math.Abs(float64(ev.Delta.X+99)) > 1 || ev.Touchpad {
		t.Errorf("horizontal wheel = %+v, want Delta.X=-99 Touchpad=false", ev)
	}

	// 3. Precision touchpad sub-notch delta (+30) with MK_CONTROL on OnWheel-only view (no OnPinch).
	postWheel(wmMouseWheel, 130, 120, 30, mkControl)
	ev = waitWheel(&wheels, 3)
	if !ev.Touchpad || !ev.Modifiers.Control || ev.Delta.Y <= 0 {
		t.Errorf("touchpad + Ctrl on OnWheel-only view = %+v, want Touchpad=true Control=true Delta.Y>0", ev)
	}

	// 4. Inner Scroll inside OnWheel takes its own wheel first; OnWheel must not fire.
	postWheel(wmMouseWheel, 55, 65, -120, 0)
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	afterInner := len(wheels)
	mu.Unlock()
	if afterInner != 3 {
		t.Errorf("OnWheel fired %d times when wheel landed on inner Scroll, want 3", afterInner)
	}

	// 5. Transformed 2x-scaled view at (40, 220) size 80x80 (scaled around center (80, 260) -> screen [0..160, 180..340]):
	// point (40, 220) is 25% into scaled rect -> local (20, 20).
	postWheel(wmMouseWheel, 40, 220, 120, 0)
	ev = waitWheel(&scaledWheels, 1)
	if math.Abs(float64(ev.Position.X-20)) > 2 || math.Abs(float64(ev.Position.Y-20)) > 2 {
		t.Errorf("scaled wheel Position = %+v, want (20, 20)", ev.Position)
	}

	// 6. Ctrl+wheel pinch over pinch-box at (260, 95) -> local (40, 55) on Began, Changed, and Ended.
	postWheel(wmMouseWheel, 260, 95, 120, mkControl)
	waitPinch := func(phase core.GesturePhase) core.GestureEvent {
		t.Helper()
		for dl := time.Now().Add(5 * time.Second); time.Now().Before(dl); time.Sleep(15 * time.Millisecond) {
			mu.Lock()
			for _, g := range pinches {
				if g.Phase == phase {
					mu.Unlock()
					return g
				}
			}
			mu.Unlock()
		}
		t.Fatalf("never received pinch phase %v", phase)
		return core.GestureEvent{}
	}
	began := waitPinch(core.GestureBegan)
	changed := waitPinch(core.GestureChanged)
	ended := waitPinch(core.GestureEnded)
	for name, g := range map[string]core.GestureEvent{"began": began, "changed": changed, "ended": ended} {
		if math.Abs(float64(g.Position.X-40)) > 2 || math.Abs(float64(g.Position.Y-55)) > 2 {
			t.Errorf("Ctrl+wheel pinch %s Position = %+v, want (40, 55)", name, g.Position)
		}
	}
	if changed.Value <= 1 {
		t.Errorf("Ctrl+wheel pinch zoom-in Value = %v, want > 1", changed.Value)
	}
}
