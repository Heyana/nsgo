package nsgotest

import (
	"math"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	// dragStep is how far a finger or a scroll moves in one display frame.
	dragStep = 10
	// landing is how long a finger rests where it lands before it moves:
	// the views under it hear it land first.
	landing = 100 * time.Millisecond
	// pressSlop is how far a finger may move and still press.
	pressSlop = 8
)

// dragMode is what a drag turned into.
type dragMode uint8

const (
	dragIdle   dragMode = iota // nothing started yet
	dragFree                   // nothing native will: the responder took it, or nothing could
	dragPan                    // a view's pan
	dragScroll                 // a scroll view
)

// finger is the finger of a Drag.
type finger struct {
	start, at, last driver.Point
	elapsed         time.Duration
	chain           []*View // the view it landed on and those around it
	reporting       bool    // the views listening to touches hear it
	press           *View   // the pressable view under it
	pressed         bool    // press heard PressDown
	cancelled       bool    // it moved away from press
	mode            dragMode
	view            *View         // the pan's or scroll view's
	id              int           // its Touch.ID: 0 for Drag's, as a mouse's
	t0              time.Duration // when it went down, which reports count from
	downAt          time.Time     // when it went down, on the clock
	from            driver.Point  // the scroll view's offset when it took the drag
	grabbed         driver.Point  // where the finger was then
	moved           bool
}

// Drag puts a finger on the middle of what selector names, rests it there
// for 100 ms, moves it through each point of path in turn (offsets from
// where it landed) 10 points a display frame, and lifts it, as the
// platforms decide what takes it: the views listening to touches hear
// them, past the slop a pan or the scroll view the drag runs along takes
// it unless the touch responder holds it, and the press under the finger
// is cancelled once it moves away. The clock moves with the frames.
func (a *App) Drag(selector string, path ...driver.Point) {
	a.t.Helper()
	a.step("drag(%s, %v)", selector, path)
	v := a.pick(selector, "view", func(m *View) (*View, bool) { return m, true })
	_, start := v.center()
	if a.target != nil {
		start = a.target.reveal(v)
	}
	a.drag(v, start, path)
}

// DragAt drags as Drag does from a point of the view selector matches,
// in its coordinates, rather than its middle: for what a finger takes by
// a part of its own, such as a selection's handle.
func (a *App) DragAt(selector string, at driver.Point, path ...driver.Point) {
	a.t.Helper()
	a.step("dragAt(%s, %v, %v)", selector, at, path)
	v := a.pick(selector, "view", func(m *View) (*View, bool) { return m, true })
	if a.target != nil {
		a.target.reveal(v)
	}
	a.drag(v, v.pagePoint(at), path)
}

// drag puts a finger on v at the surface point start and moves it
// through path, offsets from start.
func (a *App) drag(v *View, start driver.Point, path []driver.Point) {
	a.t.Helper()
	points := make([]driver.Point, len(path))
	for i, d := range path {
		points[i] = driver.Point{X: start.X + d.X, Y: start.Y + d.Y}
	}
	if a.target != nil {
		a.target.drag(start, points)
		a.done()
		return
	}
	f := &finger{start: start, at: start, last: start, reporting: a.listensToTouches(), downAt: a.Clock.Now()}
	for p := a.viewAt(start, func(*View) bool { return true }); p != nil; p = p.Parent {
		f.chain = append(f.chain, p)
	}
	// A finger going down ends what a responder held before.
	a.blocked = false
	inScroll := false
	for _, p := range f.chain {
		if f.press == nil && (pressable(p) || p.Events.Has(driver.EventLongPress)) && !v.disabled(p) {
			f.press = p
		}
		inScroll = inScroll || p != v && p.Kind == driver.KindScroll && !p.Scroll.Behavior.NoScroll
	}
	a.touchAt(driver.GestureBegan, start, 0)
	a.pressOutside(v, start)
	// Inside a scroll view a touch presses only once it rests.
	if !inScroll {
		a.pressIn(f)
	}
	for f.elapsed+driver.FrameInterval <= landing {
		a.frame(f)
	}
	a.pressIn(f)
	prev := start
	for _, to := range points {
		n := int(math.Ceil(math.Max(math.Abs(float64(to.X-prev.X)), math.Abs(float64(to.Y-prev.Y))) / dragStep))
		for i := 1; i <= max(n, 1); i++ {
			a.frame(f)
			t := driver.Points(i) / driver.Points(max(n, 1))
			a.fingerMoved(f, driver.Point{X: prev.X + (to.X-prev.X)*t, Y: prev.Y + (to.Y-prev.Y)*t})
		}
		prev = to
	}
	a.fingerLifted(f)
	a.done()
}

