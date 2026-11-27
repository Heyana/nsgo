package watchos

import (
	"errors"
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWatchSimulatorEnvAndNativeHIDDecision(t *testing.T) {
	if watchIsSimulatorEnv(nil) {
		t.Error("nil getenv reported simulator")
	}
	if watchIsSimulatorEnv(func(string) string { return "" }) {
		t.Error("empty env reported simulator")
	}
	for _, key := range []string{"SIMULATOR_UDID", "SIMULATOR_MODEL_IDENTIFIER", "SIMULATOR_ROOT"} {
		if !watchIsSimulatorEnv(func(k string) string {
			if k == key {
				return "watch-sim"
			}
			return ""
		}) {
			t.Errorf("%s did not detect simulator", key)
		}
	}

	loadCalled := false
	okLoad := func() error { loadCalled = true; return nil }
	okWin := func() bool { return true }

	// Physical watch never calls load.
	if watchShouldUseNativeHID(false, false, okLoad, okWin) {
		t.Error("physical device enabled native simulator HID")
	}
	if loadCalled {
		t.Error("physical device called loadWatchHID")
	}

	// Broken HID flag disables native HID without retrying.
	if watchShouldUseNativeHID(true, true, okLoad, okWin) {
		t.Error("broken flag still enabled native HID")
	}
	if loadCalled {
		t.Error("broken flag called loadWatchHID")
	}

	// Load error falls back cleanly.
	if watchShouldUseNativeHID(true, false, func() error { return errors.New("missing") }, okWin) {
		t.Error("load error enabled native HID")
	}

	// Missing key window falls back cleanly.
	if watchShouldUseNativeHID(true, false, okLoad, func() bool { return false }) {
		t.Error("missing window enabled native HID")
	}

	// Simulator + loaded HID + hosting window enables native HID.
	if !watchShouldUseNativeHID(true, false, okLoad, okWin) {
		t.Error("ready simulator did not enable native HID")
	}
}

func TestWatchTouchPhaseHIDAndDragSteps(t *testing.T) {
	touching, mask := watchTouchPhaseHID(watchHIDDown)
	if touching != 1 || mask != watchHIDMaskRange|watchHIDMaskTouch {
		t.Fatalf("down = (%d, %#x)", touching, mask)
	}
	touching, mask = watchTouchPhaseHID(watchHIDMove)
	if touching != 1 || mask != watchHIDMaskPosition {
		t.Fatalf("move = (%d, %#x)", touching, mask)
	}
	touching, mask = watchTouchPhaseHID(watchHIDUp)
	if touching != 0 || mask != watchHIDMaskRange|watchHIDMaskTouch {
		t.Fatalf("up = (%d, %#x)", touching, mask)
	}

	steps := watchDragSteps([]driver.Point{{X: 10, Y: 20}, {X: 10, Y: 55}})
	if len(steps) < 4 || steps[0] != (driver.Point{X: 10, Y: 20}) || steps[len(steps)-1] != (driver.Point{X: 10, Y: 55}) {
		t.Fatalf("steps = %v", steps)
	}
	for i := 1; i < len(steps); i++ {
		if d := math.Abs(float64(steps[i].Y - steps[i-1].Y)); d > 10.01 {
			t.Fatalf("step %d jumped %v points (>10)", i, d)
		}
	}
}

func TestPlanWatchScrollDrags(t *testing.T) {
	surface := driver.Size{Width: 184, Height: 224}
	scrollBounds := driver.Bounds{Origin: driver.Point{X: 0, Y: 40}, Size: driver.Size{Width: 184, Height: 184}}

	// Zero delta plans nothing.
	if segs := planWatchScrollDrags(surface, scrollBounds, driver.Point{X: 92, Y: 130}, driver.Point{}); len(segs) != 0 {
		t.Fatalf("zero delta planned %v", segs)
	}

	// Single vertical scroll down (delta.Y = +60 -> finger drags up by -60) stays safely inside scroll bounds.
	segs := planWatchScrollDrags(surface, scrollBounds, driver.Point{X: 92, Y: 132}, driver.Point{Y: 60})
	if len(segs) != 1 {
		t.Fatalf("small scroll planned %d segments, want 1: %v", len(segs), segs)
	}
	from, to := segs[0][0], segs[0][1]
	if math.Abs(float64((to.Y-from.Y)-(-60))) > 0.01 {
		t.Fatalf("finger travel Y = %v, want -60", to.Y-from.Y)
	}
	if from.Y < 52 || from.Y > 212 || to.Y < 52 || to.Y > 212 {
		t.Fatalf("segment [%v -> %v] escaped safe bounds [52, 212]", from, to)
	}

	// Large vertical scroll splits into multiple safe on-screen drags summing to -400.
	segs = planWatchScrollDrags(surface, scrollBounds, driver.Point{X: 92, Y: 132}, driver.Point{Y: 400})
	if len(segs) < 3 {
		t.Fatalf("large scroll planned %d segments, want >= 3: %v", len(segs), segs)
	}
	var totalY driver.Points
	for i, seg := range segs {
		a, b := seg[0], seg[1]
		if a.Y < 52 || a.Y > 212 || b.Y < 52 || b.Y > 212 {
			t.Fatalf("segment %d [%v -> %v] escaped safe bounds [52, 212]", i, a, b)
		}
		totalY += b.Y - a.Y
	}
	if math.Abs(float64(totalY-(-400))) > 0.5 {
		t.Fatalf("summed finger travel Y = %v, want -400", totalY)
	}

	// Horizontal scroll moves along X in the opposite direction of delta.X.
	segs = planWatchScrollDrags(surface, scrollBounds, driver.Point{X: 92, Y: 132}, driver.Point{X: -80})
	if len(segs) != 1 || math.Abs(float64((segs[0][1].X-segs[0][0].X)-80)) > 0.01 {
		t.Fatalf("horizontal scroll = %v, want +80 X travel", segs)
	}
}

func TestResolveWatchDragFallback(t *testing.T) {
	start := driver.Point{X: 90, Y: 150}
	end := driver.Point{X: 90, Y: 90}

	if _, ok := resolveWatchDragFallback(true, false, start, end); ok {
		t.Error("active OnPan still triggered fallback scroll")
	}
	if _, ok := resolveWatchDragFallback(false, true, start, end); ok {
		t.Error("blocked touch responder still triggered fallback scroll")
	}
	if _, ok := resolveWatchDragFallback(false, false, start, driver.Point{X: 90, Y: 145}); ok {
		t.Error("drag below slop triggered fallback scroll")
	}
	delta, ok := resolveWatchDragFallback(false, false, start, end)
	if !ok || delta != (driver.Point{X: 0, Y: 60}) {
		t.Fatalf("fallback scroll = (%v, %v), want ((0, 60), true)", delta, ok)
	}
}
