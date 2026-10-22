//go:build linux && !android

package linux

import (
	"math"
	"strings"
	"time"
	"unicode"

	"github.com/oneo-me/nsgo/core"
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
	gdkKP0       = 0xffb0
	gdkKP9       = 0xffb9
)

// GDK modifier bits.
const (
	gdkShift   = 1 << 0
	gdkControl = 1 << 2
	gdkAlt     = 1 << 3
	gdkSuper   = 1 << 26
	gdkMeta    = 1 << 28
)

const (
	propagationPhaseCapture = 1
	propagationPhaseBubble  = 2
)

// pan is a drag in progress on a view that follows pans.
type pan struct {
	v          *view
	start      core.Point
	active     bool
	failed     bool
	last       core.Point
	lastTime   time.Time
	velocity   core.Point
	activation core.PanActivation
}

// twoFinger is a pinch or a rotation in progress.
type twoFinger struct {
	v        *view
	kind     core.EventKind
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
	at := core.Point{X: core.Points(x), Y: core.Points(y)}
	w.lastPointer, w.pressConsumed = at, false
	// A press on the surface leaves the focused field or page; a press
	// on either never reaches this controller.
	if w.focus != nil || w.focusedWeb() != nil {
		w.focusEntry(nil)
	}
	w.pressOutside(at)
	switch gtkGestureSingleGetCurrentButton(gesture) {
	case 3:
		w.contextMenu(at)
		w.pressConsumed = true
		return
	}
	if w.beginWindowDrag(gesture, at) {
		w.pressConsumed = true
	}
	w.panDown(at)
}

func clickReleasedCallback(gesture uintptr, n int32, x, y float64, data uintptr) {
	w := windowOf(data)
	if w == nil || w.closing || w.pressConsumed {
		return
	}
	if w.panning != nil && w.panning.active {
		return
	}
	w.press(core.Point{X: core.Points(x), Y: core.Points(y)})
}

func motionCallback(controller uintptr, x, y float64, data uintptr) {
	w := windowOf(data)
	if w == nil {
		return
	}
	w.lastPointer = core.Point{X: core.Points(x), Y: core.Points(y)}
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
	if w == nil || w.panning == nil || w.panning.failed {
		return
	}
	g := w.panning
	if g.v.deleted {
		w.panning = nil
		return
	}
	from := g.start
	now := core.Point{X: from.X + core.Points(offsetX), Y: from.Y + core.Points(offsetY)}
	d := core.Point{X: now.X - from.X, Y: now.Y - from.Y}
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
		w.engine.Dispatch(core.Event{Kind: core.EventPan, ID: g.v.id, Phase: core.GestureBegan, Point: d})
	}
	dt := time.Since(g.lastTime).Seconds()
	var velocity core.Point
	if dt > 0 {
		velocity = core.Point{X: core.Points(float64(now.X-g.last.X) / dt), Y: core.Points(float64(now.Y-g.last.Y) / dt)}
	}
	g.last, g.lastTime, g.velocity = now, time.Now(), velocity
	w.engine.Dispatch(core.Event{Kind: core.EventPan, ID: g.v.id, Phase: core.GestureChanged, Point: d, Velocity: velocity})
}

func dragEndCallback(gesture uintptr, offsetX, offsetY float64, data uintptr) {
	w := windowOf(data)
	if w == nil || w.panning == nil {
		return
	}
	g := w.panning
	w.panning = nil
	if g.failed || g.v.deleted {
		return
	}
	d := core.Point{X: core.Points(offsetX), Y: core.Points(offsetY)}
	if !g.active && !activates(g.activation, d) {
		return
	}
	if !g.active {
		g.active = true
		w.engine.Dispatch(core.Event{Kind: core.EventPan, ID: g.v.id, Phase: core.GestureBegan, Point: d})
	}
	w.engine.Dispatch(core.Event{Kind: core.EventPan, ID: g.v.id, Phase: core.GestureEnded, Point: d, Velocity: g.velocity})
}

