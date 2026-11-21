package core

import (
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Pointer events: where a press, the mouse or a secondary click was and
// which modifier keys were held, as NSH's PressEvent, MouseEvent,
// MouseButtonEvent and ContextMenuEvent report them. Positions are in points.

// The mouse buttons of a MouseEvent or MouseButtonEvent; none is the zero value.
const (
	MouseButtonNone    = driver.MouseButtonNone
	MouseButtonLeft    = driver.MouseButtonLeft
	MouseButtonRight   = driver.MouseButtonRight
	MouseButtonMiddle  = driver.MouseButtonMiddle
	MouseButtonBack    = driver.MouseButtonBack
	MouseButtonForward = driver.MouseButtonForward
)

// PressEvent is where the pointer was at a press. Backends that know them report the modifier keys held when the press began and the click count; a keyboard press keeps its modifiers where the platform reports them. Keyboard and assistive-technology presses are at the view's origin with one click.
type PressEvent struct {
	// Position is in the pressed view, its transforms undone; for a
	// pressed span, in its text's view.
	Position Point
	// PagePosition is in the root view the view is in: the surface, or
	// its modal.
	PagePosition Point
	Modifiers    Modifiers
	// Clicks counts consecutive clicks on the same view at the same place: 1 for a click,
	// 2 for a double click.
	Clicks int
	// Keyboard is set when the last input was a key, as Return or Space
	// pressing the focused view: a menu opened so focuses its first item.
	Keyboard bool
}

// MouseEvent is where the mouse is over a view, as GPUI's MouseMoveEvent
// says, or was last when it leaves.
type MouseEvent struct {
	Position     Point
	PagePosition Point
	Modifiers    Modifiers
	// PressedButton is the button held down, none as the mouse hovers.
	PressedButton MouseButton
}

// MouseButtonEvent is a mouse button going down on a view (View.OnMouseDown)
// or coming up after going down there (View.OnMouseUp).
type MouseButtonEvent struct {
	// Position is in the view, its transforms undone.
	Position Point
	// PagePosition is in the root view the view is in: the surface, or
	// its modal.
	PagePosition Point
	// Modifiers are the modifier keys held.
	Modifiers Modifiers
	// Button is the mouse button that went down or came up.
	Button MouseButton
	// Clicks counts consecutive clicks of the button at the same place:
	// 1 for a click, 2 for a double click, and 0 when a held button is
	// released because the window lost focus.
	Clicks int
}

// MouseButtonsDown returns the mouse buttons currently held on cx's window, in
// button order (Left, Right, Middle, Back, Forward). It is a tracked read, so
// what reads it renders again as a button goes down or comes up, and a
// RequestAnimationFrame callback, which runs untracked, reads the same buttons,
// as a game's frame loop polls them. A button still held as the window loses
// focus comes up then.
//
// The first read on a window asks its platform for every mouse button press and
// release, not only those on a view with OnMouseDown or OnMouseUp, and that
// stays on for the life of the window; a button already held then is unknown
// until it is pressed again. macOS, Windows, Linux and the web report them;
// mobile targets do not, so none is ever held there.
func MouseButtonsDown(cx *Scope) []MouseButton {
	t := cx.tree()
	if t == nil {
		return nil
	}
	return slices.Clone(t.mouseButtonsHeld().Get())
}

// MouseButtonHeld reports whether button is held on cx's window, a tracked read
// of MouseButtonsDown's buttons. MouseButtonNone is never held.
func MouseButtonHeld(cx *Scope, button MouseButton) bool {
	if button < MouseButtonLeft || button > MouseButtonForward {
		return false
	}
	t := cx.tree()
	if t == nil {
		return false
	}
	return slices.Contains(t.mouseButtonsHeld().Get(), button)
}

// mouseButtonsHeld is the signal MouseButtonsDown reads, made by the first
// read, which asks the platform for every mouse button going down and up
// (driver.MountItem.HeldMouseButtons) with the next keymap.
func (t *Tree) mouseButtonsHeld() *Signal[[]MouseButton] {
	if t.heldMouseRead == nil {
		t.heldMouseRead = NewSignalFunc(t.root, t.heldButtonList(), slices.Equal[[]MouseButton])
		t.heldMouse = true
		t.keysChanged, t.changed = true, true
	}
	return t.heldMouseRead
}

// holdMouseButton records button b going down or coming up on the surface.
func (t *Tree) holdMouseButton(b driver.MouseButton, down bool) {
	if b < MouseButtonLeft || b > MouseButtonForward {
		return
	}
	bit := uint8(1 << b)
	next := t.heldButtons
	if down {
		next |= bit
	} else {
		next &^= bit
	}
	if next == t.heldButtons {
		return
	}
	t.heldButtons = next
	if t.heldMouseRead != nil {
		t.heldMouseRead.Set(t.heldButtonList())
	}
}

// heldButtonList returns the buttons currently held, in button order.
func (t *Tree) heldButtonList() []MouseButton {
	if t.heldButtons == 0 {
		return nil
	}
	out := make([]MouseButton, 0, 5)
	for b := MouseButtonLeft; b <= MouseButtonForward; b++ {
		if t.heldButtons&(1<<b) != 0 {
			out = append(out, b)
		}
	}
	return out
}

// ContextMenuEvent is where a context menu was asked for. PagePosition is
// where a menu for it opens.
type ContextMenuEvent struct {
	Position     Point
	PagePosition Point
	Modifiers    Modifiers
}

func pressOf(e driver.Event) PressEvent {
	return PressEvent{Position: e.Point, PagePosition: e.Page, Modifiers: e.Modifiers, Clicks: max(int(e.Count), 1)}
}

func mouseOf(e driver.Event) MouseEvent {
	return MouseEvent{Position: e.Point, PagePosition: e.Page, Modifiers: e.Modifiers, PressedButton: e.Button}
}

func mouseButtonOf(e driver.Event, down bool) MouseButtonEvent {
	clicks := int(e.Count)
	if down {
		clicks = max(clicks, 1)
	}
	return MouseButtonEvent{Position: e.Point, PagePosition: e.Page, Modifiers: e.Modifiers, Button: e.Button, Clicks: clicks}
}

// overflowArea returns the area n's children cover outside its bounds, in its own coordinates and holding them, or zero Bounds when they stay inside or n clips them: UIKit, AppKit and Android look for a touch's target only inside a view's bounds, so platforms look in the children of a view with an area too. Their own areas count by layout frames; transformed reports a child, or one of theirs, drawn transformed, which may show anywhere.
func (n *node) overflowArea(kids []*node) (area Bounds, transformed bool) {
	x, y := n.style.overflow()
	if n.scroll != nil || n.confines || x == Hidden || y == Hidden || x == OverflowScroll || y == OverflowScroll || len(kids) == 0 {
		return Bounds{}, false
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
		transformed = transformed || c.transformed || c.sent.transformed
		if c.frame.Size.Width > 0 && c.frame.Size.Height > 0 {
			grow(c.frame)
		}
		if a := c.sent.overflow; a.Size.Width > 0 {
			grow(Bounds{Origin: Point{X: c.frame.Origin.X + a.Origin.X, Y: c.frame.Origin.Y + a.Origin.Y}, Size: a.Size})
		}
	}
	const epsilon = 0.5
	if x0 >= -epsilon && y0 >= -epsilon && x1 <= w+epsilon && y1 <= h+epsilon {
		return Bounds{}, transformed
	}
	return Bounds{Origin: Point{X: x0, Y: y0}, Size: Size{Width: x1 - x0, Height: y1 - y0}}, transformed
}

// mayTransform reports whether what n was last sent can draw it away from its frame: a transform, keyframes or a bound transform.
func (n *node) mayTransform() bool {
	vs := &n.sent.viewStyle
	if vs.Transform != (driver.Transform{}) || vs.Animation.Keyframes != 0 {
		return true
	}
	if a := n.anim; a != nil {
		for _, p := range [...]driver.AnimatedProp{driver.PropTranslateX, driver.PropTranslateY, driver.PropScale, driver.PropScaleX,
			driver.PropScaleY, driver.PropRotate, driver.PropRotateX, driver.PropRotateY} {
			if a.sent[p] != 0 {
				return true
			}
		}
	}
	return false
}

// pointerMatrix is the transform sent to the platform, with a parent's perspective about the same viewport center the backend uses.
func (n *node) pointerMatrix() driver.Matrix3D {
	m := n.sent.viewStyle.Matrix(n.frame.Size)
	if parent := n.nativeParent(); parent != nil {
		at := n.frame.Origin
		if parent.scroll != nil {
			at.X -= parent.scroll.offset.X
			at.Y -= parent.scroll.offset.Y
		}
		m = driver.ChildPerspective(parent.sent.viewStyle.ChildPerspective, parent.frame.Size, at).Mul(m)
	}
	return m
}

func (n *node) surfacePoint(p Point) Point {
	if parent := n.nativeParent(); parent != nil {
		p = parent.surfacePoint(p)
		if parent.scroll != nil {
			p.X += parent.scroll.offset.X
			p.Y += parent.scroll.offset.Y
		}
	}
	local, _ := driver.IntoView(n.frame, n.pointerMatrix(), p)
	return local
}
