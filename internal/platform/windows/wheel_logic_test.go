package windows

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWheelEventValues(t *testing.T) {
	// Vertical mouse wheel away from user (+120): moves content down (+99), discrete.
	d, precise, mods := wheelEventValues(120, false, 0, driver.Modifiers{})
	if d != (driver.Point{Y: 99}) || precise || mods != (driver.Modifiers{}) {
		t.Fatalf("vertical +120 = (%+v, %v, %+v), want ({0 99}, false, {})", d, precise, mods)
	}

	// Vertical mouse wheel toward user (-120): moves content up (-99), discrete.
	d, precise, _ = wheelEventValues(-120, false, 0, driver.Modifiers{})
	if d != (driver.Point{Y: -99}) || precise {
		t.Fatalf("vertical -120 = (%+v, %v), want ({0 -99}, false)", d, precise)
	}

	// Horizontal wheel tilt right (+120): shows what is right, moving content left (-99).
	d, precise, _ = wheelEventValues(120, true, 0, driver.Modifiers{})
	if d != (driver.Point{X: -99}) || precise {
		t.Fatalf("horizontal +120 = (%+v, %v), want ({-99 0}, false)", d, precise)
	}

	// Precision touchpad sub-notch delta (+20) with MK_CONTROL + MK_SHIFT in wParam.
	d, precise, mods = wheelEventValues(20, false, wheelMKControl|wheelMKShift, driver.Modifiers{Alt: true})
	if !precise {
		t.Fatal("sub-notch delta 20 reported precise = false, want true")
	}
	wantY := driver.Points(20) / 120 * wheelNotch
	if math.Abs(float64(d.Y-wantY)) > 1e-4 {
		t.Fatalf("sub-notch delta Y = %v, want %v", d.Y, wantY)
	}
	if !mods.Control || !mods.Shift || !mods.Alt {
		t.Fatalf("merged mods = %+v, want Control+Shift+Alt", mods)
	}
}

func TestZoomScale(t *testing.T) {
	if got := zoomScale(150, 100, 1); math.Abs(float64(got-1.5)) > 1e-6 {
		t.Fatalf("zoomScale(150, 100) = %v, want 1.5", got)
	}
	// GF_END with zero arguments must keep the last scale instead of snapping to 0.
	if got := zoomScale(0, 100, 1.75); got != 1.75 {
		t.Fatalf("zoomScale(0, 100, 1.75) = %v, want 1.75", got)
	}
	if got := zoomScale(150, 0, 1); got != 1 {
		t.Fatalf("zoomScale(150, 0, 1) = %v, want 1", got)
	}
}
