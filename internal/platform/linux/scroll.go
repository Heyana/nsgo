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
// (touchscroll.go). The keyboard props and zooming do nothing here.

// scrollEvent reports kind on scroll view v at its offset, if it listens.
func (w *window) scrollEvent(v *view, kind driver.EventKind) {
	if v.events.Has(kind) {
		w.engine.Dispatch(driver.Event{Kind: kind, ID: v.id, Point: v.offset, Count: v.applied})
	}
}

// interruptScroll stops v's snap or the glide a finger let go of, which ends its momentum: the app's ScrollTo, the wheel or the bar takes over, as a finger on the view does (touchDown).
func (w *window) interruptScroll(v *view) {
	if sc := v.scrolling; sc != nil && sc.Momentum {
		w.scrollEvent(v, driver.EventMomentumEnd)
	}
	v.scrolling = nil
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
	if v == nil || v.deleted || v.scroll.Behavior.Snaps() || w.env.ReduceMotion {
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
	w.scrollEvent(v, driver.EventScrollEnd)
	w.animator.Settle(v.id)
	if w.animator.Running() {
		w.startClock()
	}
}

// settle moves snapping view v to where it rests after the user let it
// go at velocity (points per millisecond along its axis).
func (w *window) settle(v *view, velocity float32) {
	to, ok := scrollbar.Settle(&v.scroll, v.frame.Size, v.size, v.offset, velocity)
	if !ok || to == v.offset {
		w.restScroll(v)
		return
	}
	w.scrollEvent(v, driver.EventMomentumStart)
	if w.env.ReduceMotion {
		w.scrollTo(v, to)
		w.scrollEvent(v, driver.EventMomentumEnd)
		w.restScroll(v)
		return
	}
	v.scrolling = &scrollbar.OffsetAnimation{From: v.offset, To: to, Start: time.Now(), Momentum: true}
	w.animate(v)
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