// Finger is a finger a test puts down with Touch and moves a step at a
// time, as Drag does in one: the views listening to touches hear it,
// presses and the touch responder follow it, and past the slop a pan or
// a scroll view may take it.
type Finger struct {
	a    *App
	f    *finger
	view *View
	// matrix keeps the coordinates of the view where the touch began.
	matrix driver.Matrix3D
}

// Touch puts a finger down at at, in the coordinates of the view selector
// names, and reports it there.
func (a *App) Touch(selector string, at driver.Point) *Finger {
	a.t.Helper()
	a.step("touch(%s, %v)", selector, at)
	v := a.pick(selector, "view", func(m *View) (*View, bool) { return m, true })
	start := v.pagePoint(at)
	f := &finger{start: start, at: start, last: start, reporting: a.listensToTouches(), id: 1, downAt: a.Clock.Now(),
		t0: a.Clock.Now().Sub(a.start)}
	for p := a.viewAt(start, func(*View) bool { return true }); p != nil; p = p.Parent {
		f.chain = append(f.chain, p)
	}
	a.blocked = false
	for _, p := range f.chain {
		if f.press == nil && (pressable(p) || p.Events.Has(driver.EventLongPress)) && !v.disabled(p) {
			f.press = p
		}
	}
	// It names the view it went down on, as a platform that knows.
	if f.reporting {
		a.dispatch(driver.Event{Kind: driver.EventTouch, ID: v.ID, Phase: driver.GestureBegan, Time: f.t0,
			Touches: []driver.Touch{{ID: f.id, PagePosition: start, Changed: true}}})
	}
	a.pressOutside(v, start)
	a.pressIn(f)
	a.done()
	return &Finger{a: a, f: f, view: v, matrix: v.pageMatrix()}
}

// Move moves the finger to at, in the coordinates of the view it went
// down on, a display frame later.
func (g *Finger) Move(at driver.Point) {
	g.a.t.Helper()
	g.a.step("move(%v)", at)
	g.a.frame(g.f)
	g.a.fingerMoved(g.f, g.pagePoint(at))
	g.a.done()
}

// Lift lifts the finger, where it is: a tap where it went down presses.
func (g *Finger) Lift() {
	g.a.t.Helper()
	g.a.step("lift")
	g.a.frame(g.f)
	g.a.fingerLifted(g.f)
	g.a.done()
}

// LiftAt lifts the finger at at, in the coordinates of the view it went
// down on, a display frame later, with no move reported on the way.
func (g *Finger) LiftAt(at driver.Point) {
	g.a.t.Helper()
	g.a.step("lift(%v)", at)
	g.a.frame(g.f)
	f := g.f
	f.last, f.at = f.at, g.pagePoint(at)
	g.a.fingerLifted(f)
	g.a.done()
}

// Cancel ends the finger as the system does when it takes the touches:
// no press.
func (g *Finger) Cancel() {
	g.a.t.Helper()
	g.a.step("cancel")
	f := g.f
	if f.reporting {
		g.a.touchAt(driver.GestureCancelled, f.at, f.t0+f.elapsed, f.id)
	}
	if f.pressed && !f.cancelled {
		f.cancelled = true
		ev := g.a.pressEvent(f.press, f.start)
		ev.Kind = driver.EventPressUp
		g.a.send(f.press, ev)
	}
	g.a.done()
}

// frame takes one display frame of a gesture.
func (a *App) frame(f *finger) {
	a.elapse(driver.FrameInterval)
	f.elapsed += driver.FrameInterval
}

func (a *App) pressIn(f *finger) {
	if f.press != nil && !f.pressed && !f.cancelled {
		f.pressed = true
		ev := a.pressEvent(f.press, f.start)
		ev.Kind = driver.EventPressDown
		a.send(f.press, ev)
	}
}

func (a *App) fingerMoved(f *finger, at driver.Point) {
	f.last, f.at = f.at, at
	f.moved = true
	t := driver.Point{X: at.X - f.start.X, Y: at.Y - f.start.Y}
	if f.reporting {
		a.touchAt(driver.GestureChanged, at, f.t0+f.elapsed, f.id)
	}
	distance := driver.Points(math.Hypot(float64(t.X), float64(t.Y)))
	if f.pressed && !f.cancelled && distance > pressSlop {
		f.cancelled = true
		ev := a.pressEvent(f.press, f.start)
		ev.Kind = driver.EventPressUp
		a.send(f.press, ev)
	}
	// The touch responder heard the move above: a claim blocks native
	// drags before one starts.
	if f.mode == dragIdle {
		a.startDrag(f, t)
	}
	switch f.mode {
	case dragPan:
		a.pan(f, driver.GestureChanged, t)
	case dragScroll:
		a.dragScroll(f)
	}
}

