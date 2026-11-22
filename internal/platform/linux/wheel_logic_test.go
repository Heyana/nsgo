package linux

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWheelScrollUnits(t *testing.T) {
	// GTK 4.8+ authoritative unit reporting:
	if !wheelScrollUnits(0, 5, false, true) {
		t.Fatal("GTK 4.8+ wheel unit with fast spin (dy=5) must stay discrete units")
	}
	if wheelScrollUnits(0, 1, true, true) {
		t.Fatal("GTK 4.8+ surface unit with small delta (dy=1) must stay touchpad points")
	}

	// GTK < 4.8 fallback heuristic:
	if !wheelScrollUnits(0, 1, false, false) {
		t.Fatal("GTK < 4.8 dy=1 should be discrete units")
	}
	if wheelScrollUnits(0, 12, false, false) {
		t.Fatal("GTK < 4.8 dy=12 should be touchpad points")
	}
}

func TestWheelShiftAxis(t *testing.T) {
	dx, dy := wheelShiftAxis(0, 2, true)
	if dx != 2 || dy != 0 {
		t.Fatalf("Shift+vertical = (%v, %v), want (2, 0)", dx, dy)
	}
	// Preserve existing horizontal delta when Shift is held.
	dx, dy = wheelShiftAxis(5, 0, true)
	if dx != 5 || dy != 0 {
		t.Fatalf("Shift+horizontal = (%v, %v), want (5, 0)", dx, dy)
	}
}

func TestWheelPhaseAndDelta(t *testing.T) {
	if got := wheelPhase(true, true); got != driver.GestureBegan {
		t.Fatalf("touchpad first event phase = %v, want GestureBegan", got)
	}
	if got := wheelPhase(true, false); got != driver.GestureChanged {
		t.Fatalf("touchpad next event phase = %v, want GestureChanged", got)
	}
	if got := wheelPhase(false, true); got != driver.GestureChanged {
		t.Fatalf("mouse wheel phase = %v, want GestureChanged", got)
	}

	// GTK scroll down (+1 notch) moves content up (-99).
	if got := wheelContentDelta(0, 1, true); got != (driver.Point{Y: -99}) {
		t.Fatalf("notch down delta = %+v, want {0 -99}", got)
	}
	// GTK scroll right (+12pt touchpad) moves content left (-12pt).
	if got := wheelContentDelta(12, -8, false); got != (driver.Point{X: -12, Y: 8}) {
		t.Fatalf("touchpad delta = %+v, want {-12 8}", got)
	}
}
