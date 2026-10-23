package core

import (
	"math"
	"slices"
)

// ScrollBehavior is how a scroll view scrolls, as NSH's Scrollable props
// (React Native's ScrollView props) say: Scroll's Behavior. Its
// zero value scrolls freely, bounces and decelerates as the platform does.
type ScrollBehavior struct {
	// Paging stops at multiples of the viewport.
	Paging bool
	// SnapToInterval stops at multiples of this length; 0 turns it off.
	SnapToInterval Points
	// SnapToOffsets stops at these offsets.
	SnapToOffsets []Points
	// SnapToAlignment is the point of a SnapToInterval item that aligns
	// with the viewport.
	SnapToAlignment SnapAlignment
	// DisableIntervalMomentum moves a fling one snap point at most.
	DisableIntervalMomentum bool
	// DecelerationRate is the velocity a fling keeps per millisecond:
	// DecelerationNormal, DecelerationFast or between; 0 is the
	// platform's.
	DecelerationRate float32
	// NoScroll keeps the user from scrolling it (React Native's
	// scrollEnabled false); the app still can.
	NoScroll bool
	// NoBounces stops drags at the edges instead of rubber banding.
	NoBounces bool
	// ContentInset is room around the content that scrolls with it.
	ContentInset Edges
	// KeyboardDismissMode is whether dragging dismisses the software
	// keyboard.
	KeyboardDismissMode KeyboardDismissMode
	// KeyboardShouldPersistTaps is whether a tap while the software
	// keyboard shows dismisses it.
	KeyboardShouldPersistTaps KeyboardTaps
}

// Deceleration rates, as UIKit's.
const (
	DecelerationNormal float32 = 0.998
	DecelerationFast   float32 = 0.99
)

// SnapAlignment is where a snapped item sits in the viewport.
type SnapAlignment uint8

const (
	SnapStart SnapAlignment = iota
	SnapCenter
	SnapEnd
)

// KeyboardDismissMode is whether dragging a scroll view dismisses the
// software keyboard.
type KeyboardDismissMode uint8

const (
	KeyboardDismissNone KeyboardDismissMode = iota
	// KeyboardDismissOnDrag dismisses it as a drag starts.
	KeyboardDismissOnDrag
	// KeyboardDismissInteractive follows the finger dragging down into
	// it (iOS; on drag elsewhere).
	KeyboardDismissInteractive
)

// KeyboardTaps is what a tap in a scroll view does while the software
// keyboard shows.
type KeyboardTaps uint8

const (
	// KeyboardTapsNever dismisses the keyboard, and the tap reaches
	// nothing else.
	KeyboardTapsNever KeyboardTaps = iota
	// KeyboardTapsAlways never dismisses it.
	KeyboardTapsAlways
	// KeyboardTapsHandled dismisses it unless something handles the tap.
	KeyboardTapsHandled
)

// SnapPoints returns the offsets, in order, a view snapping or paging
// along a viewport and content of those lengths stops at, or none when it
// does not snap. Backends without SnapTarget's hook, such as the web's
// CSS scroll snapping, mark them.
func (b *ScrollBehavior) SnapPoints(viewport, content Points) []Points {
	limit := max(content-viewport, 0)
	var points []Points
	switch {
	case b.SnapToInterval > 0:
		interval := b.SnapToInterval
		var shift Points // aligns the item at k * interval with the viewport
		switch b.SnapToAlignment {
		case SnapCenter:
			shift = (viewport - interval) / 2
		case SnapEnd:
			shift = viewport - interval
		}
		n := max(int(math.Ceil(float64((limit+shift)/interval))), 0)
		for i := 0; i <= n; i++ {
			points = append(points, min(max(Points(i)*interval-shift, 0), limit))
		}
	case len(b.SnapToOffsets) > 0:
		for _, o := range b.SnapToOffsets {
			points = append(points, min(max(o, 0), limit))
		}
		slices.Sort(points)
	case b.Paging:
		page := max(viewport, 1)
		n := int(math.Ceil(float64(limit / page)))
		for i := 0; i <= n; i++ {
			points = append(points, min(Points(i)*page, limit))
		}
	}
	return points
}

// Snaps reports whether flings end at snap points: snapping or paging.
func (b *ScrollBehavior) Snaps() bool {
	return b.Paging || b.SnapToInterval > 0 || len(b.SnapToOffsets) > 0
}

// Deceleration returns the velocity a fling keeps per millisecond:
// DecelerationRate, else DecelerationFast when it snaps and
// DecelerationNormal otherwise.
func (b *ScrollBehavior) Deceleration() float32 {
	switch {
	case b.DecelerationRate > 0 && b.DecelerationRate < 1:
		return b.DecelerationRate
	case b.Snaps():
		return DecelerationFast
	}
	return DecelerationNormal
}

