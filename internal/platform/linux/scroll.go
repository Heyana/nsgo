//go:build linux && !android

package linux

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/scrollbar"
)

// Scroll behavior: content insets widen the offsets, NoScroll keeps the
// wheel and the bar out, and snapping views settle on a snap point once
// the wheel rests or the bar is let go, as NSH's desktops do. A touchpad
// glides on as its fingers lift (wheelGlide), as a finger's drag does
// (touchscroll.go). Zooming scroll views (MaxZoomScale > MinZoomScale) scale their content around the origin, respond to ItemZoomTo, Ctrl+wheel and two-finger pinches, and pan along both axes.

// zoomAnim is an animated scroll zoom in flight.
type zoomAnim struct {
	ease  scrollbar.ZoomEase
	start time.Time
}

// zoomScale returns v's active scroll zoom scale, 1 when v does not zoom.
func (v *view) zoomScale() float64 {
	if !v.scroll.Zooms() || v.zoom <= 0 {
		return 1
	}
	return v.zoom
}

// zoomValue returns the zoom reported on scroll events: 0 when v does not zoom.
func (v *view) zoomValue() float32 {
	if v.scroll.Zooms() {
		return float32(v.zoomScale())
	}
	return 0
}

// clampOffset clamps offset to v's content limits, accounting for scroll zoom and any active refresh room.
func (v *view) clampOffset(offset driver.Point, refreshing bool) driver.Point {
	if v.scroll.Zooms() {
		return scrollbar.ClampZoomOffset(&v.scroll, v.frame.Size, v.size, offset, v.zoomScale())
	}
	return scrollbar.ClampRefreshing(&v.scroll, v.frame.Size, v.size, offset, refreshing)
}

// contentPoint maps a point p in v's viewport into v's unzoomed content space.
func (v *view) contentPoint(p driver.Point) driver.Point {
	p.X += v.offset.X
	p.Y += v.offset.Y
	if z := v.zoomScale(); z != 1 {
		zp := driver.Points(z)
		p.X /= zp
		p.Y /= zp
	}
	return p
}

// configureScrollZoom updates v's zoom state after v.scroll changed from old.
func (w *window) configureScrollZoom(v *view, old *driver.ScrollConfig) {
	c := &v.scroll
	if !c.Zooms() {
		v.zooming = nil
		if v.zoom != 0 && v.zoom != 1 {
			v.zoom = 1
			w.moved(v)
		}
		return
	}
	if !old.Zooms() {
		v.zoom = float64(c.ClampZoom(c.ZoomScale))
		w.moved(v)
		return
	}
	if c.MinZoomScale != old.MinZoomScale || c.MaxZoomScale != old.MaxZoomScale {
		v.zooming = nil
		z := scrollbar.LimitZoom(c, v.zoomScale(), false)
		if z != v.zoom {
			v.zoom = z
			w.moved(v)
		}
	}
	if c.ZoomScale != old.ZoomScale {
		w.zoomTo(v, &driver.ZoomRequest{Scale: c.ZoomScale}, true)
	}
}

// zoomTo zooms scroll view v to r.Scale, keeping content point r.Center (or the current viewport center) in the middle of the viewport.
func (w *window) zoomTo(v *view, r *driver.ZoomRequest, animated bool) {
	if r == nil || !v.scroll.Zooms() {
		return
	}
	w.stopWheelGlide()
	w.interruptScroll(v)
	target := scrollbar.LimitZoom(&v.scroll, float64(r.Scale), false)
	focus := driver.Point{X: v.frame.Size.Width / 2, Y: v.frame.Size.Height / 2}
	z := v.zoomScale()
	center := scrollbar.ContentAt(v.offset, focus, z)
	if r.Center != nil {
		center = *r.Center
	}
	if !animated || w.env.ReduceMotion {
		at := scrollbar.ClampZoomOffset(&v.scroll, v.frame.Size, v.size, scrollbar.OffsetFor(center, focus, target), target)
		v.zoom = target
		w.moveOffset(v, at)
		w.restScroll(v)
		return
	}
	ease, at, ok := scrollbar.NewZoomEase(&v.scroll, v.frame.Size, v.size, v.offset, z, center, focus, target)
	if !ok {
		v.zoom = target
		w.moveOffset(v, at)
		w.restScroll(v)
		return
	}
	v.zooming = &zoomAnim{ease: ease, start: time.Now()}
	w.animate(v)
}

// wheelZoom handles Ctrl+wheel over a zooming scroll view, zooming around p.
func (w *window) wheelZoom(p driver.Point, delta float64) bool {
	root := w.view(w.root)
	if root == nil {
		return false
	}
	under := scrollsAt(root, p, nil)
	for i := len(under) - 1; i >= 0; i-- {
		v := under[i]
		if !v.scroll.Zooms() || v.scroll.NoPinchZoom {
			continue
		}
		w.stopWheelGlide()
		w.interruptScroll(v)
		focus := v.surfacePoint(p)
		z := v.zoomScale()
		center := scrollbar.ContentAt(v.offset, focus, z)
		next := scrollbar.LimitZoom(&v.scroll, scrollbar.WheelZoom(z, delta*48), false)
		at := scrollbar.ClampZoomOffset(&v.scroll, v.frame.Size, v.size, scrollbar.OffsetFor(center, focus, next), next)
		if next != z || at != v.offset {
			v.zoom = next
			w.moveOffset(v, at)
		}
		return true
	}
	return false
}

