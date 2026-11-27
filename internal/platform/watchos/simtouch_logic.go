package watchos

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// watchHIDPhase names the touch phases encoded into IOHID digitizer events on
// the watchOS simulator.
type watchHIDPhase uint8

const (
	watchHIDDown watchHIDPhase = iota
	watchHIDMove
	watchHIDUp
)

// IOHIDEvent digitizer constants shared with simtouch.go.
const (
	watchHIDTransducerHand   uint32 = 3
	watchHIDMaskRange        uint32 = 1 << 0
	watchHIDMaskTouch        uint32 = 1 << 1
	watchHIDMaskPosition     uint32 = 1 << 2
	watchHIDFieldIntegrated  uint32 = 11<<16 | 25
	watchHIDFieldMajorRadius uint32 = 11<<16 | 20
	watchHIDFieldMinorRadius uint32 = 11<<16 | 21
	watchHIDTestSenderID     uint64 = 0x4e53_474f // "NSGO"
)

// watchIsSimulatorEnv reports whether the process environment belongs to
// Xcode's watchOS simulator.
func watchIsSimulatorEnv(getenv func(string) string) bool {
	if getenv == nil {
		return false
	}
	return getenv("SIMULATOR_UDID") != "" || getenv("SIMULATOR_MODEL_IDENTIFIER") != "" || getenv("SIMULATOR_ROOT") != ""
}

// watchShouldUseNativeHID reports whether watchOS testSurface should inject
// IOHID digitizer touches into the simulator's hosting UIWindow.
func watchShouldUseNativeHID(isSim, broken bool, load func() error, hasWindow func() bool) bool {
	if !isSim || broken || load == nil || hasWindow == nil {
		return false
	}
	if load() != nil {
		return false
	}
	return hasWindow()
}

// watchTouchPhaseHID returns the IOHID touching flag and event mask for ph.
func watchTouchPhaseHID(ph watchHIDPhase) (touching uint8, mask uint32) {
	if ph != watchHIDUp {
		touching = 1
	}
	if ph == watchHIDMove {
		return touching, watchHIDMaskPosition
	}
	return touching, watchHIDMaskRange | watchHIDMaskTouch
}

// watchDragSteps expands path into steps of at most 10 points along each segment.
func watchDragSteps(path []driver.Point) []driver.Point {
	if len(path) == 0 {
		return nil
	}
	steps := []driver.Point{path[0]}
	at := path[0]
	for _, to := range path[1:] {
		n := int(math.Max(math.Abs(float64(to.X-at.X)), math.Abs(float64(to.Y-at.Y)))/10) + 1
		for i := 1; i <= n; i++ {
			f := driver.Points(i) / driver.Points(n)
			steps = append(steps, driver.Point{
				X: at.X + (to.X-at.X)*f,
				Y: at.Y + (to.Y-at.Y)*f,
			})
		}
		at = to
	}
	return steps
}

// planWatchScrollDrags plans one or more on-screen finger drags wholly inside
// the visible intersection of surface and scrollBounds whose finger travel sums
// to -delta (dragging a finger up scrolls content down). Keeping every segment
// inset from the screen and scroll edges prevents triggering watchOS edge
// gestures or clipping off-screen on compact watch displays.
func planWatchScrollDrags(surface driver.Size, scrollBounds driver.Bounds, at, delta driver.Point) [][]driver.Point {
	if delta.X == 0 && delta.Y == 0 {
		return nil
	}
	left := max(scrollBounds.Origin.X, 0)
	top := max(scrollBounds.Origin.Y, 0)
	right := scrollBounds.Origin.X + scrollBounds.Size.Width
	bottom := scrollBounds.Origin.Y + scrollBounds.Size.Height
	if surface.Width > 0 && right > surface.Width {
		right = surface.Width
	}
	if surface.Height > 0 && bottom > surface.Height {
		bottom = surface.Height
	}
	if right <= left || bottom <= top {
		return [][]driver.Point{{at, {X: at.X - delta.X, Y: at.Y - delta.Y}}}
	}
	const edgeMargin driver.Points = 12
	insetRange := func(lo, hi driver.Points) (driver.Points, driver.Points) {
		m := min(edgeMargin, (hi-lo)/4)
		return lo + m, hi - m
	}
	minX, maxX := insetRange(left, right)
	minY, maxY := insetRange(top, bottom)
	spanX, spanY := maxX-minX, maxY-minY
	if spanX < 1 && spanY < 1 {
		return [][]driver.Point{{at, {X: at.X - delta.X, Y: at.Y - delta.Y}}}
	}

	clampAxis := func(v, lo, hi driver.Points) driver.Points {
		return min(max(v, lo), hi)
	}
	baseX := clampAxis(at.X, minX, maxX)
	baseY := clampAxis(at.Y, minY, maxY)

	// Finger moves opposite to scroll content delta.
	remX, remY := -delta.X, -delta.Y
	var segs [][]driver.Point
	for range 64 {
		if math.Abs(float64(remX)) < 0.5 && math.Abs(float64(remY)) < 0.5 {
			break
		}
		var stepX, stepY driver.Points
		if remX != 0 && spanX >= 1 {
			stepX = clampAxis(remX, -spanX, spanX)
		}
		if remY != 0 && spanY >= 1 {
			stepY = clampAxis(remY, -spanY, spanY)
		}
		if stepX == 0 && stepY == 0 {
			break
		}
		startX := baseX
		if stepX > 0 {
			startX = min(baseX, maxX-stepX)
			if startX < minX {
				startX = minX
			}
		} else if stepX < 0 {
			startX = max(baseX, minX-stepX)
			if startX > maxX {
				startX = maxX
			}
		}
		startY := baseY
		if stepY > 0 {
			startY = min(baseY, maxY-stepY)
			if startY < minY {
				startY = minY
			}
		} else if stepY < 0 {
			startY = max(baseY, minY-stepY)
			if startY > maxY {
				startY = maxY
			}
		}
		from := driver.Point{X: startX, Y: startY}
		to := driver.Point{X: startX + stepX, Y: startY + stepY}
		segs = append(segs, []driver.Point{from, to})
		remX -= stepX
		remY -= stepY
	}
	return segs
}

// resolveWatchDragFallback decides whether a fallback (non-HID) drag should
// scroll an enclosing ScrollView when neither OnPan nor a touch responder took it.
func resolveWatchDragFallback(panActivated, touchBlocked bool, start, end driver.Point) (scrollDelta driver.Point, shouldScroll bool) {
	if panActivated || touchBlocked {
		return driver.Point{}, false
	}
	dx := end.X - start.X
	dy := end.Y - start.Y
	if math.Hypot(float64(dx), float64(dy)) < float64(driver.DefaultPanSlop) {
		return driver.Point{}, false
	}
	return driver.Point{X: -dx, Y: -dy}, true
}
