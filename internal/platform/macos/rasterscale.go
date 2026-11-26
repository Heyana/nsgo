//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Core Animation scales a layer's pixels with its transform, so text and
// drawings under a view scaled up, such as a canvas's zoomed plane, would
// blur. A view whose scale passes a step draws the content layers inside
// it again at that step (apple.RasterStep), which a zoom crosses a few
// times rather than every frame.

// contentScale is the pixels per point v's content layer draws at: the
// window's, times the raster steps of v and the views around it, times a
// canvas's ResolutionScale.
func (v *view) contentScale() float64 {
	scale := float64(v.surface.env.PixelRatio)
	for n := v; n != nil; n = n.parentView() {
		if n.rasterStep > 1 {
			scale *= n.rasterStep
		}
	}
	return scale * v.drawing.ResolutionScale()
}

// parentView is the view whose native view holds v's, or nil.
func (v *view) parentView() *view {
	for n := v.native.Send(selSuperview); n != 0 && n != v.surface.native; n = n.Send(selSuperview) {
		if p := views[n]; p != nil && p.native == n {
			return p
		}
	}
	return nil
}

// followScale draws the content inside v again when vs, v's style with
// its animated values, scales it past another step.
func (v *view) followScale(vs *driver.ViewStyle) {
	step := apple.RasterStep(vs.Transform.RasterScale())
	if step == v.rasterStep || (step <= 1 && v.rasterStep <= 1) {
		return
	}
	v.rasterStep = step
	// The views inside v, from its native view down: going up from each
	// of the window's views took a few percent of a core on the gallery's
	// Animations page, whose rings pass a step every loop.
	var walk func(n objc.ID)
	walk = func(n objc.ID) {
		if w := views[n]; w != nil && w.native == n && w.contentLayer != 0 {
			w.contentLayer.Send(selSetContentsScale, w.contentScale())
			w.redraw()
		}
		for _, c := range apple.Subviews(n) {
			walk(c)
		}
	}
	walk(v.native)
}

// restretch gives canvas v's content layer the pixels per point and the
// stretch of its drawing, which replaced was, when they changed.
func (v *view) restretch(was *driver.Drawing) {
	if v.contentLayer == 0 {
		return
	}
	if was == nil || was.ResolutionScale() != v.drawing.ResolutionScale() {
		v.contentLayer.Send(selSetContentsScale, v.contentScale())
	}
	if was == nil || was.Smooth() != v.drawing.Smooth() {
		apple.StretchCanvas(v.contentLayer, v.drawing)
	}
}