// Projected returns where a glide from offset at velocity (points per
// millisecond) comes to rest, decelerating by rate per millisecond.
func Projected(offset Points, velocity, rate float32) Points {
	return offset - Points(velocity/float32(math.Log(float64(rate))))
}

// SnapTarget returns the snap point a fling ends at, as React Native's,
// and false when the view neither snaps nor pages. All values are along
// the main axis: offset is where the drag ended, projected where the
// fling would glide to (Projected), velocity its speed in points per
// millisecond (positive: the offset grows), for a viewport and content of
// those lengths. Pages, and DisableIntervalMomentum, move one snap point;
// other snapping rounds the projected offset in the fling's direction.
// Platforms call it, or Android's port, as the user lets go.
func (b *ScrollBehavior) SnapTarget(offset, projected Points, velocity float32, viewport, content Points) (Points, bool) {
	points := b.SnapPoints(viewport, content)
	if len(points) == 0 {
		return 0, false
	}
	limit := max(content-viewport, 0)
	const slow = 0.3 // slower releases settle at the nearest point
	step := b.DisableIntervalMomentum || b.Paging && b.SnapToInterval <= 0 && len(b.SnapToOffsets) == 0
	from := projected
	if step {
		from = offset
	}
	var target Points
	found := false
	switch {
	case velocity > slow:
		for _, p := range points {
			if step && p > from+0.5 || !step && p >= from-0.5 {
				target, found = p, true
				break
			}
		}
		if !found {
			target, found = limit, true
		}
	case velocity < -slow:
		for i := len(points) - 1; i >= 0; i-- {
			if p := points[i]; step && p < from-0.5 || !step && p <= from+0.5 {
				target, found = p, true
				break
			}
		}
		if !found {
			target, found = 0, true
		}
	default:
		target = points[0]
		for _, p := range points[1:] {
			if abs32(p-from) < abs32(target-from) {
				target = p
			}
		}
	}
	return target, true
}

func abs32(p Points) Points {
	if p < 0 {
		return -p
	}
	return p
}

func (b *ScrollBehavior) equal(o *ScrollBehavior) bool {
	return b.Paging == o.Paging && b.SnapToInterval == o.SnapToInterval &&
		slices.Equal(b.SnapToOffsets, o.SnapToOffsets) && b.SnapToAlignment == o.SnapToAlignment &&
		b.DisableIntervalMomentum == o.DisableIntervalMomentum && b.DecelerationRate == o.DecelerationRate &&
		b.NoScroll == o.NoScroll && b.NoBounces == o.NoBounces && b.ContentInset == o.ContentInset &&
		b.KeyboardDismissMode == o.KeyboardDismissMode && b.KeyboardShouldPersistTaps == o.KeyboardShouldPersistTaps
}

// ScrollEvent is where a scroll view is: what its scroll, drag and
// momentum events report.
type ScrollEvent struct {
	// Offset is the content's offset, UIKit's contentOffset: negative
	// past the start, as ContentInset allows.
	Offset Point
	// ContentSize is the content's size, zoomed.
	ContentSize Size
	// Size is the scroll view's.
	Size Size
	// ZoomScale is how far the content is zoomed; 1 unzoomed.
	ZoomScale float32
}

// ZoomRequest is ItemZoomTo's: the scale to zoom to, and the content
// point (unzoomed) to show in the middle of the viewport, or nil to keep
// the middle where it is.
type ZoomRequest struct {
	Scale  float32
	Center *Point
}

// Zooms reports whether the content zooms: MaxZoomScale above
// MinZoomScale.
func (c *ScrollConfig) Zooms() bool { return c.MaxZoomScale > c.MinZoomScale }

// Equal reports whether c and o configure a scroll view the same.
func (c *ScrollConfig) Equal(o *ScrollConfig) bool {
	return c.Horizontal == o.Horizontal && c.HideIndicator == o.HideIndicator && c.Behavior.equal(&o.Behavior) &&
		c.Refreshable == o.Refreshable && c.Refreshing == o.Refreshing &&
		c.MinZoomScale == o.MinZoomScale && c.MaxZoomScale == o.MaxZoomScale && c.ZoomScale == o.ZoomScale &&
		c.NoBouncesZoom == o.NoBouncesZoom && c.NoPinchZoom == o.NoPinchZoom && c.CenterContent == o.CenterContent
}