// startDrag decides what takes a drag translated by t, as platforms do
// once it passes the slop: the innermost pan it activates, else the
// innermost scroll view it runs along that can move that way.
func (a *App) startDrag(f *finger, t driver.Point) {
	if a.blocked {
		f.mode = dragFree
		return
	}
	vertical := math.Abs(float64(t.Y)) >= math.Abs(float64(t.X))
	for _, v := range f.chain {
		if v.Events.Has(driver.EventPan) {
			switch v.Props.PanActivation.Decide(t) {
			case 1:
				f.mode, f.view = dragPan, v
				a.pan(f, driver.GestureBegan, t)
				return
			case 0:
				// An undecided pan holds the drag back.
				return
			}
		}
		if v.Kind == driver.KindScroll && !v.Scroll.Behavior.NoScroll && v.Scroll.Horizontal != vertical &&
			driver.Points(math.Hypot(float64(t.X), float64(t.Y))) >= driver.DefaultPanSlop && a.canScroll(v, t) {
			if f.reporting {
				// The scroll view takes the touches over.
				a.touchAt(driver.GestureCancelled, f.at, f.t0+f.elapsed, f.id)
				f.reporting = false
			}
			f.mode, f.view, f.from, f.grabbed = dragScroll, v, v.Offset, f.at
			a.scrollEvent(v, driver.EventScrollDragStart)
			return
		}
	}
	if driver.Points(math.Hypot(float64(t.X), float64(t.Y))) >= driver.DefaultPanSlop {
		f.mode = dragFree
	}
}

// canScroll reports whether scroll view v moves for a finger translated
// by t: its content goes that way, or a refreshable view is pulled down
// at its top.
func (a *App) canScroll(v *View, t driver.Point) bool {
	at, end, d := v.Offset.Y, v.scrollEnd(), -t.Y
	if v.Scroll.Horizontal {
		at, d = v.Offset.X, -t.X
	}
	if d < 0 {
		return at > 0 || !v.Scroll.Horizontal && v.Scroll.Refreshable && !v.Scroll.Refreshing
	}
	return at < end
}

// scrollEnd is the largest offset of scroll view v along its axis.
func (v *View) scrollEnd() driver.Points {
	in := v.Scroll.Behavior.ContentInset
	if v.Scroll.Horizontal {
		return max(v.Content.Width-v.Frame.Size.Width, 0) + in.Right
	}
	return max(v.Content.Height-v.Frame.Size.Height, 0) + in.Bottom
}

func (a *App) pan(f *finger, phase driver.GesturePhase, t driver.Point) {
	ev := driver.Event{Kind: driver.EventPan, ID: f.view.ID, Phase: phase, Point: t, Position: f.view.localPoint(f.at), PagePosition: f.at, Velocity: f.velocity()}
	// A platform moves bound values before it reports the move.
	a.anim.Gesture(&ev)
	a.dispatch(ev)
}

// velocity is the finger's, in points per second, over its last frame.
func (f *finger) velocity() driver.Point {
	s := driver.Points(driver.FrameInterval.Seconds())
	return driver.Point{X: (f.at.X - f.last.X) / s, Y: (f.at.Y - f.last.Y) / s}
}

// dragScroll moves the scroll view with the finger, within its content,
// or past its top when it refreshes.
func (a *App) dragScroll(f *finger) {
	v := f.view
	off := v.Offset
	if v.Scroll.Horizontal {
		off.X = min(max(f.from.X-(f.at.X-f.grabbed.X), -v.Scroll.Behavior.ContentInset.Left), v.scrollEnd())
	} else {
		lo := -v.Scroll.Behavior.ContentInset.Top
		if v.Scroll.Refreshable {
			lo = -math.MaxFloat32
		}
		off.Y = min(max(f.from.Y-(f.at.Y-f.grabbed.Y), lo), v.scrollEnd())
	}
	a.scrollView(v, off)
}

// scrollView moves v to off as a user's scroll and reports it.
func (a *App) scrollView(v *View, off driver.Point) {
	v.Offset = off
	a.anim.Scroll(v.ID, off)
	a.pullValue(v)
	a.dispatch(driver.Event{Kind: driver.EventScroll, ID: v.ID, Point: off, Value: v.ZoomScale, Count: v.Applied})
}

// pulled is how far vertical scroll view v is pulled past its top.
func (v *View) pulled() driver.Points {
	if v.Scroll.Horizontal {
		return 0
	}
	return max(-v.Offset.Y-v.Scroll.Behavior.ContentInset.Top, 0)
}

