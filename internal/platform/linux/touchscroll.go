//go:build linux && !android

package linux

import (
	"math"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/scrollbar"
)

// A touchscreen's finger drags scroll views (scrollbar.TouchDrag): the
// innermost scroll view under it that moves along the drag takes it once
// it passes the slop, unless a pan took it first; the press it began
// presses nothing. A finger that stops a scroll view's glide or snap
// takes the touch at once so lifting presses nothing while moving still
// drags the view. Let go, the view glides, snaps or springs back on the
// frame clock, and a pull past the top refreshes, keeping room above the
// content for the spinner, or the app's own indicator, while it does.
// Values bound to the pull follow it.

const gdkSourceTouchscreen = 3

var (
	gdkDeviceGetSource func(uintptr) int32
	gActionActivate    func(action, parameter uintptr)
)

func bindTouchScroll() {
	purego.RegisterLibFunc(&gdkDeviceGetSource, gtkLib, "gdk_device_get_source")
	purego.RegisterLibFunc(&gActionActivate, gioLib, "g_action_activate")
}

// touchScroll is the touch that may drag a scroll view: the views under
// it that scroll, innermost first, the drag of the one that took it, and
// whether it braked a glide or snap so the touch is the scroll view's
// from the down.
type touchScroll = scrollbar.TouchSession[*view]

// isTouch reports whether device is a touchscreen.
func isTouch(device uintptr) bool {
	return device != 0 && gdkDeviceGetSource(device) == gdkSourceTouchscreen
}

// touchDown starts following a finger down at at for the scroll views
// under it; it stops their glides and snaps, and reports whether it
// braked one so the touch is the scroll view's and presses nothing.
func (w *window) touchDown(at driver.Point) bool {
	w.touching = nil
	root := w.view(w.root)
	if root == nil {
		return false
	}
	under := scrollsAt(root, at, nil)
	session := scrollbar.NewTouchSession(under, at, time.Now(), w.touchHost())
	if session == nil {
		return false
	}
	w.touching = session
	return session.Braked
}

// touchMove follows the finger to at; it reports whether a scroll view
// has it.
// nsgo:allow-duplicate The native window owns the session pointer; drag arbitration and offsets live in scrollbar.TouchSession.
func (w *window) touchMove(at driver.Point) bool {
	if w.touching == nil {
		return false
	}
	handled, keep := w.touching.Move(at, time.Now(), w.touchHost())
	if !keep {
		w.touching = nil
	}
	return handled
}

func (w *window) touchHost() scrollbar.TouchHost[*view] {
	return scrollbar.TouchHost[*view]{
		State: func(v *view) scrollbar.TouchView {
			return scrollbar.TouchView{Config: &v.scroll, Offset: v.offset, Frame: v.frame.Size,
				Content: v.size, Deleted: v.deleted, Moving: v.glide != nil || v.scrolling != nil, Refreshing: v.refreshing}
		},
		Interrupt: w.interruptScroll, Move: w.moveOffset,
		Blocked: w.panning != nil && w.panning.active || w.blockNative,
		Begin: func(v *view, braked bool) {
			if !braked {
				w.cancelTouches()
				w.cancelLongPress()
			}
			w.pressConsumed = true
			w.panning = nil
			w.scrollEvent(v, driver.EventScrollDragStart)
		},
	}
}

