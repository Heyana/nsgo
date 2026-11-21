package nsgotest

import (
	"math"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func (a *App) currentEnv() driver.Environment {
	if a.env != nil {
		return *a.env
	}
	return driver.Environment{}
}

func (v *View) scrollHandle() driver.ScrollHandle {
	return driver.ScrollHandle{
		Horizontal: v.Scroll.Horizontal,
		Frame:      v.Frame.Size,
		Content:    v.Content,
		Offset:     v.Offset,
		Inset:      v.Scroll.Behavior.ContentInset,
	}
}

func (a *App) scrollHandleOf(v *View) (driver.ScrollHandle, bool) {
	if v == nil || v.Kind != KindScroll ||
		!driver.HasTouchScrollHandle(a.currentEnv()) ||
		!driver.ScrollHandleEnabled(&v.Scroll) {
		return driver.ScrollHandle{}, false
	}
	h := v.scrollHandle()
	if _, _, ok := h.Rect(); !ok {
		return driver.ScrollHandle{}, false
	}
	return h, true
}

// ScrollHandle returns the phone scroll handle's resting rectangle in the
// local viewport coordinates of the scroll view selector matches, and whether
// the scroll view currently shows a touch-draggable scroll handle (iOS or
// Android touch idiom, indicator shown, scrolling enabled, non-zooming, and
// content overflowing along its active axis).
func (a *App) ScrollHandle(selector string) (core.Bounds, bool) {
	a.t.Helper()
	v := a.Find(selector)
	h, ok := a.scrollHandleOf(v)
	if !ok {
		return core.Bounds{}, false
	}
	rect, _, ok := h.Rect()
	return rect, ok
}

// tryGrabScrollHandle checks whether a finger landing at surface point start
// hits an active phone scroll handle on any scroll view in f.chain (innermost
// first). If so, it claims the finger immediately as dragHandle so the scroll
// view's content neither presses nor pans.
func (a *App) tryGrabScrollHandle(f *finger, start driver.Point) bool {
	if !driver.HasTouchScrollHandle(a.currentEnv()) {
		return false
	}
	for _, sv := range f.chain {
		h, ok := a.scrollHandleOf(sv)
		if !ok {
			continue
		}
		local := sv.localPoint(start)
		if !h.Contains(local) {
			continue
		}
		drag, ok := h.BeginDrag(local)
		if !ok {
			continue
		}
		f.mode = dragHandle
		f.view = sv
		f.handleDrag = drag
		f.press = nil
		f.reporting = false
		a.scrollEvent(sv, driver.EventScrollDragStart)
		return true
	}
	return false
}

// settleHandle snaps a snapping scroll view to its nearest snap target after a
// phone scroll handle drag ends, matching iOS and Android, before settling.
func (a *App) settleHandle(v *View) {
	c := &v.Scroll
	b := &c.Behavior
	if b.Snaps() {
		pos, viewport, length := v.Offset.Y, v.Frame.Size.Height, v.Content.Height+b.ContentInset.Bottom
		if c.Horizontal {
			pos, viewport, length = v.Offset.X, v.Frame.Size.Width, v.Content.Width+b.ContentInset.Right
		}
		if snap, ok := b.SnapTarget(pos, pos, 0, viewport, length); ok && math.Abs(float64(snap-pos)) >= 0.5 {
			to := v.Offset
			if c.Horizontal {
				to.X = snap
			} else {
				to.Y = snap
			}
			to = v.clamp(to)
			a.scrollEvent(v, driver.EventMomentumStart)
			a.scrollView(v, to)
			a.scrollEvent(v, driver.EventMomentumEnd)
		}
	}
	a.settle(v)
}
