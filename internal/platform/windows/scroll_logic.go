package windows

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/scrollbar"
)

// wheelStep is how far one notch of a mouse wheel scrolls a native scroll view or multiline editor.
const wheelStep driver.Points = 48

// preciseScrollStepMax is the farthest one step of a precise test scroll moves along either axis, matching macOS and Linux.
const preciseScrollStepMax driver.Points = 20

func absPoints(v driver.Points) driver.Points {
	if v < 0 {
		return -v
	}
	return v
}

// preciseScrollSteps splits a total scroll-offset delta into steps of at most preciseScrollStepMax along either axis whose sum equals delta.
func preciseScrollSteps(delta driver.Point) []driver.Point {
	n := int(max(absPoints(delta.X), absPoints(delta.Y))/preciseScrollStepMax) + 1
	steps := make([]driver.Point, n)
	var prev driver.Point
	for i := range n {
		var next driver.Point
		if i == n-1 {
			next = delta
		} else {
			f := driver.Points(i+1) / driver.Points(n)
			next = driver.Point{X: delta.X * f, Y: delta.Y * f}
		}
		steps[i] = driver.Point{X: next.X - prev.X, Y: next.Y - prev.Y}
		prev = next
	}
	return steps
}

// wheelNotchDelta converts a mouse wheel's notches (positive away from the user or tilted right) into the scroll-offset delta in points.
func wheelNotchDelta(notches driver.Points, tilt bool) driver.Point {
	if notches == 0 {
		return driver.Point{}
	}
	if tilt {
		return driver.Point{X: notches * wheelStep}
	}
	return driver.Point{Y: -notches * wheelStep}
}

// wheelPointsToUnits converts a point delta into signed Win32 wheel units (120 per wheelStep) for forwarding to hosted WebView2 pages.
func wheelPointsToUnits(points driver.Points) int16 {
	u := math.Round(float64(points) * 120 / float64(wheelStep))
	if u > math.MaxInt16 {
		return math.MaxInt16
	}
	if u < math.MinInt16 {
		return math.MinInt16
	}
	return int16(u)
}

// preciseWheelEvent builds a touchpad EventWheel for a precise scroll gesture phase. offsetDelta is the scroll-offset change (positive right and down), which is inverted into Event.Delta (content movement, positive right and down).
func preciseWheelEvent(id driver.NodeID, phase driver.GesturePhase, local, page, offsetDelta driver.Point, mods driver.Modifiers) driver.Event {
	ev := driver.Event{
		Kind:      driver.EventWheel,
		ID:        id,
		Phase:     phase,
		Point:     local,
		Page:      page,
		Modifiers: mods,
		Precise:   true,
	}
	if phase == driver.GestureChanged {
		if offsetDelta.X != 0 {
			ev.Delta.X = -offsetDelta.X
		}
		if offsetDelta.Y != 0 {
			ev.Delta.Y = -offsetDelta.Y
		}
	}
	return ev
}

// scrollCandidate is the geometry and configuration of a native scroll view considered for a wheel or precise scroll.
type scrollCandidate struct {
	scroll  driver.ScrollConfig
	frame   driver.Size
	content driver.Size
	offset  driver.Point
}

// scrollCandidateOffset applies offset delta to c along its axis, clamped to its content and insets, and reports whether the offset moved. When horizontalFallback is true, a horizontal view takes delta.Y if delta.X is zero, matching vertical wheel turns over a lone horizontal scroll view.
func scrollCandidateOffset(c scrollCandidate, delta driver.Point, horizontalFallback bool) (driver.Point, bool) {
	if c.scroll.Behavior.NoScroll {
		return c.offset, false
	}
	offset := c.offset
	if c.scroll.Horizontal {
		d := delta.X
		if d == 0 && horizontalFallback {
			d = delta.Y
		}
		if d == 0 {
			return c.offset, false
		}
		offset.X += d
	} else {
		if delta.Y == 0 {
			return c.offset, false
		}
		offset.Y += delta.Y
	}
	to := scrollbar.Clamp(&c.scroll, c.frame, c.content, offset)
	return to, to != c.offset
}

// pickScrollCandidate returns the index of the innermost scroll candidate in under (outermost first) that can move by delta, its next clamped offset, and whether a lone horizontal view fell back to delta.Y. Views along the dominant scroll axis take the scroll first; horizontal views take a vertical scroll only where no vertical view is in under.
func pickScrollCandidate(under []scrollCandidate, delta driver.Point) (index int, to driver.Point, fallback bool) {
	if delta.X == 0 && delta.Y == 0 {
		return -1, driver.Point{}, false
	}
	horizontal := absPoints(delta.X) > absPoints(delta.Y)
	along := false
	for _, c := range under {
		if c.scroll.Horizontal == horizontal {
			along = true
			break
		}
	}
	for i := len(under) - 1; i >= 0; i-- {
		c := under[i]
		if c.scroll.Behavior.NoScroll || along && c.scroll.Horizontal != horizontal {
			continue
		}
		if next, ok := scrollCandidateOffset(c, delta, !along); ok {
			return i, next, !along
		}
	}
	return -1, driver.Point{}, false
}
