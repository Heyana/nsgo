package android

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Keep the Android device/emulator test entry points visible to host-side test
// discovery; their bodies require the Android test host and skip elsewhere.
func TestWheelAndTouchpadMomentumNative(t *testing.T) { wheelAndTouchpadMomentumNative(t) }
func TestPinchPositionNative(t *testing.T)            { pinchPositionNative(t) }
func TestPinchStaysActiveBelowMinimumSpanNative(t *testing.T) {
	pinchStaysActiveBelowMinimumSpanNative(t)
}

type javaWheelEvent struct {
	nodeID   uint32
	phase    driver.GesturePhase
	x, y     float32
	pageX    float32
	pageY    float32
	dx, dy   float32
	vx, vy   float32
	touchpad bool
	mods     byte
}

func runWheelScrollJava(t *testing.T, writeOps func(w *writer) int) []javaWheelEvent {
	t.Helper()
	var ops writer
	count := writeOps(&ops)
	var input writer
	input.u32(uint32(count))
	input.buf = append(input.buf, ops.buf...)
	out := runJavaAlgorithm(t, "wheel", "WheelCheck", input.buf, "Protocol", "WheelScroll")
	r := reader{out}
	n := int(r.u32())
	got := make([]javaWheelEvent, 0, n)
	for range n {
		got = append(got, javaWheelEvent{
			nodeID:   r.u32(),
			phase:    driver.GesturePhase(r.byte()),
			x:        r.float(),
			y:        r.float(),
			pageX:    r.float(),
			pageY:    r.float(),
			dx:       r.float(),
			dy:       r.float(),
			vx:       r.float(),
			vy:       r.float(),
			touchpad: r.byte() != 0,
			mods:     r.byte(),
		})
	}
	return got
}

func writeWheelScrollOp(w *writer, nodeID uint32, touchpad bool, h, v, x, y, pageX, pageY float32, mods byte, timeMs uint64) {
	w.byte(1) // OP_SCROLL
	w.u32(nodeID)
	w.flags(touchpad)
	w.floats(h, v, x, y, pageX, pageY)
	w.byte(mods)
	w.u64(timeMs)
}

func writeWheelEndOp(w *writer) {
	w.byte(2) // OP_END
}

func writeWheelCancelOp(w *writer, nodeID uint32) {
	w.byte(3) // OP_CANCEL
	w.u32(nodeID)
}

func TestWheelScrollJavaMouseAndTouchpadMomentum(t *testing.T) {
	events := runWheelScrollJava(t, func(w *writer) int {
		n := 0
		// 1. Mouse wheel vertical (+1 notch away from user -> content moves down +99) and horizontal (+1 right -> content moves left -99).
		writeWheelScrollOp(w, 7, false, 1, 1, 25, 35, 125, 135, 1, 1000)
		n++
		// 2. Touchpad swipe: 4 events 16ms apart, moving content (-8, +12) each step -> (-500, +750) pt/s!
		writeWheelScrollOp(w, 7, true, 8, 12, 30, 40, 130, 140, 4, 2000)
		n++
		writeWheelScrollOp(w, 7, true, 8, 12, 30, 40, 130, 140, 4, 2016)
		n++
		writeWheelScrollOp(w, 7, true, 8, 12, 30, 40, 130, 140, 4, 2032)
		n++
		writeWheelScrollOp(w, 7, true, 8, 12, 30, 40, 130, 140, 4, 2048)
		n++
		writeWheelEndOp(w)
		n++
		// 3. Touchpad pause before lift: gap > 100ms drops old samples, leaving 1 sample -> velocity (0, 0).
		writeWheelScrollOp(w, 9, true, 0, 20, 10, 20, 10, 20, 0, 3000)
		n++
		writeWheelScrollOp(w, 9, true, 0, 1, 10, 20, 10, 20, 0, 3200)
		n++
		writeWheelEndOp(w)
		n++
		// 4. Touchpad interrupted by node deletion (cancel) emits no GestureEnded.
		writeWheelScrollOp(w, 11, true, 0, 15, 5, 5, 5, 5, 0, 4000)
		n++
		writeWheelScrollOp(w, 11, true, 0, 15, 5, 5, 5, 5, 0, 4016)
		n++
		writeWheelCancelOp(w, 11)
		n++
		writeWheelEndOp(w)
		n++
		return n
	})

	if len(events) != 11 {
		t.Fatalf("got %d events, want 11: %+v", len(events), events)
	}

	// 1. Mouse wheel.
	m := events[0]
	if m.nodeID != 7 || m.phase != driver.GestureChanged || m.touchpad || m.dx != -99 || m.dy != 99 || m.vx != 0 || m.vy != 0 || m.mods != 1 {
		t.Errorf("mouse wheel event = %+v, want node 7 GestureChanged touchpad=false d=(-99, 99) v=(0,0) mods=1", m)
	}

	// 2. Touchpad swipe: Began -> Changed -> Changed -> Changed -> Ended with release velocity (-500, +750) pt/s.
	if events[1].phase != driver.GestureBegan || !events[1].touchpad || events[1].dx != -8 || events[1].dy != 12 {
		t.Errorf("touchpad began = %+v", events[1])
	}
	for i := 2; i <= 4; i++ {
		if events[i].phase != driver.GestureChanged || !events[i].touchpad || events[i].dx != -8 || events[i].dy != 12 {
			t.Errorf("touchpad changed[%d] = %+v", i, events[i])
		}
	}
	end := events[5]
	if end.phase != driver.GestureEnded || !end.touchpad || end.dx != 0 || end.dy != 0 {
		t.Errorf("touchpad ended = %+v", end)
	}
	if math.Abs(float64(end.vx+500)) > 1 || math.Abs(float64(end.vy-750)) > 1 {
		t.Errorf("touchpad momentum release velocity = (%v, %v), want (-500, 750) pt/s", end.vx, end.vy)
	}
	if end.x != 30 || end.y != 40 || end.pageX != 130 || end.pageY != 140 || end.mods != 4 {
		t.Errorf("touchpad ended coords/mods = %+v", end)
	}

	// 3. Paused touchpad ends with zero velocity.
	pausedEnd := events[8]
	if pausedEnd.nodeID != 9 || pausedEnd.phase != driver.GestureEnded || pausedEnd.vx != 0 || pausedEnd.vy != 0 {
		t.Errorf("paused touchpad ended = %+v, want zero velocity", pausedEnd)
	}

	// 4. Cancelled node emitted only Began + Changed, no Ended.
	if events[9].nodeID != 11 || events[9].phase != driver.GestureBegan || events[10].phase != driver.GestureChanged {
		t.Errorf("cancelled touchpad events = %+v, %+v", events[9], events[10])
	}
}

