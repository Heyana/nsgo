package ios

import "github.com/nsgo-dev/nsgo/internal/driver"

// This file builds on every system, so host tests check tvOS remote press and focus-scope rules directly.

// UIPressType values of the Apple TV remote's buttons.
const (
	pressUpArrow    = 0
	pressDownArrow  = 1
	pressLeftArrow  = 2
	pressRightArrow = 3
	pressSelect     = 4
	pressMenu       = 5
)

// remoteArrowName maps a Siri Remote directional UIPressType (which has no hardware UIKey) to its keymap name.
func remoteArrowName(pressType int) string {
	switch pressType {
	case pressUpArrow:
		return "up"
	case pressDownArrow:
		return "down"
	case pressLeftArrow:
		return "left"
	case pressRightArrow:
		return "right"
	default:
		return ""
	}
}

// isTopmostCaptureProps reports whether a Topmost overlay captures pointer/focus input itself rather than letting it pass through to the app below.
func isTopmostCaptureProps(p driver.ViewProps) bool {
	return p.Topmost && p.PointerEvents != driver.PointerEventsBoxNone && p.PointerEvents != driver.PointerEventsNone
}

// tvMenuGoesBack reports whether the Apple TV remote's Menu button goes back in the app rather than exiting to the home screen.
func tvMenuGoesBack(history int, hasTopModal, hasTopmostCapture bool) bool {
	return history > 0 || hasTopModal || hasTopmostCapture
}

// tvFocusAllowed reports whether a focusable view may take focus on Apple TV under the active capture overlay or modal.
func tvFocusAllowed(inCapture, hasCapture, inModal, hasModal, inTopmost bool) bool {
	if hasCapture {
		return inCapture
	}
	if hasModal {
		return inModal || inTopmost
	}
	return true
}
