package core

// Pointer events: where a press, hover or secondary click happened and
// which modifier keys were held, as NSH's PressEvent, HoverEvent and
// ContextMenuEvent report them. Positions are in points.

// Modifiers are the modifier keys held during an event.
type Modifiers struct {
	Control, Alt, Shift bool
	// Platform is Command on Apple platforms, the Windows or Super key
	// elsewhere.
	Platform bool
}

// PressEvent is where the pointer was at a press. Backends that know them
// report the modifier keys held when the press began and the click count;
// a press from assistive technology or the keyboard reports none.
type PressEvent struct {
	// Position is in the pressed view, its transforms undone; for a
	// pressed span, in its text's view.
	Position Point
	// PagePosition is in the root view the view is in: the surface, or
	// its modal.
	PagePosition Point
	Modifiers    Modifiers
	// Clicks counts consecutive clicks at the same place: 1 for a click,
	// 2 for a double click.
	Clicks int
}

// HoverEvent is where the mouse is over a hovered view.
type HoverEvent struct {
	Position     Point
	PagePosition Point
	Modifiers    Modifiers
}

// ContextMenuEvent is where a context menu was asked for. PagePosition is
// where a menu for it opens.
type ContextMenuEvent struct {
	Position     Point
	PagePosition Point
	Modifiers    Modifiers
}

func (e Event) press() PressEvent {
	return PressEvent{Position: e.Point, PagePosition: e.Page, Modifiers: e.Modifiers, Clicks: int(e.Count)}
}

// PointerEvents says which of a view and its children take touches and
// the pointer.
type PointerEvents uint8

const (
	// PointerEventsAuto lets the view and its children take them.
	PointerEventsAuto PointerEvents = iota
	// PointerEventsNone lets neither take them: they reach what is below.
	PointerEventsNone
	// PointerEventsBoxNone lets only the children take them.
	PointerEventsBoxNone
	// PointerEventsBoxOnly lets only the view take them, never its
	// children.
	PointerEventsBoxOnly
)

// Reach reports what a point p, in the coordinates of a view's parent's
// content, may reach of a view at frame with props: the view itself, by
// its bounds and HitSlop, and its children, by its bounds and area, the
// one ItemOverflow gave it. Platforms that hit-test their own views ask
// it on the way down.
func Reach(frame Bounds, props *ViewProps, area Bounds, p Point) (self, children bool) {
	pe, slop := PointerEventsAuto, Edges{}
	if props != nil {
		pe, slop = props.PointerEvents, props.HitSlop
	}
	if pe == PointerEventsNone {
		return false, false
	}
	in := frame.Contains(p)
	if pe != PointerEventsBoxNone {
		self = in || slop != (Edges{}) && Bounds{
			Origin: Point{frame.Origin.X - slop.Left, frame.Origin.Y - slop.Top},
			Size:   Size{frame.Size.Width + slop.horizontal(), frame.Size.Height + slop.vertical()},
		}.Contains(p)
	}
	if pe != PointerEventsBoxOnly {
		children = in || area.Size.Width > 0 && area.Size.Height > 0 && Bounds{
			Origin: Point{frame.Origin.X + area.Origin.X, frame.Origin.Y + area.Origin.Y}, Size: area.Size,
		}.Contains(p)
	}
	return self, children
}

// overflowArea returns the area n's children cover outside its bounds,
// in its own coordinates and holding them, or zero Bounds when they stay
// inside or n clips them: UIKit, AppKit and Android look for a touch's
// target only inside a view's bounds, so platforms look in the children
// of a view with an area too. Their own areas count, transforms do not.
func (n *node) overflowArea(kids []*node) Bounds {
	x, y := n.style.overflow()
	if n.scroll != nil || x == Hidden || y == Hidden || x == OverflowScroll || y == OverflowScroll || len(kids) == 0 {
		return Bounds{}
	}
	w, h := n.frame.Size.Width, n.frame.Size.Height
	x0, y0, x1, y1 := Points(0), Points(0), w, h
	grow := func(b Bounds) {
		x0, y0 = min(x0, b.Origin.X), min(y0, b.Origin.Y)
		x1, y1 = max(x1, b.Origin.X+b.Size.Width), max(y1, b.Origin.Y+b.Size.Height)
	}
	for _, c := range kids {
		if c.style.Display == DisplayNone || c.hidden || c.attach != nil {
			continue
		}
		if c.frame.Size.Width > 0 && c.frame.Size.Height > 0 {
			grow(c.frame)
		}
		if a := c.sent.overflow; a.Size.Width > 0 {
			grow(Bounds{Origin: Point{c.frame.Origin.X + a.Origin.X, c.frame.Origin.Y + a.Origin.Y}, Size: a.Size})
		}
	}
	const epsilon = 0.5
	if x0 >= -epsilon && y0 >= -epsilon && x1 <= w+epsilon && y1 <= h+epsilon {
		return Bounds{}
	}
	return Bounds{Origin: Point{x0, y0}, Size: Size{x1 - x0, y1 - y0}}
}
