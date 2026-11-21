//go:build ios

package ios

import (
	"math"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Touch-draggable scroll handles for overflowing iOS scroll views along either
// axis: a transparent hit-target subview (GrabRect) pinned to the viewport over
// the content and edge effects, holding a rounded capsule layer (Rect /
// ActiveRect) and a zero-delay UILongPressGestureRecognizer that scrubs the
// UIScrollView offset across the full content range.

var selHandleDrag = sel("nsgoScrollHandle:")

type scrollHandleView struct {
	holder   objc.ID
	thumb    objc.ID
	gesture  objc.ID
	dragging bool
	drag     driver.ScrollHandleDrag
	shown    bool
}

// hasPhoneHandle reports whether v shows the phone touch-draggable scroll
// handle rather than UIScrollView's thin non-draggable indicators.
func (v *view) hasPhoneHandle() bool {
	return v.sc != nil && v.surface != nil &&
		driver.HasTouchScrollHandle(v.surface.env) &&
		driver.ScrollHandleEnabled(&v.sc.config)
}

// scrollHandle returns the current geometry model for v's scroll handle.
func (v *view) scrollHandle() driver.ScrollHandle {
	at := objc.Send[apple.CGPoint](v.native, selContentOffset)
	seen := objc.Send[apple.CGRect](v.native, selBounds).Size
	content := objc.Send[apple.CGSize](v.native, sel("contentSize"))
	in := v.sc.config.Behavior.ContentInset
	in.Bottom += v.sc.keyboard
	return driver.ScrollHandle{
		Horizontal: v.sc.config.Horizontal,
		Frame:      driver.Size{Width: driver.Points(seen.Width), Height: driver.Points(seen.Height)},
		Content:    driver.Size{Width: driver.Points(content.Width), Height: driver.Points(content.Height)},
		Offset:     driver.Point{X: driver.Points(at.X), Y: driver.Points(at.Y)},
		Inset:      in,
	}
}

// updateHandle shows, positions, or hides v's phone scroll handle.
func (v *view) updateHandle() {
	if v.sc == nil {
		return
	}
	h := &v.sc.handle
	if !v.hasPhoneHandle() {
		h.dragging = false
		h.shown = false
		if h.holder != 0 {
			h.holder.Send(selSetHidden, true)
			h.holder.Send(sel("setUserInteractionEnabled:"), false)
		}
		return
	}
	sh := v.scrollHandle()
	var rect driver.Bounds
	var ok bool
	if h.dragging {
		rect, _, ok = sh.ActiveRect()
	} else {
		rect, _, ok = sh.Rect()
	}
	grab, grabOK := sh.GrabRect()
	if !ok || !grabOK {
		h.dragging = false
		h.shown = false
		if h.holder != 0 {
			h.holder.Send(selSetHidden, true)
			h.holder.Send(sel("setUserInteractionEnabled:"), false)
		}
		return
	}
	if h.holder == 0 {
		holder := objc.ID(apple.Class("UIView")).Send(selNew)
		holder.Send(sel("setBackgroundColor:"), objc.ID(apple.Class("UIColor")).Send(sel("clearColor")))
		holder.Send(sel("setUserInteractionEnabled:"), true)
		holder.Send(selLayer).Send(sel("setZPosition:"), 2000.0)
		thumb := objc.ID(apple.Class("CALayer")).Send(selNew)
		holder.Send(selLayer).Send(sel("addSublayer:"), thumb)
		thumb.Send(selRelease)
		g := objc.ID(apple.Class("UILongPressGestureRecognizer")).Send(selAlloc).
			Send(sel("initWithTarget:action:"), v.native, selHandleDrag)
		g.Send(sel("setMinimumPressDuration:"), 0.0)
		g.Send(sel("setAllowableMovement:"), 1e6)
		g.Send(sel("setCancelsTouchesInView:"), true)
		holder.Send(sel("addGestureRecognizer:"), g)
		g.Send(selRelease)
		v.native.Send(selAddSubview, holder)
		holder.Send(selRelease)
		h.holder, h.thumb, h.gesture = holder, thumb, g
	}
	at := objc.Send[apple.CGPoint](v.native, selContentOffset)
	alpha := float32(0.45)
	if h.dragging {
		alpha = 0.8
	}
	col := driver.RGBA(0x000000, alpha)
	if v.surface.env.Dark {
		col = driver.RGBA(0xffffff, alpha)
	}
	apple.WithoutAnimation(func() {
		h.holder.Send(selSetHidden, false)
		h.holder.Send(sel("setUserInteractionEnabled:"), true)
		h.holder.Send(selSetFrame, apple.Rect(
			at.X+float64(grab.Origin.X),
			at.Y+float64(grab.Origin.Y),
			float64(grab.Size.Width),
			float64(grab.Size.Height),
		))
		tw, th := float64(rect.Size.Width), float64(rect.Size.Height)
		h.thumb.Send(selSetFrame, apple.Rect(
			float64(rect.Origin.X-grab.Origin.X),
			float64(rect.Origin.Y-grab.Origin.Y),
			tw,
			th,
		))
		h.thumb.Send(sel("setCornerRadius:"), min(tw, th)/2)
		h.thumb.Send(sel("setBackgroundColor:"), uiColor(col).Send(sel("CGColor")))
	})
	h.shown = true
}

// releaseHandle removes v's phone scroll handle view.
func (v *view) releaseHandle() {
	if v.sc == nil {
		return
	}
	h := &v.sc.handle
	if h.holder != 0 {
		h.holder.Send(selRemoveFromSuperview)
	}
	*h = scrollHandleView{}
}

// handleBlocksPan reports whether a touch for UIScrollView's pan recognizer r
// belongs to v's phone scroll handle and should keep the scroll view from
// starting a content pan.
func (v *view) handleBlocksPan(r objc.ID) bool {
	if v.sc == nil {
		return false
	}
	h := &v.sc.handle
	if h.dragging {
		return true
	}
	if !h.shown {
		return false
	}
	loc := objc.Send[apple.CGPoint](r, selLocationInView, v.native)
	at := objc.Send[apple.CGPoint](v.native, selContentOffset)
	vp := driver.Point{X: driver.Points(loc.X - at.X), Y: driver.Points(loc.Y - at.Y)}
	return v.scrollHandle().Contains(vp)
}

func scrollHandleAction(self objc.ID, _ objc.SEL, g objc.ID) {
	v := scrollView(self)
	if v == nil || !v.hasPhoneHandle() {
		return
	}
	h := &v.sc.handle
	loc := objc.Send[apple.CGPoint](g, selLocationInView, v.native)
	at := objc.Send[apple.CGPoint](v.native, selContentOffset)
	vp := driver.Point{X: driver.Points(loc.X - at.X), Y: driver.Points(loc.Y - at.Y)}
	switch objc.Send[int](g, selState) {
	case stateBegan:
		drag, ok := v.scrollHandle().BeginDrag(vp)
		if !ok {
			return
		}
		v.surface.touchesTaken()
		if objc.Send[bool](self, sel("isDecelerating")) {
			self.Send(sel("setContentOffset:animated:"), at, false)
			v.scrollEvent(driver.EventMomentumEnd)
		}
		h.dragging = true
		h.drag = drag
		v.updateHandle()
		v.scrollEvent(driver.EventScrollDragStart)
	case stateChanged:
		if !h.dragging {
			return
		}
		to := h.drag.Offset(vp)
		self.Send(sel("setContentOffset:"), apple.CGPoint{X: float64(to.X), Y: float64(to.Y)})
		v.updateHandle()
	case stateEnded, stateCancelled:
		if !h.dragging {
			return
		}
		h.dragging = false
		v.updateHandle()
		v.scrollEvent(driver.EventScrollDragEnd)
		if !v.settleHandleSnap() {
			v.scrollEvent(driver.EventScrollEnd)
		}
	}
}

// settleHandleSnap animates a snapping scroll view to the nearest snap point
// after a handle drag ends and reports whether an animated scroll started.
func (v *view) settleHandleSnap() bool {
	c := &v.sc.config
	b := &c.Behavior
	if !b.Snaps() {
		return false
	}
	at := objc.Send[apple.CGPoint](v.native, selContentOffset)
	seen := objc.Send[apple.CGRect](v.native, selBounds).Size
	size := objc.Send[apple.CGSize](v.native, sel("contentSize"))
	pos, viewport, length := at.Y, seen.Height, size.Height+float64(b.ContentInset.Bottom)
	if c.Horizontal {
		pos, viewport, length = at.X, seen.Width, size.Width+float64(b.ContentInset.Right)
	}
	snap, ok := b.SnapTarget(driver.Points(pos), driver.Points(pos), 0, driver.Points(viewport), driver.Points(length))
	if !ok || math.Abs(float64(snap)-pos) < 0.5 {
		return false
	}
	to := at
	if c.Horizontal {
		to.X = float64(snap)
	} else {
		to.Y = float64(snap)
	}
	v.native.Send(sel("setContentOffset:animated:"), to, true)
	return true
}