// touchUp lets the finger go: the view it dragged glides, snaps or
// springs back, refreshing when pulled far enough. It reports whether a
// view had the touch.
func (w *window) touchUp() bool {
	t := w.touching
	w.touching = nil
	if t == nil {
		return false
	}
	if t.Taken == nil || t.Taken.deleted {
		return t.Braked
	}
	v, c := t.Taken, &t.Taken.scroll
	if t.Drag.Refreshes(c, v.refreshing) {
		v.refreshing = true
		w.scrollEvent(v, driver.EventRefresh)
	}
	w.scrollEvent(v, driver.EventScrollDragEnd)
	lo, hi := scrollbar.Limits(c, v.frame.Size, v.size, v.refreshing)
	velocity := t.Drag.Velocity(c)
	if w.motionReduced() {
		velocity = 0
	}
	m := scrollbar.Release(c, v.frame.Size, v.size, v.offset, velocity, lo, hi, time.Now())
	if m.Moves() {
		v.glide = &m
		w.scrollEvent(v, driver.EventMomentumStart)
		w.animate(v)
	} else {
		w.restScroll(v)
	}
	return true
}

// stepGlide moves v along its glide; it reports whether it goes on.
func (w *window) stepGlide(v *view, now time.Time) bool {
	m := v.glide
	if m == nil {
		return v.refreshing && v.scroll.Behavior.RefreshIndicator == 0 && scrollbar.Pulled(&v.scroll, v.offset) > 0
	}
	at, done := m.At(now)
	offset := v.offset
	scrollbar.SetAlong(&v.scroll, &offset, at)
	w.moveOffset(v, offset)
	if done {
		if v.glide == w.wheelMotion {
			w.wheelMotion = nil
		}
		v.glide = nil
		w.scrollEvent(v, driver.EventMomentumEnd)
		w.restScroll(v)
	}
	return !done || v.refreshing
}

// refreshChanged follows the app's Refreshing: room shows above the top
// while it refreshes and goes as it ends.
func (w *window) refreshChanged(v *view) {
	c := &v.scroll
	on := c.Refreshing && !c.Horizontal
	if on == v.refreshing {
		return
	}
	v.refreshing = on
	top := -c.Behavior.ContentInset.Top
	switch {
	case on && v.offset.Y <= top+0.5 && w.touching == nil:
		w.glideTo(v, top-c.Behavior.RefreshInset())
	case !on && v.offset.Y < top && (w.touching == nil || w.touching.Taken != v):
		w.glideTo(v, top)
	}
	gtkWidgetQueueDraw(v.widget)
}

// glideTo eases v to along its axis.
func (w *window) glideTo(v *view, to driver.Points) {
	lo, hi := to, to
	m := scrollbar.Release(&v.scroll, v.frame.Size, v.size, v.offset, 0, lo, hi, time.Now())
	if !m.Moves() {
		return
	}
	v.glide = &m
	w.animate(v)
}

// snapshotSpinner draws the platform's refresh spinner in the room
// pulled above v's content: an arc that grows with the pull and turns
// while v refreshes.
func snapshotSpinner(v *view, snap uintptr, now time.Time) {
	c := &v.scroll
	if !c.Refreshable || c.Horizontal || c.Behavior.RefreshIndicator > 0 {
		return
	}
	pulled := scrollbar.Pulled(c, v.offset)
	if pulled < 4 {
		return
	}
	const radius, thick = 10, 2.5
	cx, cy := float64(v.frame.Size.Width)/2, float64(c.Behavior.ContentInset.Top)+float64(pulled)/2
	area := grapheneRect{float32(cx - radius - thick), float32(cy - radius - thick), 2 * (radius + thick), 2 * (radius + thick)}
	cr := gtkSnapshotAppendCairo(snap, &area)
	sweep := min(float64(pulled)/float64(c.Behavior.RefreshThreshold()), 1) * 1.6 * math.Pi
	turn := -math.Pi / 2
	if v.refreshing {
		sweep = 1.6 * math.Pi
		turn += math.Mod(float64(now.UnixMilli())/1000*2*math.Pi, 2*math.Pi)
	}
	cairoSetSourceRGBA(cr, 0.45, 0.45, 0.45, 1)
	cairoSetLineWidth(cr, thick)
	cairoNewPath(cr)
	cairoArc(cr, cx, cy, radius, turn, turn+sweep)
	cairoStroke(cr)
	cairoDestroy(cr)
}
