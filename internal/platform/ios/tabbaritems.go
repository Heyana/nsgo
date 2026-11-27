package ios

import (
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// tabBarIconPoints is the logical square size of a tab bar SVG icon, matching core.TabBar.
const tabBarIconPoints = 24

type tabBarItemSpec struct {
	title   string
	icon    string
	drawing *driver.Drawing
}

func parseTabBarItems(list []any) []tabBarItemSpec {
	items := make([]tabBarItemSpec, len(list))
	for i, it := range list {
		tab, _ := it.(map[string]any)
		title, _ := tab["title"].(string)
		icon, _ := tab["icon"].(string)
		drawing, _ := tab["drawing"].(*driver.Drawing)
		items[i] = tabBarItemSpec{title: title, icon: icon, drawing: drawing}
	}
	return items
}

// tabBarIconsEqual reports whether two item lists carry the same icon sources
// (SF Symbol names and SVG drawing operations), so title-only updates can
// retitle existing UITabBarItems in place without rebuilding them.
func tabBarIconsEqual(a, b []tabBarItemSpec) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].icon != b[i].icon || !drawingsEqual(a[i].drawing, b[i].drawing) {
			return false
		}
	}
	return true
}

func tabBarHasDrawings(items []tabBarItemSpec) bool {
	for _, it := range items {
		if it.drawing != nil && len(it.drawing.Ops) > 0 {
			return true
		}
	}
	return false
}

// resolveTabBarImage resolves a tab's image with SF Symbol precedence:
// systemImage is tried when icon is non-empty, and svgImage is invoked only
// when systemImage returns 0 and drawing has operations.
func resolveTabBarImage(spec tabBarItemSpec, systemImage func(string) uintptr, svgImage func(*driver.Drawing) uintptr) uintptr {
	if spec.icon != "" {
		if img := systemImage(spec.icon); img != 0 {
			return img
		}
	}
	if spec.drawing != nil && len(spec.drawing.Ops) > 0 {
		return svgImage(spec.drawing)
	}
	return 0
}

func drawingsEqual(a, b *driver.Drawing) bool {
	if a == b {
		return true
	}
	var opsA, opsB []driver.DrawOp
	if a != nil {
		opsA = a.Ops
	}
	if b != nil {
		opsB = b.Ops
	}
	if len(opsA) != len(opsB) {
		return false
	}
	for i := range opsA {
		if !drawOpsEqual(opsA[i], opsB[i]) {
			return false
		}
	}
	return true
}

func drawOpsEqual(a, b driver.DrawOp) bool {
	ga, gb := a.Gradient, b.Gradient
	da, db := a.Dash, b.Dash
	a.Gradient, b.Gradient = nil, nil
	a.Dash, b.Dash = nil, nil
	if !a.Equal(b) {
		return false
	}
	return gradientsEqual(ga, gb) && dashesEqual(da, db)
}

func gradientsEqual(a, b *driver.CanvasGradient) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Radial == b.Radial &&
		a.Conic == b.Conic &&
		a.X1 == b.X1 && a.Y1 == b.Y1 && a.R1 == b.R1 &&
		a.X2 == b.X2 && a.Y2 == b.Y2 && a.R2 == b.R2 &&
		a.Angle == b.Angle &&
		a.Transform == b.Transform &&
		a.Spread == b.Spread &&
		slices.Equal(a.Stops, b.Stops)
}

func dashesEqual(a, b *driver.LineDash) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return slices.Equal(*a, *b)
}
