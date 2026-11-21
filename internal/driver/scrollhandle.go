package driver

const (
	// ScrollHandleInset is the distance from the scroll view's trailing edge
	// (right for vertical, bottom for horizontal) to the visual handle.
	ScrollHandleInset Points = 3
	// ScrollHandleMargin keeps the handle clear of both ends of the scroll
	// axis.
	ScrollHandleMargin Points = 4
	// ScrollHandleThickness is the visual thickness of the handle at rest,
	// and ScrollHandleActiveThickness its thickness while dragged.
	ScrollHandleThickness       Points = 6
	ScrollHandleActiveThickness Points = 10
	// ScrollHandleMinLength is the shortest visual handle along the scroll
	// axis (clamped to half the track on very small scroll views so travel
	// stays positive).
	ScrollHandleMinLength Points = 44
	// ScrollHandleGrabThickness is the width of the touch-grab strip along
	// the trailing edge.
	ScrollHandleGrabThickness Points = 32
	// ScrollHandleGrabSlop extends the touch-grab target past both ends of
	// the visual handle along the scroll axis.
	ScrollHandleGrabSlop Points = 8
)

// HasTouchScrollHandle reports whether env is an iOS or Android touch device
// (phone or tablet, excluding tvOS, watchOS, and web simulations) that shows
// native touch-draggable scroll handles.
func HasTouchScrollHandle(env Environment) bool {
	return !env.Simulated && (env.OS == OSIOS || env.OS == OSAndroid) &&
		(env.Idiom == IdiomPhone || env.Idiom == IdiomTablet)
}

// ScrollHandleEnabled reports whether a scroll view configured with c may show
// a touch-draggable scroll handle: its indicator is shown, scrolling is
// enabled, and it is a single-axis (non-zooming) scroll view.
func ScrollHandleEnabled(c *ScrollConfig) bool {
	return c != nil && !c.HideIndicator && !c.Behavior.NoScroll && !c.Zooms()
}

// ScrollHandle computes the geometry and touch-scrub mapping of a phone scroll
// view's draggable handle along its active axis.
type ScrollHandle struct {
	Horizontal bool
	Frame      Size
	Content    Size
	Offset     Point
	Inset      Edges
}

// along returns the viewport length, total scrollable content length (insets
// included), offset from the leading inset, and the leading inset along the
// active axis.
func (h ScrollHandle) along() (viewport, total, offset, lead Points) {
	if h.Horizontal {
		lead = h.Inset.Left
		return h.Frame.Width, h.Content.Width + h.Inset.Left + h.Inset.Right, h.Offset.X + lead, lead
	}
	lead = h.Inset.Top
	return h.Frame.Height, h.Content.Height + h.Inset.Top + h.Inset.Bottom, h.Offset.Y + lead, lead
}

func (h ScrollHandle) rectWith(thickness Points) (rect Bounds, travel Points, ok bool) {
	viewport, total, offset, _ := h.along()
	if total <= viewport+0.5 || viewport <= 2*ScrollHandleMargin+1 {
		return Bounds{}, 0, false
	}
	track := viewport - 2*ScrollHandleMargin
	span := total - viewport
	var over Points
	if offset < 0 {
		over = -offset
	} else if offset > span {
		over = offset - span
	}
	minLen := min(ScrollHandleMinLength, track/2)
	length := min(max(track*viewport/total-over, minLen), track)
	travel = max(track-length, 0)
	t := min(max(offset/span, 0), 1)
	start := ScrollHandleMargin + travel*t
	if h.Horizontal {
		rect = Bounds{
			Origin: Point{X: start, Y: max(h.Frame.Height-ScrollHandleInset-thickness, 0)},
			Size:   Size{Width: length, Height: min(thickness, h.Frame.Height)},
		}
	} else {
		rect = Bounds{
			Origin: Point{X: max(h.Frame.Width-ScrollHandleInset-thickness, 0), Y: start},
			Size:   Size{Width: min(thickness, h.Frame.Width), Height: length},
		}
	}
	return rect, travel, true
}

