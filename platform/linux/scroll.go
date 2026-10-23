//go:build linux && !android

package linux

import (
	"time"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/platform/internal/scrollbar"
)

// Scroll behavior: content insets widen the offsets, NoScroll keeps the
// wheel and the bar out, and snapping views settle on a snap point once
// the wheel rests or the bar is let go, as NSH's desktops do. Views
// scroll with the wheel, the touchpad and the bar only, so bounces,
// deceleration, pull to refresh, the keyboard props and zooming do
// nothing here.

// scrollEvent reports kind on scroll view v at its offset, if it listens.
func (w *window) scrollEvent(v *view, kind core.EventKind) {
	if v.events.Has(kind) {
		w.engine.Dispatch(core.Event{Kind: kind, ID: v.id, Point: v.offset, Count: v.applied})
	}
}

// interruptScroll stops v's snap, which ends its momentum.
func (w *window) interruptScroll(v *view) {
	if sc := v.scrolling; sc != nil && sc.momentum {
		w.scrollEvent(v, core.EventMomentumEnd)
	}
	v.scrolling = nil
}

// wheelTurned follows a wheel turn that moved snapping view v by d; the
// frame clock settles it once the wheel rests.
func (w *window) wheelTurned(v *view, d core.Points) {
	if v.scroll.Behavior.Snaps() {
		v.wheel.Turn(time.Now(), d)
		w.animate(v)
	}
}

// settle moves snapping view v to where it rests after the user let it
// go at velocity (points per millisecond along its axis).
func (w *window) settle(v *view, velocity float32) {
	to, ok := scrollbar.Settle(&v.scroll, v.frame.Size, v.size, v.offset, velocity)
	if !ok || to == v.offset {
		return
	}
	w.scrollEvent(v, core.EventMomentumStart)
	if w.env.ReduceMotion {
		w.scrollTo(v, to)
		w.scrollEvent(v, core.EventMomentumEnd)
		return
	}
	v.scrolling = &scrolling{from: v.offset, to: to, start: time.Now(), momentum: true}
	w.animate(v)
}

// releaseBar ends dragging the scroll bar: a snapping view settles.
func (w *window) releaseBar() {
	d := w.barDrag
	w.barDrag = nil
	if d == nil || d.v.deleted {
		return
	}
	w.scrollEvent(d.v, core.EventScrollDragEnd)
	w.settle(d.v, 0)
}

// scrollBy moves v's offset by d unclamped, carrying a snap or a bar
// drag in flight along: the core keeps the visible content in place as
// rows above it change size.
func (w *window) scrollBy(v *view, d core.Point) {
	if sc := v.scrolling; sc != nil {
		sc.from.X, sc.from.Y = sc.from.X+d.X, sc.from.Y+d.Y
		sc.to.X, sc.to.Y = sc.to.X+d.X, sc.to.Y+d.Y
	}
	if b := w.barDrag; b != nil && b.v == v {
		if v.scroll.Horizontal {
			b.drag.Shift(d.X)
		} else {
			b.drag.Shift(d.Y)
		}
	}
	w.moveOffset(v, core.Point{X: v.offset.X + d.X, Y: v.offset.Y + d.Y})
}

// setSticky replaces scroll view v's sticky cells; cells no longer
// sticky go back to their frames.
func (w *window) setSticky(v *view, cells []core.StickyCell) {
	for _, c := range v.sticky {
		if cv := w.view(c.Cell); cv != nil {
			w.stick(cv, core.Point{})
		}
	}
	v.sticky = cells
	w.pinSticky(v)
}

// pinSticky moves scroll view v's sticky cells for its offset, as NSH's
// StickyCells: each by clamp(offset - start, 0, end - start) from its
// frame.
func (w *window) pinSticky(v *view) {
	at := v.offset.Y
	if v.scroll.Horizontal {
		at = v.offset.X
	}
	for _, c := range v.sticky {
		if cv := w.view(c.Cell); cv != nil {
			d := c.StickyShift(at)
			shift := core.Point{Y: d}
			if v.scroll.Horizontal {
				shift = core.Point{X: d}
			}
			w.stick(cv, shift)
		}
	}
}

// stick moves cell v by shift from the frame the core gave it.
func (w *window) stick(v *view, shift core.Point) {
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