// pullValue sets the value bound to v's pull, as a platform does: how far
// it is pulled, and at least the room it keeps while refreshing.
func (a *App) pullValue(v *View) {
	if v.Scroll.Horizontal || !a.anim.Pulls(v.ID) {
		return
	}
	pull := v.pulled()
	if v.Scroll.Refreshing {
		pull = max(pull, v.Scroll.Behavior.RefreshInset())
	}
	a.anim.Pull(v.ID, pull)
}

func (a *App) fingerLifted(f *finger) {
	if f.reporting {
		a.touchAt(driver.GestureEnded, f.at, f.t0+f.elapsed, f.id)
	}
	switch f.mode {
	case dragPan:
		a.pan(f, driver.GestureEnded, driver.Point{X: f.at.X - f.start.X, Y: f.at.Y - f.start.Y})
	case dragScroll:
		v := f.view
		a.scrollEvent(v, driver.EventScrollDragEnd)
		if !v.Scroll.Horizontal && v.Offset.Y < -v.Scroll.Behavior.ContentInset.Top {
			if v.pulled() >= v.Scroll.Behavior.RefreshThreshold() && v.Scroll.Refreshable && !v.Scroll.Refreshing {
				a.dispatch(driver.Event{Kind: driver.EventRefresh, ID: v.ID, Point: v.Offset, Count: v.Applied})
			}
			// It springs back to its top.
			a.scrollEvent(v, driver.EventMomentumStart)
			a.scrollView(v, v.clamp(v.Offset))
			a.scrollEvent(v, driver.EventMomentumEnd)
		}
		a.settle(v)
	}
	if f.pressed && !f.cancelled {
		// Held past a long press's delay, it long-presses the view that
		// hears it, if one does, and presses nothing.
		delay := f.press.Props.DelayLongPress
		var long *View
		for _, p := range f.chain {
			if p.Events.Has(driver.EventLongPress) && !p.disabled(nil) {
				long, delay = p, p.Props.DelayLongPress
				break
			}
		}
		if delay <= 0 {
			delay = defaultLongPress
		}
		if a.Clock.Now().Sub(f.downAt) >= delay {
			if long != nil {
				ev := a.pressEvent(long, f.start)
				ev.Kind = driver.EventLongPress
				a.send(long, ev)
			}
			ev := a.pressEvent(f.press, f.start)
			ev.Kind = driver.EventPressUp
			a.send(f.press, ev)
			return
		}
		// A finger that lifts where it landed taps, and taps count up as
		// presses do.
		a.countClick(f.press)
		ev := a.pressEvent(f.press, f.start)
		ev.Kind, ev.Count = driver.EventPressUp, a.clicks
		a.send(f.press, ev)
		ev.Kind = driver.EventPress
		a.send(f.press, ev)
	}
}

// Hover moves the mouse over the middle of what selector names, as a
// desktop's pointer does: the views from it out that hover matters to
// (hover styles or hover events) replace those hovered before, which
// hear MouseLeave, the new ones MouseEnter, and each hovered view MouseMove.
func (a *App) Hover(selector string) {
	a.t.Helper()
	a.step("hover(%s)", selector)
	a.hoverAt(selector, nil)
}

// HoverAt moves the mouse over what selector names as Hover does, to at in
// its coordinates: a chart's row under the pointer.
func (a *App) HoverAt(selector string, at driver.Point) {
	a.t.Helper()
	a.step("hoverAt(%s, %v, %v)", selector, at.X, at.Y)
	a.hoverAt(selector, &at)
}

func (a *App) hoverAt(selector string, local *driver.Point) {
	a.t.Helper()
	v := a.pick(selector, "view", func(m *View) (*View, bool) { return m, true })
	at := a.reveal(v)
	if local != nil {
		at = v.pagePoint(*local)
	}
	if a.target != nil {
		a.target.hover(at)
		a.done()
		return
	}
	var next []*View
	for p := a.viewAt(at, func(*View) bool { return true }); p != nil; p = p.Parent {
		if p.Events.Has(driver.EventMouseEnter) || p.Events.Has(driver.EventMouseLeave) || p.Events.Has(driver.EventMouseMove) {
			next = append(next, p)
		}
	}
	a.mouseIn = true
	a.setHovered(next, at)
	btn := a.heldMouseButton()
	for _, h := range next {
		ev := a.pressEvent(h, at)
		ev.Kind, ev.Count, ev.Button = driver.EventMouseMove, 0, btn
		a.send(h, ev)
	}
	a.done()
}

// heldMouseButton is a mouse button MouseDown holds: tree and id are the surface and view its down went to (id 0 for none), clicks is its click count, and reported whether the platform would report its up.
type heldMouseButton struct {
	tree     *core.Tree
	id       driver.NodeID
	clicks   uint32
	reported bool
}

// mouseClickState tracks consecutive clicks of one button for click counting.
type mouseClickState struct {
	view  driver.NodeID
	at    time.Time
	count uint32
}

