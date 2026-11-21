//go:build linux && !android

package linux

import (
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Keyvals GTK uses for named keys.
const (
	gdkEscape    = 0xff1b
	gdkReturn    = 0xff0d
	gdkKPEnter   = 0xff8d
	gdkBackspace = 0xff08
	gdkTab       = 0xff09
	gdkDelete    = 0xffff
	gdkInsert    = 0xff63
	gdkHome      = 0xff50
	gdkEnd       = 0xff57
	gdkPageUp    = 0xff55
	gdkPageDown  = 0xff56
	gdkLeft      = 0xff51
	gdkUp        = 0xff52
	gdkRight     = 0xff53
	gdkDown      = 0xff54
	gdkSpace     = 0x020
	gdkF1        = 0xffbe
	gdkF35       = 0xffe0
	// gdkShiftL to gdkHyperR are the modifier keys, Caps Lock among them.
	gdkShiftL = 0xffe1
	gdkHyperR = 0xffee
	gdkKP0    = 0xffb0
	gdkKP9    = 0xffb9
)

// GDK modifier bits.
const (
	gdkShift   = 1 << 0
	gdkControl = 1 << 2
	gdkAlt     = 1 << 3
	gdkSuper   = 1 << 26
	gdkMeta    = 1 << 28
)

// modifiers returns the modifier keys among GDK's state bits.
func modifiers(state uint32) driver.Modifiers {
	return driver.Modifiers{Control: state&gdkControl != 0, Alt: state&gdkAlt != 0, Shift: state&gdkShift != 0,
		Platform: state&(gdkSuper|gdkMeta) != 0}
}

const (
	propagationPhaseCapture = 1
	propagationPhaseBubble  = 2
)

// pan is a drag in progress on a view that follows pans.
type pan struct {
	v          *view
	start      driver.Point
	active     bool
	failed     bool
	last       driver.Point
	lastPage   driver.Point
	lastTime   time.Time
	velocity   driver.Point
	activation driver.PanActivation
}

// twoFinger is a pinch or a rotation in progress.
type twoFinger struct {
	v        *view
	kind     driver.EventKind
	value    float32
	velocity float32
	lastTime time.Time
}

// Callbacks handed to GTK. Each finds its window or view through the
// pointer GTK was connected with.

func clickPressedCallback(gesture uintptr, n int32, x, y float64, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing {
		return
	}
	w.hideMenuBar()
	w.currentEvent = gtkEventControllerGetCurrentEvent(gesture)
	defer func() { w.currentEvent = 0 }()
	// Presses on a field, a page or a video's controls are theirs, even
	// where they bubble up here.
	if w.nativeAt(gtkWidgetPick(w.surface, x, y, gtkPickDefault)) {
		w.pressConsumed = true
		return
	}
	if w.embedded && w.focusedView() == nil {
		// A host's widget takes the keys once pressed. A focused control in it already has them, and grabbing the surface would blur it.
		gtkWidgetGrabFocus(w.surface)
	}
	w.pointerDown(driver.Point{X: driver.Points(x), Y: driver.Points(y)},
		gtkGestureSingleGetCurrentButton(gesture), gtkGestureGetDevice(gesture),
		gtkEventControllerGetCurrentEventState(gesture), int(n))
}

// onEntry reports whether surface point at is on the field v, the topmost field there.
func (w *window) onEntry(v *view, at driver.Point) bool {
	root := w.view(w.root)
	return root != nil && hit(root, at, func(v *view) bool { return v.kind == driver.KindInput }) == v
}

// pointerDown follows a press of button at surface point at, the clicks'
// press in a row, with modifiers state.
func (w *window) pointerDown(at driver.Point, button uint32, device uintptr, state uint32, clicks int) {
	// Test input enters here without GTK's legacy controller. A press after a key must still report pointer input and hide focus rings.
	w.engine.InputModality(driver.ModalityPointer)
	w.lastPointer, w.pressConsumed, w.windowMove = at, false, nil
	if button == 1 && !isTouch(device) {
		root := w.view(w.root)
		var target *view
		if root != nil {
			target = hit(root, at, func(v *view) bool {
				return v.kind == driver.KindInput || v.events.Has(driver.EventPress) || v.events.Has(driver.EventLongPress) || v.events.Has(driver.EventPressDown) || v.events.Has(driver.EventPressUp)
			})
		}
		if clicks > 1 && target != w.lastClick {
			clicks = 1
		}
		w.lastClick = target
	}
	w.down = driver.Event{Modifiers: modifiers(state), Count: uint32(max(clicks, 1)), Page: at}
	// On X11 the released GTK 4 gives a touch's events the core pointer as their device, whose source is the mouse, so a touch is also known by its event's type.
	touch := button == 1 && (isTouch(device) || w.currentEvent != 0 && gdkEventGetEventType(w.currentEvent) == gdkTouchBegin)
	if touch {
		w.touchDown(at)
	} else {
		w.touching = nil
	}
	if button == 1 && !touch {
		// A touchscreen's fingers are reported by their sequences (touch.go).
		w.touch(w.fingers.Down(w.listensTouches(), at))
	}
	// A press on the surface leaves the focused field or page, but not a press on the focused field itself, which the test host's presses bring here after giving it the focus. A focused control keeps the focus, as on the other desktops: a press does not move it, and a handler that wants it elsewhere focuses that view itself.
	if f := w.focusedEntry(); f != nil && !w.onEntry(f, at) || w.focusedWeb() != nil {
		w.focusEntry(nil)
	}
	w.pressOutside(at)
	if button == 1 && w.pressBar(at) {
		w.pressConsumed = true
		w.panning = nil
		return
	}
	if button == 3 {
		w.contextMenu(at)
		w.pressConsumed = true
		return
	}
	if button == 1 {
		w.pressEdge(driver.EventPressDown, at)
	}
	if button == 1 && !touch && w.selectPress(at, w.down.Modifiers, clicks) {
		w.pressConsumed = true
		w.panning = nil
		return
	}
	if w.beginWindowDrag(button, device, at) {
		w.pressConsumed = true
	}
	w.panDown(at)
	if button == 1 && !w.pressConsumed {
		w.startLongPress(at)
	}
}

// pressEdge reports kind, a press going down or up at at, to the innermost
// view there that takes presses, if it hears kind.
func (w *window) pressEdge(kind driver.EventKind, at driver.Point) {
	root := w.view(w.root)
	if root == nil {
		return
	}
	v := hit(root, at, func(v *view) bool {
		m := v.events
		return m.Has(driver.EventPress) || m.Has(driver.EventLongPress) || m.Has(driver.EventPressDown) || m.Has(driver.EventPressUp)
	})
	if v == nil || !v.events.Has(kind) {
		return
	}
	ev := w.down
	ev.Kind, ev.ID, ev.Page = kind, v.id, at
	if _, ok := w.surfaceBounds(v); ok {
		ev.Point = v.surfacePoint(at)
	}
	w.engine.Dispatch(ev)
}

func clickReleasedCallback(gesture uintptr, n int32, x, y float64, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing {
		return
	}
	w.pointerUp(driver.Point{X: driver.Points(x), Y: driver.Points(y)})
}

// pointerUp follows the release of a press at surface point at.
func (w *window) pointerUp(at driver.Point) {
	w.windowMove = nil
	w.selectUp()
	w.touch(w.fingers.Up(at))
	w.cancelLongPress()
	w.pressEdge(driver.EventPressUp, at)
	if w.pressConsumed || w.panning != nil && w.panning.active {
		return
	}
	w.press(at)
}

func motionCallback(controller uintptr, x, y float64, data uintptr) {
	w := windowOf(data)
	if w == nil {
		return
	}
	var state uint32
	if controller != 0 { // tests move the pointer without one
		state = gtkEventControllerGetCurrentEventState(controller)
	}
	w.movePointer(driver.Point{X: driver.Points(x), Y: driver.Points(y)}, state)
	if w.barDrag == nil {
		w.hoverBars(w.lastPointer)
	}
}

func leaveCallback(controller uintptr, data uintptr) {
	w := windowOf(data)
	if w == nil {
		return
	}
	w.pointerIn = false
	if w.cursorText != nil {
		w.setCursor(w.cursorText)
		w.cursorText = nil
	}
	w.hoverViews()
	if w.barDrag == nil {
		w.hoverBars(driver.Point{X: -1, Y: -1})
	}
}

func dragBeginCallback(gesture uintptr, offsetX, offsetY float64, data uintptr) {
	w := windowOf(data)
	if w == nil || w.panning == nil || w.panning.failed {
		return
	}
	w.panning.lastTime = time.Now()
}

func dragUpdateCallback(gesture uintptr, offsetX, offsetY float64, data uintptr) {
	w := windowOf(data)
	if w != nil && w.windowMove != nil {
		if math.Hypot(offsetX, offsetY) > windowMoveSlop {
			w.moveWindow()
		}
		return
	}
	if w != nil && math.Hypot(offsetX, offsetY) > longPressSlop {
		w.cancelLongPress()
	}
	if w != nil && w.selectDrag(driver.Point{X: w.down.Page.X + driver.Points(offsetX), Y: w.down.Page.Y + driver.Points(offsetY)}) {
		return
	}
	if w != nil && w.touchMove(driver.Point{X: w.down.Page.X + driver.Points(offsetX), Y: w.down.Page.Y + driver.Points(offsetY)}) {
		return
	}
	if w != nil && w.barDrag != nil {
		d := w.barDrag
		w.dragBar(driver.Point{X: d.from.X + driver.Points(offsetX), Y: d.from.Y + driver.Points(offsetY)})
		return
	}
	if w == nil || w.panning == nil || w.panning.failed {
		return
	}
	g := w.panning
	if g.v.deleted {
		w.panning = nil
		return
	}
	from := g.start
	now := driver.Point{X: from.X + driver.Points(offsetX), Y: from.Y + driver.Points(offsetY)}
	d := driver.Point{X: now.X - from.X, Y: now.Y - from.Y}
	if !g.active {
		if fails(g.activation, d) {
			g.failed = true
			w.panning = nil
			return
		}
		if !activates(g.activation, d) {
			return
		}
		g.active = true
		g.lastPage = now
		w.reportGesture(driver.Event{Kind: driver.EventPan, ID: g.v.id, Phase: driver.GestureBegan, Point: d, Position: g.v.local(now), PagePosition: now})
	}
	dt := time.Since(g.lastTime).Seconds()
	var velocity driver.Point
	if dt > 0 {
		velocity = driver.Point{X: driver.Points(float64(now.X-g.last.X) / dt), Y: driver.Points(float64(now.Y-g.last.Y) / dt)}
	}
	g.last, g.lastTime, g.velocity = now, time.Now(), velocity
	g.lastPage = now
	w.reportGesture(driver.Event{Kind: driver.EventPan, ID: g.v.id, Phase: driver.GestureChanged, Point: d, Position: g.v.local(now), PagePosition: now, Velocity: velocity})
}

func dragEndCallback(gesture uintptr, offsetX, offsetY float64, data uintptr) {
	w := windowOf(data)
	// GTK's click gesture may not report a release that moved, which ends a press that never moved the window too.
	if w != nil {
		w.windowMove = nil
	}
	if w != nil && w.touchUp() {
		return
	}
	if w != nil && w.barDrag != nil {
		w.releaseBar()
		w.hoverBars(w.lastPointer)
		gtkWidgetQueueDraw(w.surface)
		return
	}
	if w == nil || w.panning == nil {
		return
	}
	g := w.panning
	w.panning = nil
	if g.failed || g.v.deleted {
		return
	}
	d := driver.Point{X: driver.Points(offsetX), Y: driver.Points(offsetY)}
	if !g.active && !activates(g.activation, d) {
		return
	}
	if !g.active {
		g.active = true
		now := driver.Point{X: g.start.X + d.X, Y: g.start.Y + d.Y}
		w.reportGesture(driver.Event{Kind: driver.EventPan, ID: g.v.id, Phase: driver.GestureBegan, Point: d, Position: g.v.local(now), PagePosition: now})
	}
	now := driver.Point{X: g.start.X + d.X, Y: g.start.Y + d.Y}
	w.reportGesture(driver.Event{Kind: driver.EventPan, ID: g.v.id, Phase: driver.GestureEnded, Point: d, Position: g.v.local(now), PagePosition: now, Velocity: g.velocity})
}

// longPressDelay is how long a press is held to be a long press, unless
// the view's DelayLongPress says else.
const longPressDelay = 500 * time.Millisecond

// longPressSlop is how far a press may move and still become a long
// press, in points, as GTK's drag threshold.
const longPressSlop = 8

// startLongPress waits for the press at surface point at to be held as
// long as the view there that takes long presses asks. A timer of NSGo's
// own rather than GTK's long press gesture, whose delay is not the view's.
func (w *window) startLongPress(at driver.Point) {
	w.longPress++
	v := w.hitEvent(at, driver.EventLongPress)
	if v == nil {
		return
	}
	delay := longPressDelay
	if d := v.props.DelayLongPress; d > 0 {
		delay = d
	}
	seq, ev := w.longPress, w.down
	ev.Kind, ev.ID, ev.Page = driver.EventLongPress, v.id, at
	if _, ok := w.surfaceBounds(v); ok {
		ev.Point = v.surfacePoint(at)
		// A long press on a text's span is the span's.
		if l, ok := v.text.(*textLayout); ok && v.kind == driver.KindText {
			ev.Index = l.indexAt(ev.Point.X-v.content.Origin.X, ev.Point.Y-v.content.Origin.Y)
		}
	}
	time.AfterFunc(delay, func() {
		w.post(func() {
			if w.longPress != seq || w.closing || v.deleted {
				return
			}
			w.longPress++
			w.pressConsumed = true // the release presses nothing
			w.engine.Dispatch(ev)
		})
	})
}

// cancelLongPress stops waiting for the press in progress to be long.
func (w *window) cancelLongPress() { w.longPress++ }

func zoomChangedCallback(gesture uintptr, scale float64, data uintptr) {
	w := windowOf(data)
	if w == nil {
		return
	}
	w.reportTwoFinger(driver.EventPinch, float32(scale))
}

func rotateChangedCallback(gesture uintptr, angle, delta float64, data uintptr) {
	w := windowOf(data)
	if w == nil {
		return
	}
	w.reportTwoFinger(driver.EventRotation, float32(delta*math.Pi/180))
}

func gestureEndCallback(gesture uintptr, sequence uintptr, data uintptr) {
	w := windowOf(data)
	if w == nil {
		return
	}
	for _, g := range []*twoFinger{w.pinching, w.rotating} {
		if g != nil && g.v != nil && !g.v.deleted {
			w.reportGesture(driver.Event{Kind: g.kind, ID: g.v.id, Phase: driver.GestureEnded, Value: g.value,
				Velocity: driver.Point{X: driver.Points(g.velocity)}})
		}
	}
	w.pinching, w.rotating = nil, nil
}

func scrollCallback(controller uintptr, dx, dy float64, data uintptr) int32 {
	w := windowOf(data)
	if w == nil {
		return 0
	}
	at := w.lastPointer
	if w.controlAt(gtkWidgetPick(w.surface, float64(at.X), float64(at.Y), gtkPickDefault)) != nil {
		return 0 // the page or the controls scroll themselves
	}
	w.wheelEvent(gtkEventControllerGetCurrentEventState(controller), at, dx, dy, controllerPoints(controller))
	return 1
}

// scrollBeginCallback follows fingers landing on the touchpad, which stop
// its glide as a finger on a moving list does.
func scrollBeginCallback(_, data uintptr) {
	if w := windowOf(data); w != nil {
		w.stopWheelGlide()
		w.wheelTaker = nil // the scroll finds its view anew
	}
}

// scrollDecelerateCallback follows a touchpad's fingers lifting at a
// velocity, which GTK gives in points per second, though it documents
// milliseconds.
func scrollDecelerateCallback(controller uintptr, vx, vy float64, data uintptr) {
	w := windowOf(data)
	if w == nil || !controllerPoints(controller) {
		return
	}
	state := gtkEventControllerGetCurrentEventState(controller)
	if state&gdkControl != 0 {
		return
	}
	if state&gdkShift != 0 {
		vx, vy = vy, 0
	}
	if v := w.wheelTaker; v != nil {
		w.wheelTaker = nil
		if !v.deleted {
			// GTK leaves the glide to the view, which carries the content
			// on at the fingers' velocity.
			ev := w.wheelEventAt(v, w.lastPointer, state)
			ev.Phase, ev.Precise, ev.Velocity = driver.GestureEnded, true, driver.Point{X: driver.Points(-vx), Y: driver.Points(-vy)}
			w.engine.Dispatch(ev)
		}
		return
	}
	w.wheelGlide(float32(vx/1000), float32(vy/1000))
}

// wheelTaken sends a wheel turn or touchpad scroll of (dx, dy), in
// points, at p to the innermost view with OnWheel under it, unless a
// scroll view inside it is there, and reports whether one took it. A
// touchpad's scroll stays on the view it began on until its fingers lift.
func (w *window) wheelTaken(state uint32, p driver.Point, dx, dy float64, touchpad bool) bool {
	v := w.wheelTaker
	if v == nil || v.deleted || !touchpad {
		w.wheelTaker = nil
		root := w.view(w.root)
		if root == nil {
			return false
		}
		under, _ := viewsAt(root, p, func(v *view) bool { return v.kind == driver.KindScroll || v.events.Has(driver.EventWheel) }, nil)
		if len(under) == 0 || under[len(under)-1].kind == driver.KindScroll {
			return false
		}
		v = under[len(under)-1]
		if touchpad {
			w.wheelTaker = v
		}
	}
	ev := w.wheelEventAt(v, p, state)
	ev.Phase, ev.Precise, ev.Delta = driver.GestureChanged, touchpad, driver.Point{X: driver.Points(-dx), Y: driver.Points(-dy)}
	w.engine.Dispatch(ev)
	return true
}

// wheelEventAt is a wheel event at surface point p over v.
func (w *window) wheelEventAt(v *view, p driver.Point, state uint32) driver.Event {
	return driver.Event{Kind: driver.EventWheel, ID: v.id, Point: v.surfacePoint(p), Page: p,
		Modifiers: modifiers(state)}
}

// controllerPoints reports whether a scroll controller's events come in
// points, as a touchpad's do. GTK says so from 4.8.
func controllerPoints(controller uintptr) bool {
	return gtkEventControllerScrollGetUnit != nil && gtkEventControllerScrollGetUnit(controller) == gdkScrollUnitSurface
}

// wheelEvent follows a wheel turn with modifiers state at surface point
// at: a pinch with Control held, else a scroll. inPoints is set when GTK
// says the deltas are points, as a touchpad's are.
func (w *window) wheelEvent(state uint32, at driver.Point, dx, dy float64, inPoints bool) {
	if state&gdkControl != 0 {
		w.wheelPinch(at, dy)
		return
	}
	if state&gdkShift != 0 {
		dx, dy = dy, 0
	}
	// GTK's wheel turns come as whole units; a touchpad's as points, which
	// only their size tells before 4.8.
	units := !inPoints && math.Abs(dx) <= 3 && math.Abs(dy) <= 3
	// A view taking the wheel moves a notch three lines of 33 points, as
	// browsers scroll about 100.
	if units && w.wheelTaken(state, at, dx*99, dy*99, false) || !units && w.wheelTaken(state, at, dx, dy, true) {
		return
	}
	notch := 48.0
	if units {
		dx, dy = dx*notch, dy*notch
	}
	w.wheel(at, dx, dy)
}

func keyPressedCallback(controller uintptr, keyval, keycode, state uint32, data uintptr) int32 {
	w := windowOf(data)
	if w == nil {
		return 0
	}
	// GTK reports no auto-repeats: a key pressed again before it was released repeats.
	_, repeat := w.held[keycode]
	if w.held == nil {
		w.held = map[uint32]heldKey{}
	}
	if !repeat {
		w.held[keycode] = heldKey{keyval: keyval}
	}
	if keyval < gdkShiftL || keyval > gdkHyperR {
		w.engine.InputModality(driver.ModalityKeyboard)
	}
	if w.keyDown(keycode, keyval, state, repeat) {
		// Alt released after a key it took does not toggle the menu bar.
		w.chrome.altAlone = false
		return 1
	}
	w.menuBarKey(keyval, state, true)
	if w.selectKey(keyval, state) {
		return 1
	}
	if w.shortcut(keyval, state, repeat) || w.controlKey(keyval, state, repeat) {
		return 1
	}
	if (keyval == 0xff09 || keyval == 0xfe20) && state&(gdkControl|gdkAlt|gdkSuper|gdkMeta) == 0 && w.focusedWeb() == nil && w.tab(state&gdkShift != 0 || keyval == 0xfe20) {
		return 1
	}
	if state&(gdkControl|gdkAlt|gdkSuper|gdkMeta) == 0 && (w.focusedWeb() != nil || w.focus != nil && keyval != gdkEscape) {
		return 0 // the page's keys, Escape among them, and the field's
	}
	if keyval == gdkEscape {
		if w.focus != nil {
			w.focusEntry(nil)
		}
		w.back()
		return 1
	}
	return 0
}

// setupInputs adds GTK's event controllers to the surface and the
// window. Events on views bubble up to the surface, which hit tests them
// itself, as NSH's input does: GTK's picking knows nothing of rounded
// clips or hidden views.
func (w *window) setupInputs() {
	shield := gtkEventControllerLegacyNew()
	gtkEventControllerSetPropagationPhase(shield, propagationPhaseCapture)
	gSignalConnect(shield, "event", cbShield, w.handle)
	gtkWidgetAddController(w.surface, shield)

	touches := gtkEventControllerLegacyNew()
	gtkEventControllerSetPropagationPhase(touches, propagationPhaseCapture)
	gSignalConnect(touches, "event", cbTouch, w.handle)
	gtkWidgetAddController(w.surface, touches)

	click := gtkGestureClickNew()
	gtkGestureSingleSetButton(click, 0)
	gSignalConnect(click, "pressed", cbClickPressed, w.handle)
	gSignalConnect(click, "released", cbClickReleased, w.handle)
	gtkWidgetAddController(w.surface, click)

	motion := gtkEventControllerMotionNew()
	gSignalConnect(motion, "motion", cbMotion, w.handle)
	gSignalConnect(motion, "leave", cbLeave, w.handle)
	gtkWidgetAddController(w.surface, motion)

	drag := gtkGestureDragNew()
	gtkGestureSingleSetButton(drag, 1)
	gSignalConnect(drag, "drag-begin", cbDragBegin, w.handle)
	gSignalConnect(drag, "drag-update", cbDragUpdate, w.handle)
	gSignalConnect(drag, "drag-end", cbDragEnd, w.handle)
	gtkWidgetAddController(w.surface, drag)

	zoom := gtkGestureZoomNew()
	gSignalConnect(zoom, "scale-changed", cbZoomChanged, w.handle)
	gSignalConnect(zoom, "end", cbGestureEnd, w.handle)
	gtkWidgetAddController(w.surface, zoom)

	rotate := gtkGestureRotateNew()
	gSignalConnect(rotate, "angle-changed", cbRotateChanged, w.handle)
	gSignalConnect(rotate, "end", cbGestureEnd, w.handle)
	gtkWidgetAddController(w.surface, rotate)

	scroll := gtkEventControllerScrollNew(scrollBothAxes | scrollKinetic)
	gSignalConnect(scroll, "scroll", cbScroll, w.handle)
	gSignalConnect(scroll, "scroll-begin", cbScrollBegin, w.handle)
	gSignalConnect(scroll, "decelerate", cbScrollDecelerate, w.handle)
	gtkWidgetAddController(w.surface, scroll)

	keys := gtkEventControllerKeyNew()
	gtkEventControllerSetPropagationPhase(keys, propagationPhaseCapture)
	gSignalConnect(keys, "key-pressed", cbKeyPressed, w.handle)
	gSignalConnect(keys, "key-released", cbKeyReleased, w.handle)
	gSignalConnect(keys, "modifiers", cbModifiers, w.handle)
	// A window of its own hears every key; a host's widget those pressed
	// while the focus is in it.
	if w.embedded {
		gtkWidgetAddController(w.surface, keys)
		// The keys held go up elsewhere once the focus leaves the surface for the host's widgets, which leaves the host's window active (activityCallback).
		focus := gtkEventControllerFocusNew()
		gSignalConnect(focus, "leave", cbKeysLeave, w.handle)
		gtkWidgetAddController(w.surface, focus)
	} else {
		gtkWidgetAddController(w.win, keys)
	}
}

// hitEvent returns the innermost view under p that reports kind.
func (w *window) hitEvent(p driver.Point, kind driver.EventKind) *view {
	root := w.view(w.root)
	if root == nil {
		return nil
	}
	return hit(root, p, func(v *view) bool { return v.events.Has(kind) })
}

// panDown starts following a drag at p if a view there follows pans.
func (w *window) panDown(p driver.Point) {
	w.panning = nil
	v := w.hitEvent(p, driver.EventPan)
	if v == nil {
		return
	}
	w.panning = &pan{v: v, start: p, last: p, lastTime: time.Now(), activation: v.props.PanActivation}
}

// reportGesture moves the animated values bound to a pan, pinch or
// rotation, so they are drawn in the frame that shows the move, and
// reports it.
func (w *window) reportGesture(ev driver.Event) {
	if w.animator.Gesture(&ev) {
		w.showAnimated()
	}
	w.engine.Dispatch(ev)
}

// activates reports whether a drag of d has moved far enough to start a
// pan.
func activates(a driver.PanActivation, d driver.Point) bool {
	if a.ActiveOffsetX == 0 && a.ActiveOffsetY == 0 {
		return math.Hypot(float64(d.X), float64(d.Y)) >= float64(driver.DefaultPanSlop)
	}
	return a.ActiveOffsetX != 0 && math.Abs(float64(d.X)) >= float64(a.ActiveOffsetX) ||
		a.ActiveOffsetY != 0 && math.Abs(float64(d.Y)) >= float64(a.ActiveOffsetY)
}

// fails reports whether a drag of d crossed a fail offset.
func fails(a driver.PanActivation, d driver.Point) bool {
	return a.FailOffsetX != 0 && math.Abs(float64(d.X)) > float64(a.FailOffsetX) ||
		a.FailOffsetY != 0 && math.Abs(float64(d.Y)) > float64(a.FailOffsetY)
}

// reportTwoFinger reports a touchscreen pinch or rotation under the
// pointer, beginning it on the first change.
func (w *window) reportTwoFinger(kind driver.EventKind, value float32) {
	slot := &w.pinching
	if kind == driver.EventRotation {
		slot = &w.rotating
	}
	if *slot == nil {
		v := w.hitEvent(w.lastPointer, kind)
		if v == nil {
			return
		}
		*slot = &twoFinger{v: v, kind: kind, value: rest(kind), lastTime: time.Now()}
		w.reportGesture(driver.Event{Kind: kind, ID: v.id, Phase: driver.GestureBegan, Value: rest(kind)})
	}
	g := *slot
	if g.v.deleted {
		*slot = nil
		return
	}
	if kind == driver.EventRotation {
		value = g.value + value
	}
	now := time.Now()
	dt := now.Sub(g.lastTime).Seconds()
	var velocity float32
	if dt > 0 {
		velocity = (value - g.value) / float32(dt)
	}
	g.value, g.lastTime, g.velocity = value, now, velocity
	w.reportGesture(driver.Event{Kind: kind, ID: g.v.id, Phase: driver.GestureChanged, Value: value,
		Velocity: driver.Point{X: driver.Points(velocity)}, Point: w.pinchPoint(g.v, kind, w.lastPointer)})
}

// pinchPoint is where a pinch at surface point p is about in v; other
// gestures have none.
func (w *window) pinchPoint(v *view, kind driver.EventKind, p driver.Point) driver.Point {
	if kind != driver.EventPinch {
		return driver.Point{}
	}
	return v.surfacePoint(p)
}

// rest is a gesture's value when it begins.
func rest(kind driver.EventKind) float32 {
	if kind == driver.EventPinch {
		return 1
	}
	return 0
}

const (
	pinchTimer = 1
	// pinchIdleMillis ends a touchpad pinch once its wheel turns stop.
	pinchIdleMillis = 150
)

// wheelPinch follows a touchpad's pinch, which comes as wheel turns with
// Control held, at p.
func (w *window) wheelPinch(p driver.Point, delta float64) {
	g := w.pinching
	if g != nil && g.v.deleted {
		g = nil
	}
	if g == nil {
		v := w.hitEvent(p, driver.EventPinch)
		if v == nil {
			return
		}
		g = &twoFinger{v: v, kind: driver.EventPinch, value: 1, lastTime: time.Now()}
		w.pinching = g
		w.reportGesture(driver.Event{Kind: driver.EventPinch, ID: v.id, Phase: driver.GestureBegan, Value: 1})
	}
	g.value *= float32(math.Pow(2, -delta/20))
	w.reportGesture(driver.Event{Kind: driver.EventPinch, ID: g.v.id, Phase: driver.GestureChanged, Value: g.value,
		Point: w.pinchPoint(g.v, driver.EventPinch, p)})
	if w.pinchTimeout != 0 {
		gSourceRemove(w.pinchTimeout)
	}
	w.pinchTimeout = gTimeoutAdd(pinchIdleMillis, cbPinchTimeout, w.handle)
}

func pinchTimeoutCallback(data uintptr) int32 {
	w := windowOf(data)
	if w == nil {
		return 0
	}
	w.pinchTimeout = 0
	if g := w.pinching; g != nil {
		w.pinching = nil
		if !g.v.deleted {
			w.reportGesture(driver.Event{Kind: driver.EventPinch, ID: g.v.id, Phase: driver.GestureEnded, Value: g.value})
		}
	}
	return 0 // G_SOURCE_REMOVE
}

// windowMove is a press of the first button on a window drag region, at surface point at, that moves the window once the pointer travels past windowMoveSlop, as GTK's own GtkWindowHandle does. A move started as the button goes down takes the pointer from the window on Wayland, so the next press would start a new row of clicks and a double click could never be told.
type windowMove struct {
	device uintptr
	at     driver.Point
}

// windowMoveSlop is how far a press on a window drag region travels before it moves the window, in points, as GTK's drag threshold.
const windowMoveSlop = 8

// beginWindowDrag takes a press of button that is in a window drag region and not on a nearer control: the second press of a double click does what the desktop's title bars do, and another press of the first button moves the window once it is dragged.
func (w *window) beginWindowDrag(button uint32, device uintptr, at driver.Point) bool {
	root := w.view(w.root)
	if root == nil || button != 1 {
		return false
	}
	drag := hit(root, at, func(v *view) bool { return v.props.WindowDrag != driver.WindowDragInherit })
	if drag == nil || drag.props.WindowDrag != driver.WindowDragOn {
		return false
	}
	if control := hit(root, at, func(v *view) bool {
		return v.events.Has(driver.EventPress) || v.events.Has(driver.EventPan) || v.kind == driver.KindInput || v.kind == driver.KindWeb
	}); control != nil && descendant(control, drag) {
		return false
	}
	// A transparent window has no title bar whose double click maximizes it, as a desktop pet's must not, nor does a panel or a view a host embeds: they drag as for one click.
	if w.down.Count == 2 && w.ownWindow() && !w.transparent {
		w.titleBarDoubleClick()
		return true
	}
	if device == 0 || gtkNativeGetSurface(gtkWidgetGetNative(w.surface)) == 0 {
		return false
	}
	w.windowMove = &windowMove{device: device, at: at}
	return true
}

// moveWindow hands the pointer to the window manager, which moves the window from where the press went down.
func (w *window) moveWindow() {
	m := w.windowMove
	w.windowMove = nil
	if surface := gtkNativeGetSurface(gtkWidgetGetNative(w.surface)); surface != 0 {
		gdkToplevelBeginMove(surface, m.device, 1, float64(m.at.X), float64(m.at.Y), 0)
	}
}

// titleBarAction is what a double click on a title bar does.
type titleBarAction int

const (
	titleBarNone titleBarAction = iota
	titleBarToggleMaximize
	titleBarMinimize
	titleBarLower
	titleBarShowMenu
)

// titleBarDoubleClickAction reads GtkSettings' gtk-titlebar-double-click, the desktop's choice that GTK's own header bars follow (GNOME's action-double-click-titlebar), as GTK 4's GtkWindowHandle does: the maximize variants all toggle maximize, as does an unset or unknown value. GTK's "menu" shows the window manager's menu for the press's GdkEvent.
func titleBarDoubleClickAction(setting string) titleBarAction {
	switch setting {
	case "none":
		return titleBarNone
	case "menu":
		return titleBarShowMenu
	case "minimize":
		return titleBarMinimize
	case "lower":
		return titleBarLower
	}
	return titleBarToggleMaximize
}

// titleBarDoubleClick carries out the desktop's action for a double click on the window's title bar, within the window's constraints.
func (w *window) titleBarDoubleClick() {
	var setting string
	if settings := gtkSettingsGetDefault(); settings != 0 {
		setting = settingString(settings, "gtk-titlebar-double-click")
	}
	switch titleBarDoubleClickAction(setting) {
	case titleBarToggleMaximize:
		w.toggleMaximize()
	case titleBarMinimize:
		w.minimize()
	case titleBarLower:
		if surface := gtkNativeGetSurface(w.win); surface != 0 {
			gdkToplevelLower(surface)
		}
	case titleBarShowMenu:
		if surface := gtkNativeGetSurface(w.win); surface != 0 && w.currentEvent != 0 {
			gdkToplevelShowWindowMenu(surface, w.currentEvent)
		}
	}
}

// descendant reports whether v is child or below.
func descendant(v, child *view) bool {
	for p := child; p != nil; p = p.parent {
		if p == v {
			return true
		}
	}
	return false
}

// modifierKeyvals name the modifier keys' keyvals as KeyEvent does, with the state bit each sets: GDK's state at a key press holds the modifiers held before it, not the one going down.
var modifierKeyvals = map[uint32]struct {
	name string
	bit  uint32
}{
	0xffe1: {"shift", gdkShift}, 0xffe2: {"shift", gdkShift}, 0xffe3: {"ctrl", gdkControl}, 0xffe4: {"ctrl", gdkControl},
	0xffe9: {"alt", gdkAlt}, 0xffea: {"alt", gdkAlt}, 0xffeb: {"meta", gdkSuper}, 0xffec: {"meta", gdkSuper},
	0xffe7: {"meta", gdkMeta}, 0xffe8: {"meta", gdkMeta},
}

// heldKey is a key down, by keycode (window.held): its keyval at the press, and once its down was reported (reported), the view that heard it, 0 for none, and its name, which its up reports. GTK reports a key's release with its keycode, and its keyval may differ from the press's as the modifiers changed in between.
type heldKey struct {
	keyval   uint32
	id       driver.NodeID
	name     string
	reported bool
}

// keyEvent names a key going down or up as KeyEvent does, with the modifiers held in state, and says whether it is a modifier alone. GDK's state at a press holds the modifiers held before it, so a modifier going down adds its own bit, and at a release still holds the modifier going up, which takes its own bit out unless another key of the same modifier stays down (the two Shifts).
func (w *window) keyEvent(keycode, keyval, state uint32, down bool) (name string, mods driver.Modifiers, modifier bool) {
	name = keyName(keyval, state&gdkShift != 0)
	if m, ok := modifierKeyvals[keyval]; ok {
		name, modifier = m.name, true
		if down {
			state |= m.bit
		} else {
			state &^= m.bit
			for code, k := range w.held {
				if o, ok := modifierKeyvals[k.keyval]; ok && code != keycode && o.bit == m.bit {
					state |= m.bit
				}
			}
		}
	}
	return name, modifiers(state), modifier
}

// keyDown reports a key going down, before the keymap and the view's own keys: to the focused view that hears keys (View.OnKeyDown), reporting whether it consumed the key, or, when no focused view hears keys and the app follows every key held (heldKeys), to no view (ID 0), which consumes nothing. Keys pressed while a field or a web view has the focus, the only places an input method composes, are not reported. The first report of the key at keycode records the view and the name its up reports (keyUp), even for a repeat of a key pressed while a field had the focus. A modifier alone is reported but never consumed, as no binding takes one.
func (w *window) keyDown(keycode, keyval, state uint32, repeat bool) bool {
	if w.focus != nil || w.focusedWeb() != nil {
		return false
	}
	v := w.controlFocus
	hears := v != nil && !v.deleted && v.events.Has(driver.EventKeyDown)
	if !hears && !w.heldKeys {
		return false
	}
	name, mods, modifier := w.keyEvent(keycode, keyval, state, true)
	if name == "" {
		return false
	}
	var id driver.NodeID
	if hears {
		id = v.id
	}
	if k := w.held[keycode]; !k.reported {
		k.id, k.name, k.reported = id, name, true
		w.held[keycode] = k
	}
	ev := driver.Event{Kind: driver.EventKeyDown, ID: id, Text: name, Modifiers: mods, Repeat: repeat}
	if !hears {
		// A down no view hears is only followed, so it goes without holding up the main loop.
		w.engine.Dispatch(ev)
		return false
	}
	consumed := w.engine.DispatchKeyDown(ev)
	return consumed && !modifier
}

// keyUp reports the key at keycode coming up, to the view that heard its down (View.OnKeyUp) or to none, with the name its down had, when its down was reported (keyDown), wherever the focus is now.
func (w *window) keyUp(keycode, keyval, state uint32) {
	k, ok := w.held[keycode]
	if !ok {
		return
	}
	delete(w.held, keycode)
	if !k.reported {
		return
	}
	_, mods, _ := w.keyEvent(keycode, keyval, state, false)
	w.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: k.id, Text: k.name, Modifiers: mods})
}