func TestDecodeWheelEventCarriesDeltaAndMomentumVelocity(t *testing.T) {
	// Encode an EVENT_WHEEL exactly as NsgoView.emitWheel writes it, then
	// decode through readEvent + decodeWheelEvent.
	var w writer
	w.u32(0) // primary surface
	w.byte(byte(driver.EventWheel))
	w.u32(42)                         // node ID
	w.u32(0)                          // count
	w.points(18)                      // Point.X
	w.points(24)                      // Point.Y
	w.str("")                         // Text
	w.points(-500)                    // Size.Width -> Velocity.X
	w.points(750)                     // Size.Height -> Velocity.Y
	w.u32(0)                          // Index
	w.byte(byte(driver.GestureEnded)) // Phase
	w.points(0)                       // Velocity.X -> Delta.X
	w.points(0)                       // Velocity.Y -> Delta.Y
	w.float(1)                        // Value -> Precise = true
	w.byte(0)                         // Navigation flags (none)
	w.u64(0)                          // Time
	w.u64(0)                          // Duration
	w.byte(0)                         // Buffering
	w.str("")                         // Data
	w.byte(0)                         // Finished
	w.points(118)                     // Page.X
	w.points(124)                     // Page.Y
	w.points(0)                       // Position.X
	w.points(0)                       // Position.Y
	w.byte(5)                         // Modifiers: Control | Shift

	from, ev, _ := readEvent(&reader{w.buf})
	if from != 0 || ev.Kind != driver.EventWheel {
		t.Fatalf("readEvent = (from=%d, kind=%v)", from, ev.Kind)
	}
	decodeWheelEvent(&ev)
	if ev.ID != 42 || ev.Phase != driver.GestureEnded || !ev.Precise {
		t.Errorf("decoded header = %+v", ev)
	}
	if ev.Point != (driver.Point{X: 18, Y: 24}) || ev.Page != (driver.Point{X: 118, Y: 124}) {
		t.Errorf("decoded points = point:%+v page:%+v", ev.Point, ev.Page)
	}
	if ev.Delta != (driver.Point{}) || ev.Velocity != (driver.Point{X: -500, Y: 750}) || ev.Size != (driver.Size{}) || ev.Value != 0 {
		t.Errorf("decoded wheel motion = delta:%+v velocity:%+v size:%+v value:%v", ev.Delta, ev.Velocity, ev.Size, ev.Value)
	}
	if !ev.Modifiers.Control || !ev.Modifiers.Shift || ev.Modifiers.Alt || ev.Modifiers.Platform {
		t.Errorf("decoded modifiers = %+v", ev.Modifiers)
	}
}
