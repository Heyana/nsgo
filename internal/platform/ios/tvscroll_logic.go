package ios

import "github.com/nsgo-dev/nsgo/internal/driver"

// This file builds on every system, so host tests check tvOS remote-surface
// scroll eligibility and touch-type rules directly.

// UITouchType values from UIKit's UITouch.h.
const (
	touchTypeDirect   = 0
	touchTypeIndirect = 1
)

// tvScrollCanPan reports whether a scroll view has content overflowing its
// viewport along a scrollable axis (and scrolling is enabled).
func tvScrollCanPan(horizontal, zooms, noScroll bool, contentSize, boundsSize driver.Size, insetTop, insetLeft, insetBottom, insetRight driver.Points) bool {
	if noScroll || boundsSize.Width <= 0 || boundsSize.Height <= 0 {
		return false
	}
	const slack = 0.5
	canX := contentSize.Width+insetLeft+insetRight > boundsSize.Width+slack
	canY := contentSize.Height+insetTop+insetBottom > boundsSize.Height+slack
	if zooms {
		return canX || canY
	}
	if horizontal {
		return canX
	}
	return canY
}

// tvScrollAllowsIndirectPan reports whether a UIScrollView's
// panGestureRecognizer should accept UITouchTypeIndirect (the Apple TV
// remote's touch surface): only on tvOS when the scroll view is eligible,
// overflows its viewport, and contains no interactive focusable descendants.
// Leaving UITouchTypeIndirect off when focusable children exist preserves
// native tvOS focus-driven scrolling without gesture interference.
func tvScrollAllowsIndirectPan(isTV, eligible, canPan, hasInteractiveFocus bool) bool {
	return isTV && eligible && canPan && !hasInteractiveFocus
}

// tvScrollCanBecomeFocused reports whether a UIScrollView itself should
// participate as a tvOS focus item so remote swipes target it: only when it
// allows indirect panning and has no focusable nested scroll view that should
// take focus instead.
func tvScrollCanBecomeFocused(isTV, eligible, canPan, hasInteractiveFocus, hasFocusableScroll bool) bool {
	return tvScrollAllowsIndirectPan(isTV, eligible, canPan, hasInteractiveFocus) && !hasFocusableScroll
}

// tvScrollPanAllowedTouchTypes returns the allowedTouchTypes array to assign
// to UIScrollView.panGestureRecognizer on tvOS given UIKit's default touch
// types and whether indirect scrolling is enabled.
func tvScrollPanAllowedTouchTypes(defaults []int, indirect bool) []int {
	if !indirect {
		if len(defaults) == 0 {
			return nil
		}
		out := make([]int, len(defaults))
		copy(out, defaults)
		return out
	}
	out := make([]int, 0, len(defaults)+1)
	hasIndirect := false
	for _, t := range defaults {
		out = append(out, t)
		if t == touchTypeIndirect {
			hasIndirect = true
		}
	}
	if !hasIndirect {
		out = append(out, touchTypeIndirect)
	}
	return out
}