// heldMouseButton returns the first mouse button held by MouseDown, or MouseButtonNone.
func (a *App) heldMouseButton() driver.MouseButton {
	for b := driver.MouseButtonLeft; b <= driver.MouseButtonForward; b++ {
		if _, ok := a.heldButtons[b]; ok {
			return b
		}
	}
	return driver.MouseButtonNone
}

// mergeHeldModifiers merges explicit modifiers with any modifier keys currently held by KeyDown.
func (a *App) mergeHeldModifiers(mods driver.Modifiers) driver.Modifiers {
	held := a.heldModifiers()
	return driver.Modifiers{
		Control:  mods.Control || held.Control,
		Alt:      mods.Alt || held.Alt,
		Shift:    mods.Shift || held.Shift,
		Platform: mods.Platform || held.Platform,
	}
}

// checkMouseButton validates that button is one of the five mouse buttons.
func (a *App) checkMouseButton(button driver.MouseButton) {
	a.t.Helper()
	if button < driver.MouseButtonLeft || button > driver.MouseButtonForward {
		a.t.Fatalf("nsgotest: %s: mouse button must be one of left, right, middle, back or forward, got %s", a.stepName, button)
	}
}

// MouseDown presses button on the middle of what selector names and holds it until MouseUp or BlurWindow.
func (a *App) MouseDown(selector string, button driver.MouseButton) {
	a.t.Helper()
	a.MouseDownWith(selector, button, driver.Modifiers{})
}

// MouseDownWith presses button as MouseDown does, with mods held.
func (a *App) MouseDownWith(selector string, button driver.MouseButton, mods driver.Modifiers) {
	a.t.Helper()
	if mods == (driver.Modifiers{}) {
		a.step("mouseDown(%s, %s)", selector, button)
	} else {
		a.step("mouseDownWith(%s, %s, %s)", selector, button, modifierNames(mods))
	}
	a.mouseButton(selector, nil, button, mods, true)
}

// MouseDownAt presses button at local point at in what selector names and holds it until MouseUp or BlurWindow.
func (a *App) MouseDownAt(selector string, at driver.Point, button driver.MouseButton) {
	a.t.Helper()
	a.MouseDownAtWith(selector, at, button, driver.Modifiers{})
}

// MouseDownAtWith presses button as MouseDownAt does, with mods held.
func (a *App) MouseDownAtWith(selector string, at driver.Point, button driver.MouseButton, mods driver.Modifiers) {
	a.t.Helper()
	if mods == (driver.Modifiers{}) {
		a.step("mouseDownAt(%s, %v, %v, %s)", selector, at.X, at.Y, button)
	} else {
		a.step("mouseDownAtWith(%s, %v, %v, %s, %s)", selector, at.X, at.Y, button, modifierNames(mods))
	}
	a.mouseButton(selector, &at, button, mods, true)
}

// MouseUp releases button held by MouseDown at the middle of what selector names. Releasing a button not held fails the step.
func (a *App) MouseUp(selector string, button driver.MouseButton) {
	a.t.Helper()
	a.MouseUpWith(selector, button, driver.Modifiers{})
}

// MouseUpWith releases button as MouseUp does, with mods held.
func (a *App) MouseUpWith(selector string, button driver.MouseButton, mods driver.Modifiers) {
	a.t.Helper()
	if mods == (driver.Modifiers{}) {
		a.step("mouseUp(%s, %s)", selector, button)
	} else {
		a.step("mouseUpWith(%s, %s, %s)", selector, button, modifierNames(mods))
	}
	a.mouseButton(selector, nil, button, mods, false)
}

// MouseUpAt releases button held by MouseDown at local point at in what selector names.
func (a *App) MouseUpAt(selector string, at driver.Point, button driver.MouseButton) {
	a.t.Helper()
	a.MouseUpAtWith(selector, at, button, driver.Modifiers{})
}

// MouseUpAtWith releases button as MouseUpAt does, with mods held.
func (a *App) MouseUpAtWith(selector string, at driver.Point, button driver.MouseButton, mods driver.Modifiers) {
	a.t.Helper()
	if mods == (driver.Modifiers{}) {
		a.step("mouseUpAt(%s, %v, %v, %s)", selector, at.X, at.Y, button)
	} else {
		a.step("mouseUpAtWith(%s, %v, %v, %s, %s)", selector, at.X, at.Y, button, modifierNames(mods))
	}
	a.mouseButton(selector, &at, button, mods, false)
}