// releaseKeys reports every key held whose down was reported as coming up, with no modifiers, and forgets every key held, as the window loses the keyboard: their releases go to another window, and a key pressed again on return is a new press, not a repeat.
func (w *window) releaseKeys() {
	for _, k := range w.held {
		if k.reported && w.engine != nil {
			w.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: k.id, Text: k.name})
		}
	}
	clear(w.held)
}

// cbKeysLeave follows the focus leaving an embedded surface (setupInputs).
var cbKeysLeave = newCallback(keysLeaveCallback)

func keysLeaveCallback(controller, data uintptr) {
	if w := windowOf(data); w != nil {
		w.releaseKeys()
	}
}

// shortcut reports a key press matching the app's keymap or the
// application menu's shortcuts, and whether it took it.
func (w *window) shortcut(keyval, state uint32, repeat bool) bool {
	name := keyName(keyval, state&gdkShift != 0)
	if name == "" {
		return false
	}
	editing := w.focus != nil || w.focusedWeb() != nil
	var names []string
	ctrl, alt := state&gdkControl != 0, state&gdkAlt != 0
	shift, meta := state&gdkShift != 0, state&(gdkSuper|gdkMeta) != 0
	for _, control := range []string{"mod", "ctrl"} {
		var parts []string
		if ctrl {
			parts = append(parts, control)
		}
		if alt {
			parts = append(parts, "alt")
		}
		if shift {
			parts = append(parts, "shift")
		}
		if meta {
			parts = append(parts, "meta")
		}
		names = append(names, strings.Join(append(parts, name), "+"))
	}
	var focus driver.KeyUse
	if editing {
		focus = driver.KeyUseText
	} else if v := w.controlFocus; v != nil && !v.deleted {
		focus = driver.FocusKeyUse(v.kind, v.props.Role, v.events.Pressable())
	}
	if id, consumed := driver.FindKey(w.keys, names, focus, repeat); consumed {
		if id != 0 {
			w.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: id})
		}
		return true
	}
	if editing && state&(gdkControl|gdkAlt|gdkSuper|gdkMeta) == 0 {
		return false // a field's or a page's own keys
	}
	for _, key := range names {
		if w.menuShortcut(key) {
			return true
		}
	}
	return false
}

