package ios

import "github.com/nsgo-dev/nsgo/internal/driver"

// UIGestureRecognizerState values.
const (
	statePossible = iota
	stateBegan
	stateChanged
	stateEnded
	stateCancelled
	stateFailed
)

// UIScrollTypeMask values from UIKit's UIPanGestureRecognizer.h:
// UIScrollTypeDiscrete = 0 (mouse wheel), UIScrollTypeContinuous = 1 (trackpad).
const (
	scrollTypeMaskDiscrete   = 1 << 0
	scrollTypeMaskContinuous = 1 << 1
)

// wheelGestureEvent translates a scroll-only UIPanGestureRecognizer callback
// into a driver.EventWheel, returning false when the callback carries nothing
// to report (for example, the zero-delta end of a discrete mouse wheel turn).
func wheelGestureEvent(state int, continuous bool, at, page, delta, vel driver.Point, mods driver.Modifiers) (driver.Event, bool) {
	ev := driver.Event{
		Kind:      driver.EventWheel,
		Point:     at,
		Page:      page,
		Delta:     delta,
		Precise:   continuous,
		Modifiers: mods,
	}
	switch state {
	case stateBegan:
		ev.Phase = driver.GestureBegan
		if !continuous {
			ev.Phase = driver.GestureChanged
		}
	case stateChanged:
		ev.Phase = driver.GestureChanged
	case stateEnded:
		if !continuous {
			if delta.X == 0 && delta.Y == 0 {
				return driver.Event{}, false
			}
			ev.Phase = driver.GestureChanged
			return ev, true
		}
		ev.Phase = driver.GestureEnded
		ev.Velocity = vel
	case stateCancelled:
		if !continuous {
			return driver.Event{}, false
		}
		ev.Phase = driver.GestureCancelled
	default:
		return driver.Event{}, false
	}
	return ev, true
}

// pinchLocation preserves the last valid pinch focal point when UIKit reports
// stateEnded or stateCancelled with zero active touches (which makes
// locationInView: return the origin).
func pinchLocation(touches uint, at, last driver.Point) driver.Point {
	if touches > 0 {
		return at
	}
	return last
}