// Rect returns the resting visual rectangle of the handle in the scroll view's
// viewport coordinates, its travel distance along the track, and false when
// the content fits without overflowing.
func (h ScrollHandle) Rect() (rect Bounds, travel Points, ok bool) {
	return h.rectWith(ScrollHandleThickness)
}

// ActiveRect returns the expanded visual rectangle of the handle while the
// user drags it, its travel distance along the track, and false when the
// content fits.
func (h ScrollHandle) ActiveRect() (rect Bounds, travel Points, ok bool) {
	return h.rectWith(ScrollHandleActiveThickness)
}

// GrabRect returns the touch hit-test rectangle around the handle in the
// scroll view's viewport coordinates, or false when the content fits.
func (h ScrollHandle) GrabRect() (Bounds, bool) {
	rect, _, ok := h.Rect()
	if !ok {
		return Bounds{}, false
	}
	if h.Horizontal {
		y := max(h.Frame.Height-ScrollHandleGrabThickness, 0)
		x0 := max(rect.Origin.X-ScrollHandleGrabSlop, 0)
		x1 := min(rect.Origin.X+rect.Size.Width+ScrollHandleGrabSlop, h.Frame.Width)
		return Bounds{Origin: Point{X: x0, Y: y}, Size: Size{Width: max(x1-x0, 0), Height: h.Frame.Height - y}}, true
	}
	x := max(h.Frame.Width-ScrollHandleGrabThickness, 0)
	y0 := max(rect.Origin.Y-ScrollHandleGrabSlop, 0)
	y1 := min(rect.Origin.Y+rect.Size.Height+ScrollHandleGrabSlop, h.Frame.Height)
	return Bounds{Origin: Point{X: x, Y: y0}, Size: Size{Width: h.Frame.Width - x, Height: max(y1-y0, 0)}}, true
}

// Contains reports whether p (in scroll view viewport coordinates) hits the
// handle's touch target.
func (h ScrollHandle) Contains(p Point) bool {
	r, ok := h.GrabRect()
	return ok && r.Contains(p)
}

// ScrollHandleDrag tracks a finger scrubbing a phone scroll handle along its
// active axis.
type ScrollHandleDrag struct {
	handle ScrollHandle
	from   Points
	start  Points
	travel Points
	span   Points
}

// BeginDrag starts dragging the handle from p (in scroll view viewport
// coordinates), or returns false when the handle cannot travel.
func (h ScrollHandle) BeginDrag(p Point) (ScrollHandleDrag, bool) {
	_, travel, ok := h.Rect()
	if !ok || travel <= 0 {
		return ScrollHandleDrag{}, false
	}
	viewport, total, offset, _ := h.along()
	span := total - viewport
	if span <= 0 {
		return ScrollHandleDrag{}, false
	}
	at := p.Y
	if h.Horizontal {
		at = p.X
	}
	return ScrollHandleDrag{
		handle: h,
		from:   at,
		start:  min(max(offset, 0), span),
		travel: travel,
		span:   span,
	}, true
}

// Shift adjusts an active drag by a relative scroll (ItemScrollBy) so
// continuing to drag stays anchored to the same content.
func (d *ScrollHandleDrag) Shift(by Points) {
	d.start = min(max(d.start+by, 0), d.span)
}

// Offset returns the scroll view's content offset for a finger at p (in scroll
// view viewport coordinates), clamped to the scrollable content range.
func (d ScrollHandleDrag) Offset(p Point) Point {
	out := d.handle.Offset
	if d.travel <= 0 || d.span <= 0 {
		return out
	}
	at := p.Y
	if d.handle.Horizontal {
		at = p.X
	}
	to := min(max(d.start+(at-d.from)*d.span/d.travel, 0), d.span)
	if d.handle.Horizontal {
		out.X = to - d.handle.Inset.Left
	} else {
		out.Y = to - d.handle.Inset.Top
	}
	return out
}