func keyReleasedCallback(controller uintptr, keyval, keycode, state uint32, data uintptr) {
	if w := windowOf(data); w != nil {
		w.keyUp(keycode, keyval, state)
		w.menuBarKey(keyval, state, false)
	}
}

// modifiersCallback reports a modifier key pressed or released while the
// pointer rests over the window as a mouse move to where it is with the
// new modifiers, so a view hears Control come and go under a still mouse.
func modifiersCallback(controller uintptr, state uint32, data uintptr) int32 {
	w := windowOf(data)
	if w == nil || !w.pointerIn || state&gdkButtonsMask != 0 {
		return 0
	}
	w.mouseMoved(state)
	return 0
}

// keyName names a keyval as core.NormalizeKey does, or "" for a key it
// has no name for.
func keyName(keyval uint32, shifted bool) string {
	switch {
	case keyval == gdkBackspace:
		return "backspace"
	case keyval == gdkTab:
		return "tab"
	case keyval == gdkReturn || keyval == gdkKPEnter:
		return "enter"
	case keyval == gdkEscape:
		return "escape"
	case keyval == gdkSpace:
		return "space"
	case keyval == gdkDelete:
		return "delete"
	case keyval == gdkInsert:
		return "insert"
	case keyval == gdkHome:
		return "home"
	case keyval == gdkEnd:
		return "end"
	case keyval == gdkPageUp:
		return "pageup"
	case keyval == gdkPageDown:
		return "pagedown"
	case keyval == gdkLeft:
		return "left"
	case keyval == gdkUp:
		return "up"
	case keyval == gdkRight:
		return "right"
	case keyval == gdkDown:
		return "down"
	case keyval >= gdkF1 && keyval <= gdkF35:
		return "f" + itoa(int(keyval-gdkF1)+1)
	case keyval >= gdkKP0 && keyval <= gdkKP9:
		return itoa(int(keyval - gdkKP0))
	case keyval >= 0xfe00 && keyval <= 0xffff:
		// Function and modifier keys, whose keyvals are no characters.
		return ""
	}
	r := rune(keyval)
	if unicode.IsPrint(r) && r != ' ' {
		if shifted {
			r = unshifted(r)
		}
		return string(unicode.ToLower(r))
	}
	return ""
}