// scrollEvent reports kind on scroll view v at its offset, if it listens.
// nsgo:allow-duplicate the Linux and Windows views are distinct types; the body only forwards to the engine and the geometry observer.
func (w *window) scrollEvent(v *view, kind driver.EventKind) {
	defer w.reportGeometry()
	if v.events.Has(kind) {
		w.engine.Dispatch(driver.Event{Kind: kind, ID: v.id, Point: v.offset, Value: v.zoomValue(), Count: v.applied})
	}
}

// interruptScroll stops v's snap, zoom animation or the glide a finger let go of, which ends its momentum: the app's ScrollTo, the wheel or the bar takes over, as a finger on the view does (touchDown).
func (w *window) interruptScroll(v *view) {
	if sc := v.scrolling; sc != nil && sc.Momentum {
		w.scrollEvent(v, driver.EventMomentumEnd)
	}
	v.scrolling = nil
	v.zooming = nil
	if v.glide != nil {
		if v.glide == w.wheelMotion {
			w.wheelMotion = nil
		}
		v.glide = nil
		w.scrollEvent(v, driver.EventMomentumEnd)
	}
}

// wheelTurned follows a wheel turn that moved snapping view v by d; the
// frame clock settles it once the wheel rests.
func (w *window) wheelTurned(v *view, d driver.Points) {
	v.wheel.Turn(time.Now(), d)
	w.animate(v)
}

// wheelGlide lets the view the touchpad moved glide on at (vx, vy) points
// per millisecond as the fingers lift, stopping at the edge it reaches:
// touchpads do not bounce here. Snapping views settle once the wheel rests
// instead.
func (w *window) wheelGlide(vx, vy float32) {
	v := w.wheeled
	if v == nil || v.deleted || v.scroll.Behavior.Snaps() || w.motionReduced() {
		return
	}
	velocity := vy
	if v.scroll.Horizontal {
		velocity = vx
	}
	c := v.scroll
	c.Behavior.NoBounces = true
	lo, hi := scrollbar.Limits(&c, v.frame.Size, v.size, v.refreshing)
	m := scrollbar.Release(&c, v.frame.Size, v.size, v.offset, velocity, lo, hi, time.Now())
	if !m.Moves() {
		return
	}
	v.wheel = scrollbar.Wheel{}
	v.glide, w.wheelMotion = &m, &m
	w.scrollEvent(v, driver.EventMomentumStart)
	w.animate(v)
}

// stopWheelGlide stops the touchpad's glide where it is; the view rests
// as it does when the wheel stops.
func (w *window) stopWheelGlide() {
	v := w.wheeled
	if v == nil || v.deleted || v.glide == nil || v.glide != w.wheelMotion {
		return
	}
	v.glide, w.wheelMotion = nil, nil
	w.scrollEvent(v, driver.EventMomentumEnd)
	w.wheelTurned(v, 0)
}

// restScroll follows v coming to rest, after a drag that set nothing in
// motion, a glide, a snap, an animated scroll or the wheel: it reports
// EventScrollEnd, and the snapping diff clamps that follow v settle.
func (w *window) restScroll(v *view) {
	scrollbar.Rest(w.animator, v.id, func() { w.scrollEvent(v, driver.EventScrollEnd) }, w.startClock)
}

// settle moves snapping view v to where it rests after the user let it
// go at velocity (points per millisecond along its axis).
func (w *window) settle(v *view, velocity float32) {
	host := scrollbar.SettleHost{
		Scroll: func(to driver.Point) { w.scrollTo(v, to) },
		Animate: func(to driver.Point) {
			v.scrolling = &scrollbar.OffsetAnimation{From: v.offset, To: to, Start: time.Now(), Momentum: true}
			w.animate(v)
		},
		Event: func(kind driver.EventKind) { w.scrollEvent(v, kind) },
		Rest:  func() { w.restScroll(v) },
	}
	host.Settle(&v.scroll, v.frame.Size, v.size, v.offset, velocity, w.motionReduced())
}

// releaseBar ends dragging the scroll bar: a snapping view settles.
func (w *window) releaseBar() {
	d := w.barDrag
	w.barDrag = nil
	if d == nil || d.v.deleted {
		return
	}
	w.scrollEvent(d.v, driver.EventScrollDragEnd)
	w.settle(d.v, 0)
}

// scrollBy moves v's offset by d unclamped, carrying a snap or a bar
// drag in flight along: the core keeps the visible content in place as
// rows above it change size.
func (w *window) scrollBy(v *view, d driver.Point) {
	var drag *scrollbar.Drag
	if b := w.barDrag; b != nil && b.v == v {
		drag = &b.drag
	}
	w.moveOffset(v, scrollbar.RebaseOffset(&v.scroll, v.offset, d, v.scrolling, drag))
}

// setSticky replaces scroll view v's sticky cells; cells no longer
// sticky go back to their frames.
func (w *window) setSticky(v *view, cells []driver.StickyCell) {
	w.stickyBound.Replace(v, v.id, &v.sticky, cells, w.placeSticky, w.pinSticky)
}

func (w *window) pinSticky(v *view) {
	v.sticky.Pin(v.offset, v.scroll.Horizontal, w.animator, w.placeSticky)
}

func (w *window) placeSticky(id driver.NodeID, shift driver.Point) {
	if v := w.view(id); v != nil {
		w.stick(v, shift)
	}
}

// stick moves cell v by shift from the frame the core gave it.
func (w *window) stick(v *view, shift driver.Point) {
	if shift == v.shift {
		return
	}
	v.shift = shift
	f := v.laid
	f.Origin.X += shift.X
	f.Origin.Y += shift.Y
	v.frame = f
	w.moved(v)
}

func (w *window) repinBound() {
	w.stickyBound.Repin(w.view, w.pinSticky)
}