func longPressCallback(gesture uintptr, x, y float64, data uintptr) {
	w := windowOf(data)
	if w == nil {
		return
	}
	at := core.Point{X: core.Points(x), Y: core.Points(y)}
	if v := w.hitEvent(at, core.EventLongPress); v != nil {
		w.engine.Dispatch(core.Event{Kind: core.EventLongPress, ID: v.id})
	}
}

func zoomChangedCallback(gesture uintptr, scale float64, data uintptr) {
	w := windowOf(data)
	if w == nil {
		return
	}
	w.reportTwoFinger(core.EventPinch, float32(scale))
}

func rotateChangedCallback(gesture uintptr, angle, delta float64, data uintptr) {
	w := windowOf(data)
	if w == nil {
		return
	}
	w.reportTwoFinger(core.EventRotation, float32(delta*math.Pi/180))
}

func gestureEndCallback(gesture uintptr, sequence uintptr, data uintptr) {
	w := windowOf(data)
	if w == nil {
		return
	}
	for _, g := range []*twoFinger{w.pinching, w.rotating} {
		if g != nil && g.v != nil && !g.v.deleted {
			w.engine.Dispatch(core.Event{Kind: g.kind, ID: g.v.id, Phase: core.GestureEnded, Value: g.value,
				Velocity: core.Point{X: core.Points(g.velocity)}})
		}
	}
	w.pinching, w.rotating = nil, nil
}

func scrollCallback(controller uintptr, dx, dy float64, data uintptr) int32 {
	w := windowOf(data)
	if w == nil {
		return 0
	}
	state := gtkEventControllerGetCurrentEventState(controller)
	if state&gdkControl != 0 {
		w.wheelPinch(w.lastPointer, dy)
		return 1
	}
	if state&gdkShift != 0 {
		dx, dy = dy, 0
	}
	// GTK's wheel turns come as whole units; a touchpad's as points.
	notch := 48.0
	if math.Abs(dx) <= 3 && math.Abs(dy) <= 3 {
		dx, dy = dx*notch, dy*notch
	}
	w.wheel(w.lastPointer, dx, dy)
	return 1
}

