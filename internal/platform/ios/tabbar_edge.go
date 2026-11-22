package ios

import "github.com/nsgo-dev/nsgo/internal/driver"

// This file builds on every system, so tests check the tab bar's edge decoding,
// measurement, vertical layout geometry and UIVerticalBarEdge mapping on any host.

// tabBarEdge is the resolved edge of the iOS native tab bar component.
type tabBarEdge uint8

const (
	tabBarEdgeBottom tabBarEdge = iota
	tabBarEdgeLeading
	tabBarEdgeTrailing
)

// verticalTabBarWidth is the content width of a vertical iOS tab bar before its edge safe area inset.
const verticalTabBarWidth driver.Points = 72

// parseTabBarEdge decodes core.TabBar's "edge" prop.
func parseTabBarEdge(v any) tabBarEdge {
	switch s, _ := v.(string); s {
	case "leading":
		return tabBarEdgeLeading
	case "trailing":
		return tabBarEdgeTrailing
	default:
		return tabBarEdgeBottom
	}
}

// verticalTabBarOnLeft reports whether a vertical tab bar on edge sits on the physical left edge.
func verticalTabBarOnLeft(edge tabBarEdge, rtl bool) bool {
	if edge == tabBarEdgeLeading {
		return !rtl
	}
	return rtl
}

// verticalTabBarInset returns the horizontal safe area inset on the physical edge the vertical bar sits on.
func verticalTabBarInset(edge tabBarEdge, rtl bool, left, right float64) float64 {
	if edge != tabBarEdgeLeading && edge != tabBarEdgeTrailing {
		return 0
	}
	if verticalTabBarOnLeft(edge, rtl) {
		return left
	}
	return right
}

// tabBarNaturalSize returns the natural size core.TabBar asks the layout engine for:
// height alone for a bottom bar (width 0 stretches across), width alone for a vertical bar (height 0 stretches down).
func tabBarNaturalSize(edge tabBarEdge, fitHeight float64, rtl bool, left, right, bottom float64) driver.Size {
	if edge == tabBarEdgeLeading || edge == tabBarEdgeTrailing {
		return driver.Size{Width: verticalTabBarWidth + driver.Points(verticalTabBarInset(edge, rtl, left, right))}
	}
	if fitHeight <= 0 {
		fitHeight = 49
	}
	return driver.Size{Height: driver.Points(fitHeight + bottom)}
}

// verticalBarSeparatorX returns the physical x origin of the 1-point separator line on the content-facing side of a vertical bar.
func verticalBarSeparatorX(edge tabBarEdge, rtl bool, width float64) float64 {
	if verticalTabBarOnLeft(edge, rtl) {
		if width > 1 {
			return width - 1
		}
		return 0
	}
	return 0
}

// mapVerticalBarEdge maps UIKit's UIVerticalBarEdge raw value to driver.VerticalBarEdge.
func mapVerticalBarEdge(raw int) driver.VerticalBarEdge {
	const leading, trailing = 1, 2
	switch raw {
	case leading:
		return driver.VerticalBarLeading
	case trailing:
		return driver.VerticalBarTrailing
	default:
		return driver.VerticalBarUnspecified
	}
}
