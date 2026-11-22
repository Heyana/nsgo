package ios

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestScrollTypeMaskConstants(t *testing.T) {
	// UIKit's UIPanGestureRecognizer.h defines:
	//   UIScrollTypeDiscrete = 0 -> UIScrollTypeMaskDiscrete = 1 << 0 (1)
	//   UIScrollTypeContinuous = 1 -> UIScrollTypeMaskContinuous = 1 << 1 (2)
	if scrollTypeMaskDiscrete != 1 {
		t.Fatalf("scrollTypeMaskDiscrete = %d, want 1", scrollTypeMaskDiscrete)
	}
	if scrollTypeMaskContinuous != 2 {
		t.Fatalf("scrollTypeMaskContinuous = %d, want 2", scrollTypeMaskContinuous)
	}
}

func TestWheelGestureEventContinuousAndDiscrete(t *testing.T) {
	at := driver.Point{X: 42, Y: 77}
	page := driver.Point{X: 142, Y: 177}
	mods := driver.Modifiers{Shift: true, Platform: true}

	// Continuous trackpad: Began -> Changed -> Ended (with Velocity) / Cancelled (zero Velocity).
	ev, ok := wheelGestureEvent(stateBegan, true, at, page, driver.Point{X: 5, Y: -8}, driver.Point{X: 99, Y: 99}, mods)
	if !ok {
		t.Fatal("continuous stateBegan dropped")
	}
	if ev.Kind != driver.EventWheel || ev.Phase != driver.GestureBegan || !ev.Precise {
		t.Errorf("continuous began = %+v, want EventWheel GestureBegan Precise=true", ev)
	}
	if ev.Point != at || ev.Page != page || ev.Delta != (driver.Point{X: 5, Y: -8}) || ev.Modifiers != mods {
		t.Errorf("continuous began fields = %+v", ev)
	}
	if ev.Velocity != (driver.Point{}) {
		t.Errorf("continuous began Velocity = %+v, want zero", ev.Velocity)
	}

	ev, ok = wheelGestureEvent(stateEnded, true, at, page, driver.Point{}, driver.Point{X: 320, Y: -480}, mods)
	if !ok {
		t.Fatal("continuous stateEnded dropped")
	}
	if ev.Phase != driver.GestureEnded || !ev.Precise || ev.Velocity != (driver.Point{X: 320, Y: -480}) {
		t.Errorf("continuous ended = %+v, want GestureEnded with Velocity (320, -480)", ev)
	}

	ev, ok = wheelGestureEvent(stateCancelled, true, at, page, driver.Point{}, driver.Point{X: 500, Y: 500}, mods)
	if !ok {
		t.Fatal("continuous stateCancelled dropped")
	}
	if ev.Phase != driver.GestureCancelled || ev.Velocity != (driver.Point{}) {
		t.Errorf("continuous cancelled = %+v, want GestureCancelled with zero Velocity", ev)
	}

	// Discrete mouse wheel: Began reports GestureChanged, zero-delta Ended is ignored, non-zero Ended reports GestureChanged.
	ev, ok = wheelGestureEvent(stateBegan, false, at, page, driver.Point{Y: 120}, driver.Point{}, driver.Modifiers{Control: true})
	if !ok || ev.Phase != driver.GestureChanged || ev.Precise || ev.Delta.Y != 120 || !ev.Modifiers.Control {
		t.Errorf("discrete began = (%+v, %v), want GestureChanged Precise=false Delta.Y=120 Control", ev, ok)
	}
	if _, ok := wheelGestureEvent(stateEnded, false, at, page, driver.Point{}, driver.Point{}, driver.Modifiers{}); ok {
		t.Error("discrete zero-delta stateEnded should not report")
	}
	ev, ok = wheelGestureEvent(stateEnded, false, at, page, driver.Point{Y: -40}, driver.Point{}, driver.Modifiers{})
	if !ok || ev.Phase != driver.GestureChanged || ev.Delta.Y != -40 {
		t.Errorf("discrete non-zero stateEnded = (%+v, %v), want GestureChanged Delta.Y=-40", ev, ok)
	}
	if _, ok := wheelGestureEvent(stateCancelled, false, at, page, driver.Point{Y: 10}, driver.Point{}, driver.Modifiers{}); ok {
		t.Error("discrete stateCancelled should not report")
	}
}

func TestPinchLocationPreservesLastWhenTouchesLift(t *testing.T) {
	last := driver.Point{X: 88, Y: 64}
	if got := pinchLocation(2, driver.Point{X: 90, Y: 66}, last); got != (driver.Point{X: 90, Y: 66}) {
		t.Fatalf("active touches location = %+v, want (90, 66)", got)
	}
	if got := pinchLocation(0, driver.Point{}, last); got != last {
		t.Fatalf("zero touches location = %+v, want last %+v", got, last)
	}
}
