package linux

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// wheelNotch is how far the content under a view taking the wheel moves
// for a discrete wheel notch: three lines of 33 points, as browsers scroll
// about 100.
const wheelNotch = 99.0

// wheelScrollUnits reports whether a GTK scroll event's deltas are discrete
// wheel notches (true) or surface points from a touchpad (false). When GTK 4.8+
// reports the scroll unit authoritatively (unitKnown), inPoints decides
// directly; on older GTK, deltas of at most 3 units are taken as notches.
func wheelScrollUnits(dx, dy float64, inPoints, unitKnown bool) bool {
	if unitKnown {
		return !inPoints
	}
	return !inPoints && math.Abs(dx) <= 3 && math.Abs(dy) <= 3
}

// wheelShiftAxis maps a vertical scroll to the horizontal axis when Shift is
// held and no horizontal delta is already present.
func wheelShiftAxis(dx, dy float64, shift bool) (float64, float64) {
	if shift && dx == 0 {
		return dy, 0
	}
	return dx, dy
}

// wheelPhase returns the gesture phase for a wheel or touchpad scroll step:
// a touchpad gesture begins on its first event and changes on subsequent ones;
// a discrete wheel always reports GestureChanged.
func wheelPhase(touchpad, began bool) driver.GesturePhase {
	if touchpad && began {
		return driver.GestureBegan
	}
	return driver.GestureChanged
}

// wheelContentDelta converts GTK scroll deltas (positive down and right) into
// WheelEvent.Delta (how far content moves, positive down and right).
func wheelContentDelta(dx, dy float64, units bool) driver.Point {
	if units {
		dx *= wheelNotch
		dy *= wheelNotch
	}
	return driver.Point{X: driver.Points(-dx), Y: driver.Points(-dy)}
}
