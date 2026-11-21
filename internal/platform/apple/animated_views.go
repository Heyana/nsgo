//go:build darwin

package apple

import "github.com/nsgo-dev/nsgo/internal/driver"

// ApplyAnimated delivers changed values to the views still on a surface.
// Styles and text update before canvases and native props, then sticky cells
// follow their animated insets.
func ApplyAnimated[V comparable](a *driver.Animator, viewOf func(driver.NodeID) V, values func(V) *driver.AnimatedValues, restyle, redrawText, redrawCanvas func(V), component func(V) driver.NativeComponent, repin func()) {
	var missing V
	a.Changed(func(id driver.NodeID, vals driver.AnimatedValues) {
		v := viewOf(id)
		if v == missing {
			return
		}
		previous := values(v)
		wasColored := previous.Has(driver.PropTextColor)
		isColored := vals.Has(driver.PropTextColor)
		recolor := wasColored != isColored || isColored && previous.TextColor != vals.TextColor
		*previous = vals
		restyle(v)
		if recolor {
			redrawText(v)
		}
	})
	a.ChangedCanvases(func(id driver.NodeID) {
		if v := viewOf(id); v != missing {
			redrawCanvas(v)
		}
	})
	a.ChangedNatives(func(id driver.NodeID, b driver.NativeBinding, num float64, col driver.Color) {
		if v := viewOf(id); v != missing {
			driver.SetAnimatedProp(component(v), b, num, col)
		}
	})
	repin()
}
