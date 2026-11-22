package ios

import (
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// This file builds on every system, so tests check tab bar sizing and
// deduplication decisions on any host.

const (
	defaultTabBarWidth  = 390.0
	defaultTabBarHeight = 49.0
	tabBarHeightEpsilon = 0.01
)

// tabBarSizing tracks the natural height last reported for a UITabBar and
// decides when and at what width to measure it. Before attachment it measures
// once at defaultTabBarWidth without guessing a window's bottom safe area;
// once attached it re-measures whenever UIKit attaches, lays out, or updates
// the bar's safe area or traits, skipping collapsed frames (such as
// DisplayNone or the pre-layout frame before the initial size takes effect)
// and deduplicating unchanged heights.
type tabBarSizing struct {
	reported    driver.Points
	hasReported bool
}

// measureWidth returns the width to pass to UITabBar's sizeThatFits: and
// whether a measurement should run now.
func (s *tabBarSizing) measureWidth(attached bool, boundsWidth, boundsHeight float64) (float64, bool) {
	if !attached {
		if s.hasReported {
			return 0, false
		}
		return defaultTabBarWidth, true
	}
	if boundsWidth <= 0 || boundsHeight <= 0 {
		return 0, false
	}
	return boundsWidth, true
}

// update normalizes UIKit's fitted height and reports whether it differs
// meaningfully from the last reported natural height.
func (s *tabBarSizing) update(fitHeight float64) (driver.Points, bool) {
	if !(fitHeight > 0) || math.IsInf(fitHeight, 0) {
		fitHeight = defaultTabBarHeight
	}
	h := driver.Points(fitHeight)
	if s.hasReported && math.Abs(float64(h-s.reported)) < tabBarHeightEpsilon {
		return s.reported, false
	}
	s.reported = h
	s.hasReported = true
	return h, true
}