func keyPressedCallback(controller uintptr, keyval, keycode, state uint32, data uintptr) int32 {
	w := windowOf(data)
	if w == nil {
		return 0
	}
	if w.focus != nil {
		// The field takes plain keys and its own editing shortcuts.
		if keyval != gdkEscape && state&(gdkControl|gdkAlt|gdkSuper|gdkMeta) == 0 {
			return 0
		}
	} else if w.focusedWeb() != nil {
		// The page takes plain keys, Escape among them.
		if state&(gdkControl|gdkAlt|gdkSuper|gdkMeta) == 0 {
			return 0
		}
	}
	if w.shortcut(keyval, state) {
		return 1
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

// setupInputs adds GTK's event controllers to the drawing area and the
// window.
func (w *window) setupInputs() {
	click := gtkGestureClickNew()
	gtkGestureSingleSetButton(click, 0)
	gSignalConnect(click, "pressed", cbClickPressed, w.handle)
	gSignalConnect(click, "released", cbClickReleased, w.handle)
	gtkWidgetAddController(w.area, click)

	motion := gtkEventControllerMotionNew()
	gSignalConnect(motion, "motion", cbMotion, w.handle)
	gtkWidgetAddController(w.area, motion)

	drag := gtkGestureDragNew()
	gtkGestureSingleSetButton(drag, 1)
	gSignalConnect(drag, "drag-begin", cbDragBegin, w.handle)
	gSignalConnect(drag, "drag-update", cbDragUpdate, w.handle)
	gSignalConnect(drag, "drag-end", cbDragEnd, w.handle)
	gtkWidgetAddController(w.area, drag)

	long := gtkGestureLongPressNew()
	gSignalConnect(long, "pressed", cbLongPress, w.handle)
	gtkWidgetAddController(w.area, long)

	zoom := gtkGestureZoomNew()
	gSignalConnect(zoom, "scale-changed", cbZoomChanged, w.handle)
	gSignalConnect(zoom, "end", cbGestureEnd, w.handle)
	gtkWidgetAddController(w.area, zoom)

	rotate := gtkGestureRotateNew()
	gSignalConnect(rotate, "angle-changed", cbRotateChanged, w.handle)
	gSignalConnect(rotate, "end", cbGestureEnd, w.handle)
	gtkWidgetAddController(w.area, rotate)

	scroll := gtkEventControllerScrollNew(3) // both axes
	gSignalConnect(scroll, "scroll", cbScroll, w.handle)
	gtkWidgetAddController(w.area, scroll)

	keys := gtkEventControllerKeyNew()
	gtkEventControllerSetPropagationPhase(keys, propagationPhaseCapture)
	gSignalConnect(keys, "key-pressed", cbKeyPressed, w.handle)
	gtkWidgetAddController(w.win, keys)
}

// hitEvent returns the innermost view under p that reports kind.
func (w *window) hitEvent(p core.Point, kind core.EventKind) *view {
	root := w.view(w.root)
	if root == nil {
		return nil
	}
	return hit(root, p, func(v *view) bool { return v.events.Has(kind) })
}

// panDown starts following a drag at p if a view there follows pans.
func (w *window) panDown(p core.Point) {
	w.panning = nil
	v := w.hitEvent(p, core.EventPan)
	if v == nil {
		return
	}
	w.panning = &pan{v: v, start: p, last: p, lastTime: time.Now(), activation: v.props.PanActivation}
}

// activates reports whether a drag of d has moved far enough to start a
// pan.
func activates(a core.PanActivation, d core.Point) bool {
	if a.ActiveOffsetX == 0 && a.ActiveOffsetY == 0 {
		return math.Hypot(float64(d.X), float64(d.Y)) >= float64(core.DefaultPanSlop)
	}
	return a.ActiveOffsetX != 0 && math.Abs(float64(d.X)) >= float64(a.ActiveOffsetX) ||
		a.ActiveOffsetY != 0 && math.Abs(float64(d.Y)) >= float64(a.ActiveOffsetY)
}

// fails reports whether a drag of d crossed a fail offset.
func fails(a core.PanActivation, d core.Point) bool {
	return a.FailOffsetX != 0 && math.Abs(float64(d.X)) > float64(a.FailOffsetX) ||
		a.FailOffsetY != 0 && math.Abs(float64(d.Y)) > float64(a.FailOffsetY)
}

// reportTwoFinger reports a touchscreen pinch or rotation under the
// pointer, beginning it on the first change.
func (w *window) reportTwoFinger(kind core.EventKind, value float32) {
	slot := &w.pinching
	if kind == core.EventRotation {
		slot = &w.rotating
	}
	if *slot == nil {
		v := w.hitEvent(w.lastPointer, kind)
		if v == nil {
			return
		}
		*slot = &twoFinger{v: v, kind: kind, value: rest(kind), lastTime: time.Now()}
		w.engine.Dispatch(core.Event{Kind: kind, ID: v.id, Phase: core.GestureBegan, Value: rest(kind)})
	}
	g := *slot
	if g.v.deleted {
		*slot = nil
		return
	}
	if kind == core.EventRotation {
		value = g.value + value
	}
	now := time.Now()
	dt := now.Sub(g.lastTime).Seconds()
	var velocity float32
	if dt > 0 {
		velocity = (value - g.value) / float32(dt)
	}
	g.value, g.lastTime, g.velocity = value, now, velocity
	w.engine.Dispatch(core.Event{Kind: kind, ID: g.v.id, Phase: core.GestureChanged, Value: value,
		Velocity: core.Point{X: core.Points(velocity)}})
}

// rest is a gesture's value when it begins.
func rest(kind core.EventKind) float32 {
	if kind == core.EventPinch {
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
func (w *window) wheelPinch(p core.Point, delta float64) {
	g := w.pinching
	if g != nil && g.v.deleted {
		g = nil
	}
	if g == nil {
		v := w.hitEvent(p, core.EventPinch)
		if v == nil {
			return
		}
		g = &twoFinger{v: v, kind: core.EventPinch, value: 1, lastTime: time.Now()}
		w.pinching = g
		w.engine.Dispatch(core.Event{Kind: core.EventPinch, ID: v.id, Phase: core.GestureBegan, Value: 1})
	}
	g.value *= float32(math.Pow(2, -delta/20))
	w.engine.Dispatch(core.Event{Kind: core.EventPinch, ID: g.v.id, Phase: core.GestureChanged, Value: g.value})
	if w.pinchTimeout != 0 {
		gSourceRemove(w.pinchTimeout)
	}
	w.pinchTimeout = gTimeoutAdd(pinchIdleMillis, cbPinchTimeout, w.handle)
}

func pinchTimeoutCallback(data uintptr) int32 {
	w := windowOf(data)
	w.pinchTimeout = 0
	if g := w.pinching; g != nil {
		w.pinching = nil
		if !g.v.deleted {
			w.engine.Dispatch(core.Event{Kind: core.EventPinch, ID: g.v.id, Phase: core.GestureEnded, Value: g.value})
		}
	}
	return 0 // G_SOURCE_REMOVE
}

// beginWindowDrag starts moving the window when the press is in a window
// drag region and not on a nearer control.
func (w *window) beginWindowDrag(gesture uintptr, at core.Point) bool {
	root := w.view(w.root)
	if root == nil {
		return false
	}
	drag := hit(root, at, func(v *view) bool { return v.props.WindowDrag != core.WindowDragInherit })
	if drag == nil || drag.props.WindowDrag != core.WindowDragOn {
		return false
	}
	if control := hit(root, at, func(v *view) bool {
		return v.events.Has(core.EventPress) || v.events.Has(core.EventPan) || v.kind == core.KindInput || v.kind == core.KindWeb
	}); control != nil && descendant(control, drag) {
		return false
	}
	device := gtkGestureGetDevice(gesture)
	if device == 0 {
		return false
	}
	surface := gtkNativeGetSurface(gtkWidgetGetNative(w.area))
	if surface == 0 {
		return false
	}
	gdkToplevelBeginMove(surface, device, 1, float64(at.X), float64(at.Y), 0)
	return true
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

// shortcut reports a key press matching the app's keymap or the
// application menu's shortcuts, and whether it took it.
func (w *window) shortcut(keyval, state uint32) bool {
	name := keyName(keyval, state&gdkShift != 0)
	if name == "" {
		return false
	}
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
		key := strings.Join(append(parts, name), "+")
		if w.keys[key] {
			w.engine.Dispatch(core.Event{Kind: core.EventKey, Text: key})
			return true
		}
		if w.menuShortcut(key) {
			return true
		}
	}
	return false
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
	if !ok {
		return false
	}
	return w.menuChosen(ref)
}

// menuChosen runs an application menu item's role, or reports its ID.
func (w *window) menuChosen(ref menuRef) bool {
	if ref.id != 0 {
		w.engine.Dispatch(core.Event{Kind: core.EventMenu, Count: ref.id})
		return true
	}
	switch ref.role {
	case core.MenuQuit, core.MenuClose:
		w.close()
	case core.MenuMinimize:
		gtkWindowMinimize(w.win)
	case core.MenuZoom:
		if w.chrome.max {
			gtkWindowUnmaximize(w.win)
		} else {
			gtkWindowMaximize(w.win)
		}
		w.chrome.max = !w.chrome.max
	case core.MenuToggleFullScreen:
		if w.chrome.fullScreen {
			gtkWindowUnfullscreen(w.win)
		} else {
			gtkWindowFullscreen(w.win)
		}
		w.chrome.fullScreen = !w.chrome.fullScreen
		w.updateEnvironment()
	case core.MenuUndo, core.MenuRedo, core.MenuCut, core.MenuCopy, core.MenuPaste, core.MenuSelectAll:
		if w.focus != nil {
			action := map[core.MenuRole]string{
				core.MenuUndo: "text.undo", core.MenuRedo: "text.redo",
				core.MenuCut: "clipboard.cut", core.MenuCopy: "clipboard.copy",
				core.MenuPaste: "clipboard.paste", core.MenuSelectAll: "selection.select-all",
			}[ref.role]
			gtkWidgetActivateAction(w.focus.entry, cstr(action), 0)
			return true
		}
		w.webEdit(ref.role)
	default:
		return false
	}
	return true
}