func (a *App) mouseButton(selector string, local *driver.Point, button driver.MouseButton, mods driver.Modifiers, down bool) {
	a.t.Helper()
	a.checkMouseButton(button)
	v := a.pick(selector, "view", func(m *View) (*View, bool) { return m, true })
	at := a.reveal(v)
	if local != nil {
		at = v.pagePoint(*local)
	}
	if a.target != nil {
		a.target.mouseButton(at, button, mods, down)
		a.done()
		return
	}
	mods = a.mergeHeldModifiers(mods)
	if down {
		a.inputBy(driver.ModalityPointer)
		if prev, ok := a.heldButtons[button]; ok {
			delete(a.heldButtons, button)
			if prev.reported && prev.tree != nil {
				prev.tree.Dispatch(driver.Event{Kind: driver.EventMouseUp, ID: prev.id, Page: at, Modifiers: mods, Button: button})
			}
		}
		target := a.viewAt(at, func(v *View) bool { return v.Events.Has(driver.EventMouseDown) })
		var targetID driver.NodeID
		if target != nil {
			targetID = target.ID
		}
		clicks := a.countMouseClick(button, targetID)
		entry := heldMouseButton{clicks: clicks}
		switch {
		case target != nil:
			entry = heldMouseButton{tree: a.tree, id: target.ID, clicks: clicks, reported: true}
			a.tree.Dispatch(driver.Event{
				Kind: driver.EventMouseDown, ID: target.ID,
				Point: target.localPoint(at), Page: at,
				Modifiers: mods, Button: button, Count: clicks,
			})
		case a.heldMouse:
			entry = heldMouseButton{tree: a.tree, clicks: clicks, reported: true}
			a.tree.Dispatch(driver.Event{
				Kind: driver.EventMouseDown, Page: at,
				Modifiers: mods, Button: button, Count: clicks,
			})
		}
		if a.heldButtons == nil {
			a.heldButtons = map[driver.MouseButton]heldMouseButton{}
		}
		a.heldButtons[button] = entry
		a.done()
		return
	}
	h, ok := a.heldButtons[button]
	if !ok {
		a.t.Fatalf("nsgotest: %s: %s button is not held; MouseDown holds a button", a.stepName, button)
	}
	delete(a.heldButtons, button)
	if h.reported && h.tree != nil {
		ev := driver.Event{
			Kind: driver.EventMouseUp, ID: h.id,
			Page: at, Modifiers: mods, Button: button, Count: h.clicks,
		}
		if h.id != 0 {
			if target := a.views[h.id]; target != nil {
				ev.Point = target.localPoint(at)
			}
		}
		h.tree.Dispatch(ev)
	}
	a.done()
}

// countMouseClick counts consecutive clicks of button on targetID within 500 ms.
func (a *App) countMouseClick(button driver.MouseButton, targetID driver.NodeID) uint32 {
	now := a.Clock.Now()
	if a.mouseClicks == nil {
		a.mouseClicks = map[driver.MouseButton]mouseClickState{}
	}
	st := a.mouseClicks[button]
	if st.view == targetID && !st.at.IsZero() && now.Sub(st.at) <= 500*time.Millisecond {
		st.count++
	} else {
		st.view = targetID
		st.count = 1
	}
	st.at = now
	a.mouseClicks[button] = st
	return st.count
}

// BlurWindow simulates the window losing focus (resign key / blur), releasing every held key and held mouse button just as desktop and web platforms do on focus loss.
func (a *App) BlurWindow() {
	a.t.Helper()
	a.step("blurWindow()")
	if a.target != nil {
		a.target.blurWindow()
		a.done()
		return
	}
	for name, h := range a.held {
		delete(a.held, name)
		if h.reported && h.tree != nil {
			h.tree.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: h.id, Text: name})
		}
	}
	for b := driver.MouseButtonLeft; b <= driver.MouseButtonForward; b++ {
		if h, ok := a.heldButtons[b]; ok {
			delete(a.heldButtons, b)
			if h.reported && h.tree != nil {
				h.tree.Dispatch(driver.Event{Kind: driver.EventMouseUp, ID: h.id, Button: b})
			}
		}
	}
	a.done()
}

// reveal brings the middle of a named view into its scroll viewports before picking at it, as a target's native steps do.
func (a *App) reveal(v *View) driver.Point {
	if a.target != nil {
		return a.target.reveal(v)
	}
	for range 30 {
		_, at := v.center()
		moved := false
		for p := v.Parent; p != nil; p = p.Parent {
			if p.Kind != driver.KindScroll || p.Scroll.Behavior.NoScroll {
				continue
			}
			local := p.localPoint(at)
			if (driver.Bounds{Size: p.Frame.Size}).Contains(local) {
				continue
			}
			off := p.Offset
			if p.Scroll.Horizontal {
				off.X += local.X - p.Frame.Size.Width/2
			} else {
				off.Y += local.Y - p.Frame.Size.Height/2
			}
			if off = p.clamp(off); off != p.Offset {
				a.scrollView(p, off)
				moved = true
				break
			}
		}
		if !moved {
			return at
		}
	}
	_, at := v.center()
	return at
}