// unshifted maps a shifted punctuation character back to the key that
// types it. GTK no longer exposes the keyboard layout's unshifted keys.
func unshifted(r rune) rune {
	if u := strings.IndexRune("~!@#$%^&*()_+{}|:\"<>?", r); u >= 0 {
		return []rune("`1234567890-=[]\\;',./")[u]
	}
	return r
}

// itoa is strconv.Itoa without the import at every call site.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// menuShortcut runs the menu item bound to key, if any.
func (w *window) menuShortcut(key string) bool {
	ref, ok := w.chrome.keys[key]
	if !ok || ref.notInText && (w.focus != nil || w.focusedWeb() != nil) {
		return false
	}
	return w.menuChosen(ref)
}

// menuChosen runs an application menu item's role, or reports its ID. An editing role goes to the focused field, the focused page or the text selection first, and reports its ID only when none of them takes it.
func (w *window) menuChosen(ref menuRef) bool {
	if (ref.role.EditsText() || ref.role == driver.MenuPasteAndMatchStyle) && w.menuEdit(ref.role) {
		return true
	}
	if ref.id != 0 {
		w.engine.Dispatch(driver.Event{Kind: driver.EventMenu, Count: ref.id})
		return true
	}
	switch ref.role {
	case driver.MenuClose:
		if w.primary || w.embedded {
			w.close()
		} else if w.engine != nil && !w.closing {
			w.engine.Dispatch(driver.Event{Kind: driver.EventWindowClose})
		}
	case driver.MenuQuit:
		if w.primary || w.embedded {
			w.close()
			break
		}
		var root *window
		windowHandles.Range(func(_, value any) bool {
			candidate := value.(*window)
			if candidate.primary && !candidate.embedded {
				root = candidate
				return false
			}
			return true
		})
		if root != nil {
			root.close()
		} else {
			w.close()
		}
	case driver.MenuMinimize:
		w.minimize()
	case driver.MenuZoom:
		w.toggleMaximize()
	case driver.MenuToggleFullScreen:
		if w.chrome.fullScreen {
			gtkWindowUnfullscreen(w.win)
		} else {
			gtkWindowFullscreen(w.win)
		}
		w.chrome.fullScreen = !w.chrome.fullScreen
		w.updateEnvironment()
	case driver.MenuUndo, driver.MenuRedo, driver.MenuCut, driver.MenuCopy, driver.MenuPaste, driver.MenuPasteAndMatchStyle, driver.MenuDelete, driver.MenuSelectAll:
		// Nothing took the edit; the item still takes its key, as it does with an editor.
	case driver.MenuStartSpeaking:
		w.speakMenu(true)
	case driver.MenuStopSpeaking:
		w.speakMenu(false)
	default:
		return false
	}
	return true
}

