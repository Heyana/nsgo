package windows

import (
	"math"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestPreciseScrollSteps(t *testing.T) {
	cases := []struct {
		name      string
		delta     driver.Point
		wantSteps int
	}{
		{name: "zero", delta: driver.Point{}, wantSteps: 1},
		{name: "small", delta: driver.Point{X: 10, Y: -5}, wantSteps: 1},
		{name: "vertical 80", delta: driver.Point{Y: 80}, wantSteps: 5},
		{name: "diagonal -40,-60", delta: driver.Point{X: -40, Y: -60}, wantSteps: 4},
		{name: "non-divisible 50", delta: driver.Point{Y: 50}, wantSteps: 3},
		{name: "fractional", delta: driver.Point{X: 37.5, Y: -12.25}, wantSteps: 2},
		{name: "large", delta: driver.Point{X: -123.5, Y: 215.25}, wantSteps: 11},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			steps := preciseScrollSteps(tc.delta)
			if len(steps) != tc.wantSteps {
				t.Fatalf("len(preciseScrollSteps(%v)) = %d, want %d", tc.delta, len(steps), tc.wantSteps)
			}
			var sum driver.Point
			for i, s := range steps {
				if absPoints(s.X) > preciseScrollStepMax+1e-4 || absPoints(s.Y) > preciseScrollStepMax+1e-4 {
					t.Fatalf("step %d = %v exceeds max step %v", i, s, preciseScrollStepMax)
				}
				sum.X += s.X
				sum.Y += s.Y
			}
			if math.Abs(float64(sum.X-tc.delta.X)) > 1e-4 || math.Abs(float64(sum.Y-tc.delta.Y)) > 1e-4 {
				t.Fatalf("sum of steps = %v, want %v", sum, tc.delta)
			}
		})
	}
}

func TestPreciseWheelEvent(t *testing.T) {
	local := driver.Point{X: 25, Y: 35}
	page := driver.Point{X: 125, Y: 235}
	mods := driver.Modifiers{Shift: true, Control: true}

	began := preciseWheelEvent(7, driver.GestureBegan, local, page, driver.Point{X: -10, Y: -15}, mods)
	if began.Kind != driver.EventWheel || began.ID != 7 || began.Phase != driver.GestureBegan || !began.Precise {
		t.Fatalf("began = %+v", began)
	}
	if began.Delta != (driver.Point{}) {
		t.Fatalf("began delta = %v, want zero", began.Delta)
	}
	if began.Point != local || began.Page != page || began.Modifiers != mods {
		t.Fatalf("began metadata = (%v, %v, %+v)", began.Point, began.Page, began.Modifiers)
	}

	changed := preciseWheelEvent(7, driver.GestureChanged, local, page, driver.Point{X: -40, Y: -60}, mods)
	if changed.Phase != driver.GestureChanged || !changed.Precise {
		t.Fatalf("changed = %+v", changed)
	}
	if changed.Delta != (driver.Point{X: 40, Y: 60}) {
		t.Fatalf("changed delta = %v, want {40 60}", changed.Delta)
	}

	// Zero components do not produce -0.
	axisOnly := preciseWheelEvent(7, driver.GestureChanged, local, page, driver.Point{Y: 20}, driver.Modifiers{})
	if math.Signbit(float64(axisOnly.Delta.X)) || axisOnly.Delta != (driver.Point{Y: -20}) {
		t.Fatalf("axisOnly delta = %v, want {0 -20}", axisOnly.Delta)
	}

	ended := preciseWheelEvent(7, driver.GestureEnded, local, page, driver.Point{X: -10, Y: -15}, mods)
	if ended.Phase != driver.GestureEnded || !ended.Precise || ended.Delta != (driver.Point{}) {
		t.Fatalf("ended = %+v", ended)
	}
}

func TestWheelNotchDeltaAndUnits(t *testing.T) {
	if got := wheelNotchDelta(-1, false); got != (driver.Point{Y: wheelStep}) {
		t.Fatalf("vertical down 1 notch = %v, want {0 %v}", got, wheelStep)
	}
	if got := wheelNotchDelta(1, false); got != (driver.Point{Y: -wheelStep}) {
		t.Fatalf("vertical up 1 notch = %v, want {0 %v}", got, -wheelStep)
	}
	if got := wheelNotchDelta(1, true); got != (driver.Point{X: wheelStep}) {
		t.Fatalf("tilt right 1 notch = %v, want {%v 0}", got, wheelStep)
	}
	if got := wheelNotchDelta(0, false); got != (driver.Point{}) {
		t.Fatalf("zero notches = %v, want zero", got)
	}

	if got := wheelPointsToUnits(wheelStep); got != 120 {
		t.Fatalf("wheelPointsToUnits(%v) = %d, want 120", wheelStep, got)
	}
	if got := wheelPointsToUnits(-wheelStep); got != -120 {
		t.Fatalf("wheelPointsToUnits(%v) = %d, want -120", -wheelStep, got)
	}
	if got := wheelPointsToUnits(1e9); got != math.MaxInt16 {
		t.Fatalf("wheelPointsToUnits(1e9) = %d, want MaxInt16", got)
	}
	if got := wheelPointsToUnits(-1e9); got != math.MinInt16 {
		t.Fatalf("wheelPointsToUnits(-1e9) = %d, want MinInt16", got)
	}
}