// scrollSpec is what Scroll and List give a scroll view, beside its
// direction and indicator.
type scrollSpec struct {
	behavior      ScrollBehavior
	behaviorFunc  func() ScrollBehavior
	offset        Point // the initial one
	refreshing    func() bool
	onRefresh     func()
	onScroll      func(ScrollEvent)
	onDragStart   func(ScrollEvent)
	onDragEnd     func(ScrollEvent)
	onMomentum    func(ScrollEvent)
	onMomentumEnd func(ScrollEvent)
	// Zooming, for Scroll only.
	minZoom, maxZoom, zoom float32
	zoomFunc               func() float32
	noBouncesZoom, noPinch bool
	centerContent          bool
}

// bindScrollSpec configures scroll view n from s: its ScrollConfig, kept
// up to date, its handlers and its initial offset.
func (t *Tree) bindScrollSpec(cx *Scope, n *node, s scrollSpec) {
	sc := n.scroll
	c := &sc.config
	c.Behavior = s.behavior
	c.Refreshable = s.onRefresh != nil
	c.MinZoomScale = zoomOr(s.minZoom, 1)
	c.MaxZoomScale = max(zoomOr(s.maxZoom, 1), c.MinZoomScale)
	c.ZoomScale = c.clampZoom(zoomOr(s.zoom, 1))
	c.NoBouncesZoom, c.NoPinchZoom, c.CenterContent = s.noBouncesZoom, s.noPinch, s.centerContent
	sc.zoom = c.ZoomScale
	sc.on = s
	if f := s.behaviorFunc; f != nil {
		cx.renderEffect(func() {
			b := f()
			if !b.equal(&sc.config.Behavior) {
				sc.config.Behavior = b
				t.touch(n)
			}
		})
	}
	if f := s.refreshing; f != nil {
		cx.renderEffect(func() {
			if r := f(); r != sc.config.Refreshing {
				sc.config.Refreshing = r
				t.touch(n)
			}
		})
	}
	if f := s.zoomFunc; f != nil {
		cx.renderEffect(func() {
			if z := sc.config.clampZoom(zoomOr(f(), 1)); z != sc.config.ZoomScale {
				sc.config.ZoomScale = z
				t.touch(n)
			}
		})
	}
	if s.offset != (Point{}) {
		t.scrollTo(n, s.offset, false)
	}
}

func zoomOr(z, or float32) float32 {
	if z > 0 && z == z {
		return z
	}
	return or
}

func (c *ScrollConfig) clampZoom(z float32) float32 {
	return min(max(z, c.MinZoomScale), c.MaxZoomScale)
}

// scrollEvent is where scroll view n is, with its offset at p.
func (n *node) scrollEvent(p Point) ScrollEvent {
	sc := n.scroll
	return ScrollEvent{Offset: p, ZoomScale: sc.zoom, Size: n.frame.Size,
		ContentSize: Size{sc.content.Width * Points(sc.zoom), sc.content.Height * Points(sc.zoom)}}
}

// dispatchScroll handles scroll view n's scroll events.
func (t *Tree) dispatchScroll(n *node, ev Event) {
	sc := n.scroll
	if ev.Kind != EventRefresh && ev.Value > 0 {
		sc.zoom = ev.Value
	}
	call := func(fn func(ScrollEvent)) {
		if fn != nil {
			e := n.scrollEvent(ev.Point)
			t.within(n.owner, func() { fn(e) })
		}
	}
	switch ev.Kind {
	case EventScrollDragStart:
		call(sc.on.onDragStart)
	case EventScrollDragEnd:
		call(sc.on.onDragEnd)
	case EventMomentumStart:
		call(sc.on.onMomentum)
	case EventMomentumEnd:
		call(sc.on.onMomentumEnd)
	case EventRefresh:
		// The platform shows its spinner as the pull ends; the next
		// config tells it whether the app keeps it.
		sc.resend = true
		t.changed = true
		if sc.on.onRefresh != nil {
			t.within(n.owner, sc.on.onRefresh)
		}
	}
}

func (sc *scrollNode) events() EventMask {
	var m EventMask
	bit := func(k EventKind, on bool) {
		if on {
			m |= 1 << k
		}
	}
	bit(EventScroll, sc.on.onScroll != nil || sc.config.Zooms() || len(sc.dependents) > 0)
	bit(EventScrollDragStart, sc.on.onDragStart != nil)
	bit(EventScrollDragEnd, sc.on.onDragEnd != nil)
	bit(EventMomentumStart, sc.on.onMomentum != nil)
	bit(EventMomentumEnd, sc.on.onMomentumEnd != nil)
	bit(EventRefresh, sc.on.onRefresh != nil)
	return m
}