// menuEdit carries out an editing role in the focused field, else the focused page, else the text selection (Copy and Select All), reporting whether one of them took it.
func (w *window) menuEdit(role driver.MenuRole) bool {
	if w.focus != nil {
		if role == driver.MenuSelectAll && w.focus.textView == 0 {
			gtkEditableSelectRegion(w.focus.entry, 0, -1)
			return true
		}
		action := map[driver.MenuRole]string{
			driver.MenuUndo: "text.undo", driver.MenuRedo: "text.redo",
			driver.MenuCut: "clipboard.cut", driver.MenuCopy: "clipboard.copy",
			driver.MenuPaste: "clipboard.paste", driver.MenuPasteAndMatchStyle: "clipboard.paste",
			driver.MenuDelete: "selection.delete", driver.MenuSelectAll: "selection.select-all",
		}[role]
		gtkWidgetActivateAction(w.focus.entry, cstr(action), 0)
		return true
	}
	return w.webEdit(role) || w.selectionEdit(role)
}

// GDK's event types and modifier bits the shield reads.
const (
	gdkMotionNotify  = 1
	gdkButtonPress   = 2
	gdkButtonRelease = 3
	gdkScrollEvent   = 15
	gdkTouchBegin    = 17
	gdkScrollSmooth  = 4

	scrollBothAxes       = 3 // GTK_EVENT_CONTROLLER_SCROLL_BOTH_AXES
	scrollKinetic        = 8 // GTK_EVENT_CONTROLLER_SCROLL_KINETIC
	gdkScrollUnitSurface = 1
	gdkButtonsMask       = 1<<8 | 1<<9 | 1<<10
	gdkButton1           = 1 << 8
	gdkButton2           = 1 << 9
	gdkButton3           = 1 << 10
)