// SelectText has the selection of the Selectable view selector names touch box of its texts, in the view, no Size clearing it, as the platforms report it. The text OnTextSelection receives is that of the texts box overlaps, whole, as a copy gives it: texts on one row joined by a tab and the others by a line break, hidden spans left out.
func (a *App) SelectText(selector string, box driver.Bounds) {
	a.t.Helper()
	a.step("selectText(%s, %v)", selector, box)
	v := a.pick(selector, "view", func(m *View) (*View, bool) { return m, true })
	text := ""
	if box.Size.Width > 0 && box.Size.Height > 0 {
		text = selectedText(v, box)
	}
	a.send(v, driver.Event{Kind: driver.EventSelectionChange, Point: box.Origin, Size: box.Size, Text: text})
	a.done()
}

// selectedText is the text of area's selectable texts that box, in area, overlaps, whole, through the model the platforms copy a selection with.
func selectedText(area *View, box driver.Bounds) string {
	origin := area.surfaceFrame().Origin
	var texts areaTexts
	var walk func(*View)
	walk = func(n *View) {
		if n.Kind == driver.KindText {
			f := n.surfaceFrame()
			f.Origin.X -= origin.X
			f.Origin.Y -= origin.Y
			if n.Props.Selectable != driver.SelectableOff && overlaps(f, box) {
				texts = append(texts, areaText{n, f})
			}
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(area)
	var m driver.SelectionModel
	m.SelectAll(texts)
	return m.Text(texts)
}

// overlaps reports whether a and b share some area.
func overlaps(a, b driver.Bounds) bool {
	return a.Origin.X < b.Origin.X+b.Size.Width && b.Origin.X < a.Origin.X+a.Size.Width &&
		a.Origin.Y < b.Origin.Y+b.Size.Height && b.Origin.Y < a.Origin.Y+a.Size.Height
}

// areaText is a text of a selection area and its frame in the area.
type areaText struct {
	v     *View
	frame driver.Bounds
}

// areaTexts are an area's texts as driver.SelectionModel reads them.
type areaTexts []areaText

func (t areaTexts) Len() int                           { return len(t) }
func (t areaTexts) Content(i int) string               { return t[i].v.Text }
func (t areaTexts) Frame(i int) driver.Bounds          { return t[i].frame }
func (t areaTexts) OffsetAt(i int, _ driver.Point) int { return 0 }
func (t areaTexts) Key(i int) uint64                   { return uint64(t[i].v.ID) }
func (t areaTexts) Spans(i int) []driver.TextSpan      { return t[i].v.Spans }

// HoverOut moves the mouse out of the surface: every view hovered hears
// MouseLeave.
func (a *App) HoverOut() {
	a.t.Helper()
	a.step("hoverOut")
	if a.target != nil {
		a.target.hover(driver.Point{X: -1, Y: -1})
		a.done()
		return
	}
	a.mouseIn = false
	a.setHovered(nil, driver.Point{})
	a.done()
}

// ActiveCursor describes the resolved pointer cursor over a view or at the
// current mouse position (ActiveCursor, CursorAt).
type ActiveCursor struct {
	// Cursor is the active named cursor (or the effective fallback when Custom
	// has empty images).
	Cursor driver.Cursor
	// Custom is the active CustomCursor specification, or nil when a named
	// cursor is shown.
	Custom *driver.CustomCursor
	// Source is the ImageSource selected for the surface's pixel ratio
	// (Image2x when PixelRatio >= 1.5 and Image2x is set, else Image).
	Source driver.ImageSource
	// Is2x reports whether Source is Custom.Image2x.
	Is2x bool
	// Hotspot is Custom.Hotspot clamped to the image's logical bounds (or
	// clamped to >= 0 when the image dimensions are unknown).
	Hotspot driver.Point
	// Fallback is the effective fallback named cursor.
	Fallback driver.Cursor
}

// ActiveCursor returns the resolved cursor at the current mouse position (after
// Hover or HoverAt), walking up from the view under the pointer to the nearest
// ancestor that names a cursor or custom cursor image. Returns CursorAuto when
// the pointer is outside the surface or no view under it names a cursor.
func (a *App) ActiveCursor() ActiveCursor {
	if !a.mouseIn {
		return ActiveCursor{}
	}
	v := a.viewAt(a.mouse, func(*View) bool { return true })
	return a.resolveCursor(v)
}

// CursorAt returns the resolved cursor over the view selector matches, walking
// up to its nearest ancestor that names a cursor or custom cursor image.
func (a *App) CursorAt(selector string) ActiveCursor {
	a.t.Helper()
	v := a.pick(selector, "view", func(m *View) (*View, bool) { return m, true })
	return a.resolveCursor(v)
}

func (a *App) resolveCursor(v *View) ActiveCursor {
	scale := float32(1)
	if a.env != nil && a.env.PixelRatio > 0 {
		scale = a.env.PixelRatio
	}
	for p := v; p != nil; p = p.Parent {
		if cc := p.Style.CursorImage; cc != nil {
			src, is2x := cc.SelectSource(scale)
			hot := driver.ClampHotspot(cc.Hotspot, cc.LogicalSize())
			fb := cc.EffectiveFallback(p.Style.Cursor)
			if src.Empty() {
				return ActiveCursor{Cursor: fb, Custom: cc, Hotspot: hot, Fallback: fb}
			}
			return ActiveCursor{
				Cursor:   p.Style.Cursor,
				Custom:   cc,
				Source:   src,
				Is2x:     is2x,
				Hotspot:  hot,
				Fallback: fb,
			}
		}
		if p.Style.Cursor != driver.CursorAuto {
			return ActiveCursor{Cursor: p.Style.Cursor, Fallback: p.Style.Cursor}
		}
	}
	return ActiveCursor{}
}

// setHovered makes next, innermost first, the views hovered: those that
// leave hear MouseLeave, innermost first, and those that come MouseEnter,
// outermost first.
func (a *App) setHovered(next []*View, at driver.Point) {
	for _, h := range a.hovered {
		if !containsView(next, h) && a.views[h.ID] == h {
			h.Hovered = false
			ev := a.pressEvent(h, a.mouse)
			ev.Kind, ev.Count = driver.EventMouseLeave, 0
			a.send(h, ev)
		}
	}
	for i := len(next) - 1; i >= 0; i-- {
		if h := next[i]; !containsView(a.hovered, h) {
			h.Hovered = true
			ev := a.pressEvent(h, at)
			ev.Kind, ev.Count = driver.EventMouseEnter, 0
			a.send(h, ev)
		}
	}
	a.hovered, a.mouse = next, at
}

// Scroll drags the scroll view selector names (the match, the nearest
// scroll view around it, else the first in it) by distance points along
// its axis, 10 points a display frame within its content, as a finger
// does: ScrollDragStart, the offsets as platforms report them, then
// ScrollDragEnd. It fails when the view cannot move that way at all or
// the user cannot scroll it.
func (a *App) Scroll(selector string, distance driver.Points) {
	a.t.Helper()
	a.step("scroll(%s, %v)", selector, distance)
	isScroll := func(v *View) bool { return v.Kind == driver.KindScroll }
	v := a.pick(selector, "scroll view", func(m *View) (*View, bool) {
		t := m.upTo(isScroll)
		if t == nil {
			t = m.downTo(isScroll)
		}
		return t, t != nil && !t.Scroll.Behavior.NoScroll
	})
	at := func() driver.Points {
		if v.Scroll.Horizontal {
			return v.Offset.X
		}
		return v.Offset.Y
	}
	start := at()
	to := min(max(start+distance, 0), v.scrollEnd())
	if math.Abs(float64(to-start)) < 0.5 {
		a.t.Fatalf("nsgotest: %q cannot scroll by %v: it is at %v of %v", selector, distance, start, v.scrollEnd())
	}
	if a.target != nil {
		a.target.scroll(v, to-start)
		a.done()
		return
	}
	a.scrollEvent(v, driver.EventScrollDragStart)
	f := &finger{}
	// The app may shift the offset meanwhile (ScrollBy): each frame moves
	// on from where the view is.
	for frames := int(math.Abs(float64(to-start))/dragStep)*2 + 10; frames > 0; frames-- {
		now := at()
		if math.Abs(float64(to-now)) < 0.5 {
			break
		}
		next := min(now+dragStep, to)
		if to < now {
			next = max(now-dragStep, to)
		}
		off := v.Offset
		if v.Scroll.Horizontal {
			off.X = next
		} else {
			off.Y = next
		}
		a.scrollView(v, off)
		a.frame(f)
	}
	a.scrollEvent(v, driver.EventScrollDragEnd)
	a.settle(v)
	a.done()
}

// ScrollTo reports that the scroll view selector matches is at offset, as
// a platform reports where it is; it moves no finger.
func (a *App) ScrollTo(selector string, offset driver.Point) {
	a.t.Helper()
	v := a.Find(selector)
	a.scrollView(v, offset)
}

func (g *Finger) pagePoint(at driver.Point) driver.Point {
	x, y := g.matrix.Apply(float64(at.X), float64(at.Y))
	return driver.Point{X: driver.Points(x), Y: driver.Points(y)}
}