func TestPickScrollCandidateAxesAndNesting(t *testing.T) {
	vertical := scrollCandidate{
		frame:   driver.Size{Width: 100, Height: 100},
		content: driver.Size{Width: 100, Height: 400},
	}
	horizontal := scrollCandidate{
		scroll:  driver.ScrollConfig{Horizontal: true},
		frame:   driver.Size{Width: 80, Height: 80},
		content: driver.Size{Width: 400, Height: 80},
	}

	// Vertical delta over nested vertical + horizontal picks the vertical view.
	under := []scrollCandidate{vertical, horizontal}
	if idx, to, fb := pickScrollCandidate(under, driver.Point{Y: 80}); idx != 0 || to != (driver.Point{Y: 80}) || fb {
		t.Fatalf("vertical over nested = (%d, %v, %v), want (0, {0 80}, false)", idx, to, fb)
	}
	// Horizontal delta over nested vertical + horizontal picks the horizontal view.
	if idx, to, fb := pickScrollCandidate(under, driver.Point{X: 75}); idx != 1 || to != (driver.Point{X: 75}) || fb {
		t.Fatalf("horizontal over nested = (%d, %v, %v), want (1, {75 0}, false)", idx, to, fb)
	}
	// Diagonal predominantly vertical picks vertical; predominantly horizontal picks horizontal.
	if idx, to, fb := pickScrollCandidate(under, driver.Point{X: 20, Y: 60}); idx != 0 || to != (driver.Point{Y: 60}) || fb {
		t.Fatalf("diagonal vertical = (%d, %v, %v), want (0, {0 60}, false)", idx, to, fb)
	}
	if idx, to, fb := pickScrollCandidate(under, driver.Point{X: 60, Y: 20}); idx != 1 || to != (driver.Point{X: 60}) || fb {
		t.Fatalf("diagonal horizontal = (%d, %v, %v), want (1, {60 0}, false)", idx, to, fb)
	}
	// At the vertical view's end, a vertical scroll does not spill into the horizontal view inside it.
	atEnd := []scrollCandidate{vertical, horizontal}
	atEnd[0].offset.Y = 300
	if idx, _, _ := pickScrollCandidate(atEnd, driver.Point{Y: 40}); idx != -1 {
		t.Fatalf("vertical at end = %d, want -1", idx)
	}
	// Horizontal scroll over a lone vertical view does not scroll vertically.
	if idx, _, _ := pickScrollCandidate([]scrollCandidate{vertical}, driver.Point{X: 40}); idx != -1 {
		t.Fatalf("horizontal over lone vertical = %d, want -1", idx)
	}
	// Vertical scroll over a lone horizontal view falls back to scrolling horizontally.
	if idx, to, fb := pickScrollCandidate([]scrollCandidate{horizontal}, driver.Point{Y: 55}); idx != 0 || to != (driver.Point{X: 55}) || !fb {
		t.Fatalf("vertical over lone horizontal = (%d, %v, %v), want (0, {55 0}, true)", idx, to, fb)
	}

	// Nested vertical scroll views: inner scrolls first, outer takes over when inner is at its edge.
	inner := scrollCandidate{
		frame:   driver.Size{Width: 100, Height: 100},
		content: driver.Size{Width: 100, Height: 150},
	}
	nested := []scrollCandidate{vertical, inner}
	if idx, to, _ := pickScrollCandidate(nested, driver.Point{Y: 30}); idx != 1 || to != (driver.Point{Y: 30}) {
		t.Fatalf("nested inner = (%d, %v), want (1, {0 30})", idx, to)
	}
	nested[1].offset.Y = 50
	if idx, to, _ := pickScrollCandidate(nested, driver.Point{Y: 30}); idx != 0 || to != (driver.Point{Y: 30}) {
		t.Fatalf("nested inner at edge = (%d, %v), want (0, {0 30})", idx, to)
	}

	// NoScroll inner view is skipped in favor of outer scrollable view.
	locked := inner
	locked.scroll.Behavior.NoScroll = true
	if idx, to, _ := pickScrollCandidate([]scrollCandidate{vertical, locked}, driver.Point{Y: 25}); idx != 0 || to != (driver.Point{Y: 25}) {
		t.Fatalf("locked inner = (%d, %v), want (0, {0 25})", idx, to)
	}

	// ContentInset widens the clamped offset range precisely.
	inset := scrollCandidate{
		scroll:  driver.ScrollConfig{Behavior: driver.ScrollBehavior{ContentInset: driver.Edges{Top: 20, Bottom: 30}}},
		frame:   driver.Size{Width: 100, Height: 100},
		content: driver.Size{Width: 100, Height: 200},
		offset:  driver.Point{Y: -20},
	}
	if to, ok := scrollCandidateOffset(inset, driver.Point{Y: 50}, false); !ok || to != (driver.Point{Y: 30}) {
		t.Fatalf("inset scroll = (%v, %v), want ({0 30}, true)", to, ok)
	}
	if to, ok := scrollCandidateOffset(inset, driver.Point{Y: 500}, false); !ok || to != (driver.Point{Y: 130}) {
		t.Fatalf("inset clamp = (%v, %v), want ({0 130}, true)", to, ok)
	}
}