// shieldCallback keeps from a hosted control (a page, a video's
// controls) the events GTK would give it where the app does not show it,
// as NSH's shields_page does: GTK picks widgets by their allocations and
// knows nothing of rounded clips, so a control a scroll view or a
// rounded parent cuts off would take clicks, hovers and wheels outside
// what is drawn of it. The view actually under the pointer gets them.
func shieldCallback(controller, event, data uintptr) int32 {
	w := windowOf(data)
	if w == nil || w.closing {
		return 0
	}
	w.currentEvent = event
	defer func() { w.currentEvent = 0 }()
	return boolInt(w.shield(event))
}

func (w *window) shield(event uintptr) bool {
	typ := gdkEventGetEventType(event)
	if typ == gdkButtonPress || typ == gdkTouchBegin {
		w.engine.InputModality(driver.ModalityPointer)
	}
	if typ != gdkMotionNotify && typ != gdkButtonPress && typ != gdkButtonRelease && typ != gdkScrollEvent {
		return false
	}
	at, ok := w.eventPoint(event)
	if !ok {
		return false
	}
	if w.shielding {
		// The press went to the app; its moves and release follow.
		switch typ {
		case gdkButtonRelease:
			w.shielding = false
			w.pointerUp(at)
			return true
		case gdkMotionNotify:
			w.movePointer(at, gdkEventGetModifierState(event))
			return true
		}
	}
	if typ == gdkButtonRelease || typ == gdkMotionNotify && gdkEventGetModifierState(event)&gdkButtonsMask != 0 {
		return false // drags that began in the control stay with it
	}
	if !w.hostsControls() {
		return false
	}
	v := w.controlAt(gtkWidgetPick(w.surface, float64(at.X), float64(at.Y), gtkPickDefault))
	if v == nil {
		return false
	}
	if root := w.view(w.root); root != nil && hit(root, at, func(*view) bool { return true }) == v {
		return false
	}
	switch typ {
	case gdkMotionNotify:
		w.movePointer(at, gdkEventGetModifierState(event))
	case gdkButtonPress:
		w.shielding = true
		w.pointerDown(at, gdkButtonEventGetButton(event), gdkEventGetDevice(event), gdkEventGetModifierState(event), 1)
	case gdkScrollEvent:
		var dx, dy float64
		switch gdkScrollEventGetDirection(event) {
		case gdkScrollSmooth:
			gdkScrollEventGetDeltas(event, &dx, &dy)
		case 0:
			dy = -1
		case 1:
			dy = 1
		case 2:
			dx = -1
		case 3:
			dx = 1
		}
		w.lastPointer = at
		points := gdkScrollEventGetUnit != nil && gdkScrollEventGetUnit(event) == gdkScrollUnitSurface
		w.wheelEvent(gdkEventGetModifierState(event), at, dx, dy, points)
	}
	return true
}

// eventPoint returns where event happened in the surface's points.
func (w *window) eventPoint(event uintptr) (driver.Point, bool) {
	var x, y float64
	if gdkEventGetPosition(event, &x, &y) == 0 {
		return driver.Point{}, false
	}
	native := gtkWidgetGetNative(w.surface)
	if native == 0 {
		return driver.Point{}, false
	}
	var sx, sy float64
	gtkNativeGetSurfaceTransform(native, &sx, &sy)
	var out graphenePoint
	if gtkWidgetComputePoint(native, w.surface, &graphenePoint{float32(x - sx), float32(y - sy)}, &out) == 0 {
		return driver.Point{}, false
	}
	return driver.Point{X: driver.Points(out.x), Y: driver.Points(out.y)}, true
}
