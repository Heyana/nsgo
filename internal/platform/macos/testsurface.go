//go:build darwin && !ios && (nsgotest || nsgoautomation)

package macos

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// A surface acting as the user does, with events posted to the application's queue, for the test host (testhost.go) and for nsgo dev's automation (automation.go), whose live surfaces are the running app's windows, with no test's options: test is nil.

// onMainSync runs fn on the main thread and waits for it. It goes through
// the run loop in its common modes, as batches do, rather than the main
// queue: a context menu tracks in a loop of its own inside a main-queue
// block, which holds the queue until the menu closes.
func onMainSync(fn func()) {
	done := make(chan struct{})
	callsMu.Lock()
	calls = append(calls, func() {
		defer close(done)
		fn()
	})
	callsMu.Unlock()
	if caller == 0 {
		callerOnce.Do(func() {
			class := apple.RegisterClass("NsgoTestCaller", apple.Class("NSObject"), nil, []objc.MethodDef{{
				Cmd: sel("call"),
				Fn: func(objc.ID, objc.SEL) {
					callsMu.Lock()
					fns := calls
					calls = nil
					callsMu.Unlock()
					for _, fn := range fns {
						fn()
					}
				},
			}})
			caller = objc.ID(class).Send(selNew)
		})
	}
	caller.Send(sel("performSelectorOnMainThread:withObject:waitUntilDone:"), sel("call"), objc.ID(0), false)
	<-done
}

var (
	caller     objc.ID
	callerOnce sync.Once
	callsMu    sync.Mutex
	calls      []func()
)

// testSurface acts on s: for a test, the main window its app opened, else its own surface, own, which shows no view; for nsgo dev's automation, the running app's surface it was made for.
type testSurface struct {
	s       *surface
	own     *surface
	test    *testWindow
	fixed   bool // act on s even when another window is main
	hovered []objc.ID
	clicks  testhost.Clicks
	pressID int // the last press's marker, read and written on the main thread
	// heldFlags are the flags of the modifiers KeyDown and KeyCodeDown hold, which every key after them carries until their up (holdModifier).
	heldFlags uint
	// heldModifiers are the modifier keys held, by key code, by KeyDown of the modifier's name, which holds its left-hand key (byName), or by KeyCodeDown of its position (byPosition). A key stays down while either holds it.
	heldModifiers map[uint16]struct{ byName, byPosition bool }
	// heldCodes are the key codes of the keys other than a modifier that KeyDown holds, whose next KeyDown is their auto-repeat.
	heldCodes map[uint16]bool
}

// follow has a test's surface act on the main window its app opened, while one is open, as the test's model follows it (testhost.WindowHost), else on its own surface.
func (t *testSurface) follow() {
	if t.test == nil || t.fixed {
		return
	}
	if main := t.test.main.Load(); main != nil {
		t.s = main
	} else {
		t.s = t.own
	}
}

func (t *testSurface) WindowSurface(id uint32) testhost.Surface {
	var out *testSurface
	onMainSync(func() {
		for s := range surfaces {
			if s.test == t.test && s.id == id && !s.closed {
				out = &testSurface{s: s, own: t.own, test: t.test, fixed: true}
				break
			}
		}
	})
	if out == nil {
		return nil
	}
	return out
}

func (t *testSurface) RequestClose() error {
	t.follow()
	if t.s.window == 0 {
		return errors.New("this surface has no window to close")
	}
	onMainSync(func() { t.s.window.Send(sel("performClose:"), objc.ID(0)) })
	return nil
}

// windowPoint is p, in the surface's points from its top left, in the
// window's base coordinates.
func (t *testSurface) windowPoint(p driver.Point) apple.CGPoint {
	return objc.Send[apple.CGPoint](t.s.native, sel("convertPoint:toView:"), apple.CGPoint{X: float64(p.X), Y: float64(p.Y)}, objc.ID(0))
}

// mouse posts a mouse event of kind at p to the application's queue.
func (t *testSurface) mouse(kind uint, p driver.Point, clicks int) {
	t.mouseWith(kind, p, clicks, driver.Modifiers{})
}

func (t *testSurface) mouseWith(kind uint, p driver.Point, clicks int, mods driver.Modifiers) {
	onMainSync(func() { t.postMouse(kind, p, clicks, mods) })
}

func (t *testSurface) postMouse(kind uint, p driver.Point, clicks int, mods driver.Modifiers) {
	if t.s.closed {
		// What it did closed the window, as a key or a press on Close does: its next event has nowhere to go.
		return
	}
	var flags uint
	if mods.Control {
		flags |= flagControl
	}
	if mods.Alt {
		flags |= flagOption
	}
	if mods.Shift {
		flags |= flagShift
	}
	if mods.Platform {
		flags |= flagCommand
	}
	if t.s.testPointer != nil {
		*t.s.testPointer = t.windowPoint(p)
	}
	ev := objc.ID(apple.Class("NSEvent")).Send(
		sel("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
		kind, t.windowPoint(p), flags, uptime(), objc.Send[int](t.s.window, sel("windowNumber")), objc.ID(0), 0, clicks, float32(1))
	rememberTestInput(ev)
	nsApp().Send(sel("postEvent:atStart:"), ev, false)
}

func (t *testSurface) Press(at driver.Point, hold time.Duration, mods driver.Modifiers) error {
	t.follow()
	// The view the window's hit testing finds there: a press of another one starts a new click sequence.
	var target any
	onMainSync(func() {
		t.s.commitPending()
		if n := t.hit(at); n != 0 {
			target = n
			if v := views[n]; v != nil {
				target = v.id
			}
		}
	})
	interval := time.Duration(objc.Send[float64](objc.ID(apple.Class("NSEvent")), sel("doubleClickInterval")) * float64(time.Second))
	clicks := t.clicks.Press(at, target, time.Now(), interval)
	t.mouseWith(evMoved, at, 0, mods)
	t.mouseWith(evLeftDown, at, clicks, mods)
	if hold > 0 {
		time.Sleep(hold + 50*time.Millisecond)
	}
	onMainSync(func() {
		t.postMouse(evLeftUp, at, clicks, mods)
		if hold == 0 && !t.s.closed {
			t.markPress()
		}
	})
	return nil
}

const pressMarker int16 = 0x4e53

// These serials belong to AppKit's main thread and follow its shared event queue across windows.
var pressPosted, pressHandled int

// markPress posts an event behind the press. The engine can reach its barrier before AppKit delivers the mouse events, so the step also checks this marker when deciding whether the app is quiet.
func (t *testSurface) markPress() {
	const applicationDefined = 15 // NSEventTypeApplicationDefined
	nsEvent := objc.ID(apple.Class("NSEvent"))
	if pressPosted == 0 {
		handler := objcblock.New(func(_ objc.Block, event objc.ID) objc.ID {
			if objc.Send[int16](event, sel("subtype")) != pressMarker {
				return event
			}
			pressHandled = max(pressHandled, objc.Send[int](event, sel("data1")))
			return 0
		})
		nsEvent.Send(sel("addLocalMonitorForEventsMatchingMask:handler:"), uint64(1<<applicationDefined), handler)
		handler.Release()
	}
	pressPosted++
	t.pressID = pressPosted
	event := nsEvent.Send(sel("otherEventWithType:location:modifierFlags:timestamp:windowNumber:context:subtype:data1:data2:"),
		uint(applicationDefined), apple.CGPoint{}, uint(0), uptime(), 0, objc.ID(0), pressMarker, t.pressID, 0)
	nsApp().Send(sel("postEvent:atStart:"), event, false)
}

func (t *testSurface) RightClick(at driver.Point) error {
	t.follow()
	t.mouse(evRightDown, at, 1)
	t.mouse(evRightUp, at, 1)
	return nil
}

// MouseDown implements testhost.MouseButtonSurface for all five mouse buttons.
func (t *testSurface) MouseDown(at driver.Point, button driver.MouseButton, mods driver.Modifiers) error {
	return t.mouseButtonStep(at, button, mods, true)
}

// MouseUp implements testhost.MouseButtonSurface for all five mouse buttons.
func (t *testSurface) MouseUp(at driver.Point, button driver.MouseButton, mods driver.Modifiers) error {
	return t.mouseButtonStep(at, button, mods, false)
}

func (t *testSurface) mouseButtonStep(at driver.Point, button driver.MouseButton, mods driver.Modifiers, down bool) error {
	t.follow()
	const evRightUp, evOtherDown, evOtherUp = 4, 25, 26
	switch button {
	case driver.MouseButtonLeft:
		kind := uint(evLeftUp)
		if down {
			kind = evLeftDown
		}
		t.mouseWith(kind, at, 1, mods)
		return nil
	case driver.MouseButtonRight:
		kind := uint(evRightUp)
		if down {
			kind = evRightDown
		}
		t.mouseWith(kind, at, 1, mods)
		return nil
	case driver.MouseButtonMiddle, driver.MouseButtonBack, driver.MouseButtonForward:
		btnNum := int(button) - 1 // Middle=2, Back=3, Forward=4
		if button == driver.MouseButtonMiddle {
			btnNum = 2
		} else if button == driver.MouseButtonBack {
			btnNum = 3
		} else {
			btnNum = 4
		}
		loadCG()
		onMainSync(func() {
			if t.s.closed {
				return
			}
			t.s.commitPending()
			if t.s.testPointer != nil {
				*t.s.testPointer = t.windowPoint(at)
			}
			cgType := uint32(evOtherUp)
			if down {
				cgType = evOtherDown
			}
			cg := cgEventCreateMouseEvent(0, cgType, t.screenPoint(at), uint32(btnNum))
			if cg == 0 {
				return
			}
			defer apple.Release(cg)
			const fieldClickState, fieldButtonNumber, fieldWindowUnder, fieldWindowHandle = 1, 3, 91, 92
			winNum := int64(objc.Send[int](t.s.window, sel("windowNumber")))
			cgEventSetIntegerValueField(cg, fieldClickState, 1)
			cgEventSetIntegerValueField(cg, fieldButtonNumber, int64(btnNum))
			cgEventSetIntegerValueField(cg, fieldWindowUnder, winNum)
			cgEventSetIntegerValueField(cg, fieldWindowHandle, winNum)
			var flags uint64
			if mods.Shift {
				flags |= flagShift
			}
			if mods.Control {
				flags |= flagControl
			}
			if mods.Alt {
				flags |= flagOption
			}
			if mods.Platform {
				flags |= flagCommand
			}
			cgEventSetFlags(cg, flags)
			ev := objc.ID(apple.Class("NSEvent")).Send(sel("eventWithCGEvent:"), cg)
			if ev == 0 {
				return
			}
			rememberTestInput(ev)
			if ev.Send(selWindow) == t.s.window {
				nsApp().Send(sel("postEvent:atStart:"), ev, false)
				return
			}
			// When AppKit does not associate a synthetic CGEvent with a background window, deliver directly through the surface's handlers at the exact window point.
			loc := t.windowPoint(at)
			if down {
				t.s.mouseButtonDownAt(button, loc, mods, 1)
			} else {
				t.s.mouseButtonUpAt(button, loc, mods, 1)
			}
		})
		return nil
	default:
		return fmt.Errorf("unsupported mouse button %s", button)
	}
}

// BlurWindow implements testhost.BlurWindowSurface, releasing held keys and mouse buttons as NSWindowDidResignKeyNotification does.
func (t *testSurface) BlurWindow() error {
	t.follow()
	onMainSync(func() {
		if t.s.closed {
			return
		}
		t.s.releaseKeys()
		t.s.releaseMouseButtons()
	})
	return nil
}

// Drag rests where it lands, then moves 10 points a frame, as a hand.
func (t *testSurface) Drag(path []driver.Point) error {
	t.follow()
	if len(path) == 0 {
		return errors.New("an empty drag")
	}
	t.mouse(evMoved, path[0], 0)
	t.mouse(evLeftDown, path[0], 1)
	time.Sleep(100 * time.Millisecond)
	at := path[0]
	for _, to := range path[1:] {
		for _, p := range steps(at, to, 10) {
			t.mouse(evLeftDragged, p, 1)
			time.Sleep(16 * time.Millisecond)
		}
		at = to
	}
	t.mouse(evLeftUp, at, 1)
	return nil
}

// Pinch finds the native view under at and gives its gesture handler
// the samples AppKit would read from a magnify event.
func (t *testSurface) Pinch(at driver.Point, scales []float32) error {
	t.follow()
	if len(scales) == 0 {
		return nil
	}
	var err error
	onMainSync(func() {
		t.s.commitPending()
		location := t.windowPoint(at)
		if t.s.testPointer != nil {
			*t.s.testPointer = location
		}
		hit := t.hit(at)
		for hit != 0 && views[hit] == nil {
			hit = hit.Send(selSuperview)
		}
		now := uptime()
		send := func(phase uint, delta float64) bool {
			sample := trackpadSample{phase: phase, time: now, at: location}
			now += driver.FrameInterval.Seconds()
			return magnifyAt(hit, sample, delta)
		}
		if !send(phaseBegan, float64(scales[0])-1) {
			err = fmt.Errorf("no native pinch view at %v", at)
			return
		}
		for i := 1; i < len(scales); i++ {
			send(phaseChanged, float64(scales[i])-float64(scales[i-1]))
		}
		send(phaseEnded, 0)
	})
	return err
}

// steps are the points from a (excluded) to b, at most step apart.
func steps(a, b driver.Point, step driver.Points) []driver.Point {
	dx, dy := b.X-a.X, b.Y-a.Y
	n := int(max(abs(dx), abs(dy))/step) + 1
	out := make([]driver.Point, n)
	for i := range out {
		f := driver.Points(i+1) / driver.Points(n)
		out[i] = driver.Point{X: a.X + dx*f, Y: a.Y + dy*f}
	}
	return out
}

var (
	cgEventCreateMouseEvent          func(source uintptr, mouseType uint32, pos apple.CGPoint, button uint32) uintptr
	cgEventCreateScrollWheelEvent2   func(source uintptr, units uint32, count uint32, w1, w2, w3 int32) uintptr
	cgEventSetIntegerValueField      func(event uintptr, field uint32, value int64)
	cgEventSetFlags                  func(event uintptr, flags uint64)
	cgEventSetLocation               func(event uintptr, p apple.CGPoint)
	cgWindowListCreateImage          func(bounds apple.CGRect, option uint32, window uint32, imageOption uint32) uintptr
	cgWindowListCreateImageFromArray func(bounds apple.CGRect, windows uintptr, imageOption uint32) uintptr
	cfWindowIDArrayCreate            func(allocator uintptr, values *uintptr, count int, callbacks uintptr) uintptr
	cgPopupMenuLevel                 int32
	screenshotFallbackOnce           sync.Once
)

func loadCG() {
	if cgEventSetLocation != nil {
		return
	}
	cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW)
	if err != nil {
		panic(err)
	}
	purego.RegisterLibFunc(&cgEventCreateMouseEvent, cg, "CGEventCreateMouseEvent")
	purego.RegisterLibFunc(&cgEventCreateScrollWheelEvent2, cg, "CGEventCreateScrollWheelEvent2")
	purego.RegisterLibFunc(&cgEventSetIntegerValueField, cg, "CGEventSetIntegerValueField")
	purego.RegisterLibFunc(&cgEventSetFlags, cg, "CGEventSetFlags")
	purego.RegisterLibFunc(&cgEventSetLocation, cg, "CGEventSetLocation")
	// Deprecated since macOS 14; a system without it captures through AppKit instead of failing to load.
	if _, err := purego.Dlsym(cg, "CGWindowListCreateImage"); err == nil {
		purego.RegisterLibFunc(&cgWindowListCreateImage, cg, "CGWindowListCreateImage")
	}
	if addr, err := purego.Dlsym(cg, "CGWindowListCreateImageFromArray"); err == nil {
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_NOW)
		if err == nil {
			purego.RegisterFunc(&cgWindowListCreateImageFromArray, addr)
			purego.RegisterLibFunc(&cfWindowIDArrayCreate, cf, "CFArrayCreate")
			if cgWindowLevelForKey == nil {
				purego.RegisterLibFunc(&cgWindowLevelForKey, cg, "CGWindowLevelForKey")
			}
			cgPopupMenuLevel = cgWindowLevelForKey(11) // kCGPopUpMenuWindowLevelKey
		}
	}
}

// Scroll is a trackpad gesture: it begins, moves 20 points at most every
// 16 ms, and ends without momentum.
func (t *testSurface) Scroll(at driver.Point, delta driver.Point) error {
	t.follow()
	loadCG()
	const pixel, scrollPhase, phaseBegan, phaseChanged, phaseEnded = 0, 99, 1, 2, 4
	// A trackpad's deltas are continuous, in points.
	const continuous = 88
	n := int(max(abs(delta.X), abs(delta.Y))/20) + 1
	// The gesture finds its view with AppKit's geometry, as the window's monitor has it do. A trackpad scrolls under the pointer, so the hover pick after the scroll is made there too.
	onMainSync(func() {
		if t.s.testPointer != nil {
			*t.s.testPointer = t.windowPoint(at)
		}
		t.s.commitPending()
	})
	send := func(phase int64, dx, dy driver.Points) {
		onMainSync(func() {
			// The wheel's deltas are the content's way: positive shows what
			// is above or left.
			ev := cgEventCreateScrollWheelEvent2(0, pixel, 2, int32(-dy), int32(-dx), 0)
			defer apple.Release(ev)
			cgEventSetIntegerValueField(ev, scrollPhase, phase)
			cgEventSetIntegerValueField(ev, continuous, 1)
			cgEventSetLocation(ev, t.screenPoint(at))
			ns := objc.ID(apple.Class("NSEvent")).Send(sel("eventWithCGEvent:"), ev)
			rememberTestInput(ns)
			// AppKit hands wheels to the window under the system's pointer,
			// which a test leaves where the user has it: the view under at
			// takes it, and passes it up as a real one.
			if hit := t.hit(at); hit != 0 {
				hit.Send(sel("scrollWheel:"), ns)
			}
		})
		time.Sleep(16 * time.Millisecond)
	}
	send(phaseBegan, 0, 0)
	for range n {
		send(phaseChanged, delta.X/driver.Points(n), delta.Y/driver.Points(n))
	}
	send(phaseEnded, 0, 0)
	return nil
}

// screenPoint is p in the global display coordinates CGEvents use: from
// the main screen's top left.
func (t *testSurface) screenPoint(p driver.Point) apple.CGPoint {
	wp := t.windowPoint(p)
	sp := objc.Send[apple.CGPoint](t.s.window, sel("convertPointToScreen:"), wp)
	screens := objc.ID(apple.Class("NSScreen")).Send(sel("screens"))
	main := screens.Send(sel("objectAtIndex:"), uint(0))
	h := objc.Send[apple.CGRect](main, sel("frame")).Size.Height
	return apple.CGPoint{X: sp.X, Y: h - sp.Y}
}

// hit is the view at at, as the window's hit testing finds it.
func (t *testSurface) hit(at driver.Point) objc.ID {
	root := t.s.native
	wp := t.windowPoint(at)
	return root.Send(sel("hitTest:"), objc.Send[apple.CGPoint](root.Send(sel("superview")), sel("convertPoint:fromView:"), wp, objc.ID(0)))
}

// Hover moves the mouse over at: posted moves reach only the first
// responder, so the views under it hear what their tracking areas would
// report.
func (t *testSurface) Hover(at driver.Point) error {
	t.follow()
	onMainSync(func() {
		if t.s.testPointer != nil {
			*t.s.testPointer = t.windowPoint(at)
		}
		t.s.commitPending()
		var under []objc.ID
		root := t.s.native
		if at.X >= 0 && at.Y >= 0 {
			for n := t.hit(at); n != 0 && n != root; n = n.Send(selSuperview) {
				if v := views[n]; v != nil && v.native == n {
					under = append(under, n)
				}
			}
		}
		in := func(list []objc.ID, n objc.ID) bool {
			for _, o := range list {
				if o == n {
					return true
				}
			}
			return false
		}
		for _, n := range t.hovered {
			// A view the app removed since is gone with its events.
			if v := views[n]; v != nil && v.native == n && !in(under, n) {
				n.Send(sel("mouseExited:"), objc.ID(0))
			}
		}
		for i := len(under) - 1; i >= 0; i-- {
			if !in(t.hovered, under[i]) {
				under[i].Send(sel("mouseEntered:"), objc.ID(0))
			}
		}
		t.hovered = under
		if len(under) > 0 {
			ev := objc.ID(apple.Class("NSEvent")).Send(
				sel("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
				uint(evMoved), t.windowPoint(at), uint(0), uptime(), objc.Send[int](t.s.window, sel("windowNumber")), objc.ID(0), 0, 0, float32(0))
			rememberTestInput(ev)
			for _, n := range under {
				n.Send(sel("mouseMoved:"), ev)
			}
		}
	})
	return nil
}

// editActions are the Edit menu's items by their key, which Command
// presses.
var editActions = map[string]string{"a": "selectAll:", "c": "copy:", "v": "paste:", "x": "cut:", "z": "undo:"}

// testModifiers are the left modifier keys' codes and the flags each sets, its device-dependent one included, by the names tests press them alone with.
var testModifiers = map[string]struct {
	code  uint16
	flags uint
}{
	"shift": {56, flagShift | 0x02}, "ctrl": {59, flagControl | 0x01}, "control": {59, flagControl | 0x01},
	"alt": {58, flagOption | 0x20}, "option": {58, flagOption | 0x20},
	"mod": {55, flagCommand | 0x08}, "meta": {55, flagCommand | 0x08}, "cmd": {55, flagCommand | 0x08}, "command": {55, flagCommand | 0x08},
}

// modifier sends a modifier key going down or coming up alone, as AppKit reports it: flagsChanged with flags, the modifiers held once it changed, its own set as it goes down and cleared as it comes up.
func (t *testSurface) modifier(code uint16, flags uint) {
	const evFlagsChanged = 12
	onMainSync(func() {
		if t.s.closed {
			// What it did closed the window, as a key or a press on Close does: its next event has nowhere to go.
			return
		}
		ev := apple.KeyEvent(evFlagsChanged, flags, uptime(), objc.Send[int](t.s.window, sel("windowNumber")), "", "", false, code)
		rememberTestInput(ev)
		if objc.Send[bool](t.s.window, sel("isKeyWindow")) {
			nsApp().Send(sel("postEvent:atStart:"), ev, false)
		} else if ev = t.s.filter(ev); ev != 0 {
			if r := t.s.window.Send(sel("firstResponder")); r != 0 {
				r.Send(sel("flagsChanged:"), ev)
			}
		}
	})
	time.Sleep(10 * time.Millisecond)
}

// key sends a key event of kind, evKeyDown or evKeyUp, with chars, ignoring is what it types without modifiers, and repeat set for a down that is the auto-repeat of a key held. A background window receives its keys directly: the filter reports its down or up, and the responder then hears it.
func (t *testSurface) key(kind uint, code uint16, flags uint, chars, ignoring string, repeat bool) {
	onMainSync(func() {
		if t.s.closed {
			// What it did closed the window, as a key or a press on Close does: its next event has nowhere to go.
			return
		}
		ev := apple.KeyEvent(kind, flags, uptime(), objc.Send[int](t.s.window, sel("windowNumber")), chars, ignoring, repeat, code)
		rememberTestInput(ev)
		if objc.Send[bool](t.s.window, sel("isKeyWindow")) {
			nsApp().Send(sel("postEvent:atStart:"), ev, false)
		} else if ev = t.s.filter(ev); ev != 0 {
			// A background app can have no key window. Its monitors still see queued keys, but AppKit never hands them to the responder: run the same filter and deliver to that responder, without activating the app.
			r := t.s.window.Send(sel("firstResponder"))
			switch {
			case kind == evKeyDown && flags&(flagCommand|flagControl) != 0 && t.keyEquivalent(ev):
				// A view or the menu bar took it.
			case code == 48 && kind == evKeyDown && (r == t.s.window || r == t.s.native || r == 0):
				t.s.tabFromSurface(flags&flagShift != 0)
			case kind == evKeyUp:
				r.Send(sel("keyUp:"), ev)
			default:
				r.Send(sel("keyDown:"), ev)
			}
		}
	})
	time.Sleep(10 * time.Millisecond)
}

// keyEquivalent offers a background window's key down to its views and then the menu bar, as AppKit's own dispatch does before the responder sees the key, and reports whether one took it.
func (t *testSurface) keyEquivalent(ev objc.ID) bool {
	if objc.Send[bool](t.s.window, sel("performKeyEquivalent:"), ev) {
		return true
	}
	// An item whose action has no target, as a role's Close, goes to the key window, which a background app has none of: the menu bar would take the key and send the action nowhere.
	menu := nsApp().Send(sel("mainMenu"))
	if item := keyItem(menu, apple.GoString(ev.Send(sel("charactersIgnoringModifiers"))), objc.Send[uint](ev, sel("modifierFlags"))); item != 0 && t.performUntargeted(item) {
		return true
	}
	return objc.Send[bool](menu, sel("performKeyEquivalent:"), ev)
}

// keyItem returns the item of menu or its submenus whose key equivalent is key with the modifiers in flags, 0 for none.
func keyItem(menu objc.ID, key string, flags uint) objc.ID {
	if menu == 0 || key == "" {
		return 0
	}
	const modifiers = flagCommand | flagControl | flagOption | flagShift
	for i := range objc.Send[int](menu, sel("numberOfItems")) {
		it := menu.Send(sel("itemAtIndex:"), i)
		if sub := it.Send(sel("submenu")); sub != 0 {
			if found := keyItem(sub, key, flags); found != 0 {
				return found
			}
			continue
		}
		if strings.EqualFold(apple.GoString(it.Send(sel("keyEquivalent"))), key) && objc.Send[uint](it, sel("keyEquivalentModifierMask"))&modifiers == flags&modifiers {
			return it
		}
	}
	return 0
}

// performUntargeted sends item's action, which has no target, down the window's responder chain, as AppKit sends it down the key window's, and reports whether a responder took it: a background app has no key window for AppKit to send it to.
func (t *testSurface) performUntargeted(item objc.ID) bool {
	action := objc.Send[objc.SEL](item, sel("action"))
	if action == 0 || item.Send(sel("target")) != 0 || t.s.window == 0 {
		return false
	}
	r := t.s.window.Send(sel("firstResponder"))
	if r == 0 {
		r = t.s.window
	}
	return objc.Send[bool](r, sel("tryToPerform:with:"), action, item)
}

// activates reports whether the run opted in to activating the test app for the keyboard (nsgo test -target macos -activate). Live surfaces never activate here.
func (t *testSurface) activates() bool {
	return t.test != nil && os.Getenv("NSGO_TEST_ACTIVATE") == "1"
}

// keyboard readies the window for a key step. A live surface's window becomes key within the app, without activating it, as the test host gives a test's window as it opens. A test's window is activated only with the run's opt-in; otherwise key delivers to it in the background.
func (t *testSurface) keyboard() error {
	if t.test != nil && (!t.fixed || t.activates()) {
		return t.PrepareKeyboard()
	}
	onMainSync(func() {
		if w := t.s.window; w != 0 && !objc.Send[bool](w, sel("isKeyWindow")) {
			w.Send(sel("makeKeyWindow"))
		}
	})
	return nil
}

// PrepareKeyboard implements testhost.KeyboardSurface. With the run's opt-in it activates the app and waits up to 2 seconds for it to be active with its window key: AppKit activates asynchronously, so the wait polls from the test goroutine rather than blocking the main thread. Without the opt-in it leaves the user's active app alone and does nothing.
func (t *testSurface) PrepareKeyboard() error {
	t.follow()
	if !t.activates() {
		return nil
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		var active, key bool
		onMainSync(func() {
			active = objc.Send[bool](nsApp(), sel("isActive"))
			if !active {
				activateApp()
			}
			w := t.s.window
			if !objc.Send[bool](w, sel("isKeyWindow")) {
				w.Send(sel("makeKeyWindow"))
			}
			key = objc.Send[bool](w, sel("isKeyWindow"))
		})
		if active && key {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("the macOS test app could not activate and make its window key within 2s (active=%v, key=%v); -activate needs an unlocked graphical session", active, key)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// FocusRefusal implements testhost.KeyboardSurface: after a click left a field without the focus, it names an inactive app as the cause when the run did not opt in to activating it.
func (t *testSurface) FocusRefusal() error {
	t.follow()
	if t.test == nil || t.activates() {
		return nil
	}
	var active bool
	onMainSync(func() { active = objc.Send[bool](nsApp(), sel("isActive")) })
	if active {
		return nil
	}
	return errors.New("the macOS test app is not active, and AppKit gave the clicked field no keyboard focus: rerun with -target macos -activate to let the test app take the keyboard")
}

func (t *testSurface) Type(text string) error {
	t.follow()
	if err := t.keyboard(); err != nil {
		return err
	}
	for _, r := range text {
		name := strings.ToLower(string(r))
		code, flags := keyCodes[name], uint(0)
		if r == '\n' {
			code, name = 36, "\r"
		}
		if r >= 'A' && r <= 'Z' {
			flags = flagShift
		}
		t.key(evKeyDown, code, flags, string(r), name, false)
		t.key(evKeyUp, code, flags, string(r), name, false)
	}
	return nil
}

// Compose marks text in the first responder as an input method does,
// through NSTextInputClient: the field's editor shows it marked and
// reports it, as it does a keyboard's composition.
func (t *testSurface) Compose(text string) error {
	t.follow()
	if err := t.keyboard(); err != nil {
		return err
	}
	var err error
	onMainSync(func() {
		r := t.s.window.Send(sel("firstResponder"))
		if r == 0 || !objc.Send[bool](r, sel("respondsToSelector:"), sel("setMarkedText:selectedRange:replacementRange:")) {
			err = errors.New("no text input client has the keyboard")
			return
		}
		n := uint(len(utf16.Encode([]rune(text))))
		r.Send(sel("setMarkedText:selectedRange:replacementRange:"), apple.NSString(text), apple.NSRange{Location: n}, apple.NSRange{Location: nsNotFound})
		if text == "" {
			r.Send(sel("unmarkText"))
		}
	})
	time.Sleep(10 * time.Millisecond)
	return err
}

// Commit inserts text over what is marked, which ends the composition, as
// an input method commits.
func (t *testSurface) Commit(text string) error {
	t.follow()
	if err := t.keyboard(); err != nil {
		return err
	}
	var err error
	onMainSync(func() {
		r := t.s.window.Send(sel("firstResponder"))
		if r == 0 || !objc.Send[bool](r, sel("respondsToSelector:"), sel("insertText:replacementRange:")) {
			err = errors.New("no text input client has the keyboard")
			return
		}
		r.Send(sel("insertText:replacementRange:"), apple.NSString(text), apple.NSRange{Location: nsNotFound})
	})
	time.Sleep(10 * time.Millisecond)
	return err
}

func (t *testSurface) Key(key string) error {
	t.follow()
	if err := t.keyboard(); err != nil {
		return err
	}
	parts := strings.Split(strings.ToLower(key), "+")
	name := parts[len(parts)-1]
	var flags uint
	for _, m := range parts[:len(parts)-1] {
		switch m {
		case "mod", "cmd", "meta", "command":
			flags |= flagCommand
		case "ctrl", "control":
			flags |= flagControl
		case "alt", "option":
			flags |= flagOption
		case "shift":
			flags |= flagShift
		default:
			return fmt.Errorf("unknown modifier %q in %q", m, key)
		}
	}
	k, err := testKeyNamed(name, key)
	if err != nil {
		return err
	}
	flags |= t.heldFlags
	if k.modifier != 0 {
		t.modifier(k.code, flags|k.modifier)
		t.modifier(k.code, flags)
		return nil
	}
	if action := editActions[name]; flags&(flagCommand|flagControl|flagOption|flagShift) == flagCommand && action != "" {
		// The menu's key equivalents need the app active, which a test
		// leaves to the app the user works in: the field does the edit the
		// menu item would ask of it.
		onMainSync(func() {
			ev := apple.KeyEvent(evKeyDown, flags, uptime(), objc.Send[int](t.s.window, sel("windowNumber")), name, name, false, k.code)
			rememberTestInput(ev)
			if t.s.filter(ev) == 0 {
				return
			}
			if r := t.s.window.Send(sel("firstResponder")); r != 0 {
				r.Send(sel("tryToPerform:with:"), sel(action), objc.ID(0))
			}
		})
		// Its up still comes, so a down the surface reported comes up.
		onMainSync(func() {
			if t.s.closed {
				return
			}
			ev := apple.KeyEvent(evKeyUp, flags, uptime(), objc.Send[int](t.s.window, sel("windowNumber")), name, name, false, k.code)
			rememberTestInput(ev)
			t.s.filter(ev)
		})
		return nil
	}
	t.key(evKeyDown, k.code, flags, k.chars, k.chars, false)
	t.key(evKeyUp, k.code, flags, k.chars, k.chars, false)
	return nil
}

// KeyDown implements testhost.KeyHoldSurface: key goes down and stays held, a modifier's flags joining every key after it until its KeyUp.
func (t *testSurface) KeyDown(key string) error { return t.hold(key, true) }

// KeyUp implements testhost.KeyHoldSurface: key, which KeyDown held, comes up.
func (t *testSurface) KeyUp(key string) error { return t.hold(key, false) }

// hold sends key, one key and not a chord, going down or coming up, with the modifiers KeyDown holds.
func (t *testSurface) hold(key string, down bool) error {
	t.follow()
	if err := t.keyboard(); err != nil {
		return err
	}
	name := strings.ToLower(key)
	if strings.Contains(name, "+") {
		return fmt.Errorf("%q is a chord: a key goes down or comes up alone", key)
	}
	k, err := testKeyNamed(name, key)
	if err != nil {
		return err
	}
	t.holdKey(k, down)
	return nil
}

// KeyCodeDown implements testhost.KeyCodeSurface: the physical position code goes down and stays held, as hold holds a key.
func (t *testSurface) KeyCodeDown(code, name string) error { return t.holdCode(code, name, true) }

// KeyCodeUp implements testhost.KeyCodeSurface: the position code, which KeyCodeDown held, comes up.
func (t *testSurface) KeyCodeUp(code, name string) error { return t.holdCode(code, name, false) }

// holdCode is hold for a physical position: AppKit's key code is the position's, with the characters the current keyboard layout types there, as a real keyboard's key does, or else those of name, the US layout's key there. A modifier's position holds its own side.
func (t *testSurface) holdCode(code, name string, down bool) error {
	t.follow()
	if err := t.keyboard(); err != nil {
		return err
	}
	pc, ok := driver.KeyPositionCodes(code)
	if !ok || pc.Mac == 0xffff {
		return fmt.Errorf("a Mac keyboard has no %s key", code)
	}
	if _, ok := modifierKeys[pc.Mac]; ok {
		t.holdModifier(pc.Mac, down, true)
		return nil
	}
	k, err := testKeyNamed(strings.ToLower(name), name)
	if err != nil {
		return err
	}
	k.code = pc.Mac
	if _, named := keyNames[pc.Mac]; !named {
		if chars := currentLayoutChars(pc.Mac); chars != "" {
			k.chars = chars
		}
	}
	t.holdKey(k, down)
	return nil
}

// modifierFlags are the device-independent flag of each modifier, by its name, which either of its keys holds.
var modifierFlags = map[string]uint{"shift": flagShift, "ctrl": flagControl, "alt": flagOption, "meta": flagCommand}

// holdModifier has KeyDown (byPosition false) or KeyCodeDown hold the modifier key code, or let it go, and sends the key going down or coming up where that changes whether the key is held: a key KeyDown and KeyCodeDown both hold comes up once both let it go. The modifier's flags stay held while any of its keys is.
func (t *testSurface) holdModifier(code uint16, down, byPosition bool) {
	h := t.heldModifiers[code]
	was := h.byName || h.byPosition
	if byPosition {
		h.byPosition = down
	} else {
		h.byName = down
	}
	if t.heldModifiers == nil {
		t.heldModifiers = map[uint16]struct{ byName, byPosition bool }{}
	}
	if h.byName || h.byPosition {
		t.heldModifiers[code] = h
	} else {
		delete(t.heldModifiers, code)
	}
	t.heldFlags = 0
	for held := range t.heldModifiers {
		m := modifierKeys[held]
		t.heldFlags |= modifierFlags[m.name] | m.flag
	}
	if was != (h.byName || h.byPosition) {
		t.modifier(code, t.heldFlags)
	}
}

// holdKey sends k, a key KeyDown names, going down or coming up, with the modifiers KeyDown and KeyCodeDown hold.
func (t *testSurface) holdKey(k testKey, down bool) {
	if k.modifier != 0 {
		t.holdModifier(k.code, down, false)
		return
	}
	if !down {
		delete(t.heldCodes, k.code)
		t.key(evKeyUp, k.code, t.heldFlags, k.chars, k.chars, false)
		return
	}
	// A key already held goes down again as its auto-repeat, as AppKit sends a key held down.
	repeat := t.heldCodes[k.code]
	if t.heldCodes == nil {
		t.heldCodes = map[uint16]bool{}
	}
	t.heldCodes[k.code] = true
	t.key(evKeyDown, k.code, t.heldFlags, k.chars, k.chars, repeat)
}

// testKey is a key a test names, as AppKit sends it: its key code, the flags it sets while held for a modifier key (0 for another key), and the characters it types without modifiers.
type testKey struct {
	code     uint16
	modifier uint
	chars    string
}

// testKeyNamed returns the key named name, lowercase without modifiers, of the step's key.
func testKeyNamed(name, key string) (testKey, error) {
	if m, ok := testModifiers[name]; ok {
		return testKey{code: m.code, modifier: m.flags}, nil
	}
	c, ok := keyCodes[name]
	if !ok {
		return testKey{}, fmt.Errorf("unknown key %q", key)
	}
	chars, special := keyChars[name]
	switch {
	case !special:
		chars = name
	case chars == "":
		// A key that types nothing carries AppKit's function key character, which a text field reads as the arrow or the Home it is; its name would be typed as text.
		chars = string(keyEquivalents[name])
	}
	return testKey{code: c, chars: chars}, nil
}

// Back is Escape, twice in a field, whose first only leaves it.
func (t *testSurface) Back() error {
	t.follow()
	var editing bool
	onMainSync(func() {
		r := t.s.window.Send(sel("firstResponder"))
		editing = r != 0 && objc.Send[bool](r, sel("isKindOfClass:"), objc.ID(apple.Class("NSText")))
	})
	if editing {
		t.Key("escape")
	}
	return t.Key("escape")
}

func (t *testSurface) Answer(id uint32, index int) error {
	t.follow()
	var err error
	onMainSync(func() {
		s := t.s
		shown, ok := s.alerts[id]
		if !ok && t.own != nil && s != t.own {
			// One the app's own surface shows, as from the app's function: each surface numbers its alerts itself, and the main window's come first.
			s = t.own
			shown, ok = s.alerts[id]
		}
		if !ok {
			err = fmt.Errorf("no alert %d shows", id)
			return
		}
		window := shown.alert.Send(sel("window"))
		parent := window.Send(sel("sheetParent"))
		if parent == 0 && nsApp().Send(sel("modalWindow")) != window {
			err = fmt.Errorf("alert %d is waiting to show", id)
			return
		}
		if index < 0 {
			if parent != 0 {
				parent.Send(sel("endSheet:returnCode:"), window, 0)
			} else if nsApp().Send(sel("modalWindow")) == window {
				nsApp().Send(sel("abortModal"))
			}
			return
		}
		for i, action := range shown.order {
			if action == index {
				buttons := shown.alert.Send(sel("buttons"))
				buttons.Send(sel("objectAtIndex:"), uint(i)).Send(sel("performClick:"), objc.ID(0))
				return
			}
		}
		err = fmt.Errorf("the alert has no button %d", index)
	})
	return err
}

// compositedAttempt captures the surface's window and any native popup it is
// tracking, clipped to the view's bounds. WindowServer keeps the Core
// Animation effects (shadows, rings, masks, filters, presentation layers) that
// an AppKit cacheDisplayInRect: redraw omits. Returns retry=true when the
// window is visible and not minimized but WindowServer has not finished
// compositing a full frame yet. Must be called on the main thread.
func (t *testSurface) compositedAttempt(bounds apple.CGRect) (data []byte, retry bool) {
	w := t.s.window
	if w == 0 || bounds.Size.Width <= 0 || bounds.Size.Height <= 0 ||
		!objc.Send[bool](w, sel("isVisible")) || objc.Send[bool](w, sel("isMiniaturized")) {
		return nil, false
	}
	// Flush pending AppKit display and Core Animation layer commits so
	// WindowServer's composite reflects the settled scene.
	t.s.commitPending()
	w.Send(sel("displayIfNeeded"))
	objc.ID(apple.Class("CATransaction")).Send(sel("flush"))
	number := objc.Send[int](w, sel("windowNumber"))
	if number <= 0 {
		return nil, true
	}
	if loadCG(); cgWindowListCreateImage == nil && cgWindowListCreateImageFromArray == nil {
		return nil, false
	}
	// screenPoint is in the global top-left coordinates CGWindowListCreateImage takes.
	origin := t.screenPoint(driver.Point{})
	const (
		windowListIncludingWindow    = 1 << 3
		windowImageBoundsIgnoreFrame = 1 << 0
		windowImageBestResolution    = 1 << 3
	)
	rect := apple.Rect(origin.X, origin.Y, bounds.Size.Width, bounds.Size.Height)
	const imageOptions = windowImageBoundsIgnoreFrame | windowImageBestResolution
	image := t.popupImage(rect, uint32(number), imageOptions)
	if image == 0 && cgWindowListCreateImage != nil {
		image = cgWindowListCreateImage(rect, windowListIncludingWindow, uint32(number), imageOptions)
	}
	if image == 0 {
		return nil, true
	}
	defer apple.Release(image)
	scale := objc.Send[float64](w, sel("backingScaleFactor"))
	width, height := apple.ImageSize(image)
	if float64(width) < math.Floor(bounds.Size.Width*scale) || float64(height) < math.Floor(bounds.Size.Height*scale) {
		return nil, true
	}
	rep := objc.ID(apple.Class("NSBitmapImageRep")).Send(sel("alloc")).Send(sel("initWithCGImage:"), objc.ID(image))
	if rep == 0 {
		data, _ = apple.PNGImage(image)
		return data, data == nil
	}
	defer rep.Send(selRelease)
	// A freshly ordered opaque window whose backing store WindowServer has not
	// composited yet can return a full-sized all-transparent (zero) image.
	if objc.Send[bool](w, sel("isOpaque")) && bitmapRepIsAllZero(rep) {
		return nil, true
	}
	data, _ = pngFromBitmapRep(rep)
	return data, data == nil
}

// bitmapRepIsAllZero reports whether rep is an alpha-bearing interleaved bitmap
// whose pixels are all zero (fully transparent). Any composited opaque window
// has non-zero alpha on its very first pixel and returns false immediately.
func bitmapRepIsAllZero(rep objc.ID) bool {
	if !objc.Send[bool](rep, sel("hasAlpha")) || objc.Send[bool](rep, sel("isPlanar")) {
		return false
	}
	w := objc.Send[int](rep, sel("pixelsWide"))
	h := objc.Send[int](rep, sel("pixelsHigh"))
	bpr := objc.Send[int](rep, sel("bytesPerRow"))
	bpp := objc.Send[int](rep, sel("bitsPerPixel"))
	ptr := objc.Send[*byte](rep, sel("bitmapData"))
	if w <= 0 || h <= 0 || bpr <= 0 || bpp <= 0 || bpp%8 != 0 || ptr == nil {
		return false
	}
	rowBytes := min(w*(bpp/8), bpr)
	buf := unsafe.Slice(ptr, h*bpr)
	for y := range h {
		for _, b := range buf[y*bpr : y*bpr+rowBytes] {
			if b != 0 {
				return false
			}
		}
	}
	return true
}

// compositedSC captures the window through ScreenCaptureKit's current-process
// API (macOS 14.4+, no Screen Recording permission required) when
// CGWindowListCreateImage is unavailable or did not produce a composite.
func (t *testSurface) compositedSC() []byte {
	ch := make(chan []byte, 1)
	var minW, minH int
	var opaque, started bool
	onMainSync(func() {
		v, w := t.s.native, t.s.window
		if v == 0 || w == 0 || (t.s.popup != 0 && t.s.tracking) ||
			!objc.Send[bool](w, sel("isVisible")) || objc.Send[bool](w, sel("isMiniaturized")) {
			return
		}
		bounds := objc.Send[apple.CGRect](v, selBounds)
		if bounds.Size.Width <= 0 || bounds.Size.Height <= 0 {
			return
		}
		number := objc.Send[int](w, sel("windowNumber"))
		if number <= 0 {
			return
		}
		r := objc.Send[apple.CGRect](v, sel("convertRect:toView:"), bounds, objc.ID(0))
		region := objc.Send[apple.CGRect](w, sel("convertRectToScreen:"), r)
		scale := objc.Send[float64](w, sel("backingScaleFactor"))
		frame := objc.Send[apple.CGRect](w, sel("frame"))
		minW = int(math.Floor(bounds.Size.Width * scale))
		minH = int(math.Floor(bounds.Size.Height * scale))
		opaque = objc.Send[bool](w, sel("isOpaque"))
		loadCapture()
		started = captureOwnWindow(uint32(number), frame, region, scale, func(data []byte, err error) {
			if err != nil {
				ch <- nil
				return
			}
			ch <- data
		})
	})
	if !started {
		return nil
	}
	select {
	case data := <-ch:
		if len(data) == 0 {
			return nil
		}
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width < minW || cfg.Height < minH {
			return nil
		}
		if opaque {
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil || imageIsAllTransparent(img) {
				return nil
			}
		}
		return data
	case <-time.After(time.Second):
		return nil
	}
}

func imageIsAllTransparent(img image.Image) bool {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			_, _, _, a := img.At(x, y).RGBA()
			if a != 0 {
				return false
			}
		}
	}
	return true
}

// popupImage uses only explicit window IDs: other apps' windows must never
// appear in the capture, even if they overlap the window or its menu.
func (t *testSurface) popupImage(bounds apple.CGRect, window, options uint32) uintptr {
	if t.s.popup == 0 || !t.s.tracking || cgWindowListCreateImageFromArray == nil || !loadWindowList() {
		return 0
	}
	const windowListAboveWindow = 1 << 1
	list := cgWindowListCopyWindowInfo(windowListAboveWindow, window)
	if list == 0 {
		return 0
	}
	defer apple.Release(uintptr(list))
	ownerKey := apple.NSString("kCGWindowOwnerPID")
	layerKey := apple.NSString("kCGWindowLayer")
	numberKey := apple.NSString("kCGWindowNumber")
	pid := os.Getpid()
	var ids [8]uintptr
	windows := ids[:0]
	// The WindowServer list is front to back, so the submenus precede
	// their parent menu and the test window goes last.
	for i := range objc.Send[uint](list, sel("count")) {
		info := list.Send(sel("objectAtIndex:"), i)
		if objc.Send[int](info.Send(sel("objectForKey:"), ownerKey), sel("integerValue")) != pid ||
			objc.Send[int32](info.Send(sel("objectForKey:"), layerKey), sel("intValue")) != cgPopupMenuLevel {
			continue
		}
		if id := objc.Send[uint32](info.Send(sel("objectForKey:"), numberKey), sel("unsignedIntValue")); id != 0 {
			windows = append(windows, uintptr(id))
		}
	}
	if len(windows) == 0 {
		return 0
	}
	windows = append(windows, uintptr(window))
	// CGWindowIDs are packed into a CFArray as pointer-sized integers,
	// not as CFNumbers. A nil callback table keeps them as raw values.
	array := cfWindowIDArrayCreate(0, &windows[0], len(windows), 0)
	if array == 0 {
		return 0
	}
	defer apple.Release(array)
	return cgWindowListCreateImageFromArray(bounds, array, options)
}

// pngFromBitmapRep encodes rep as a PNG in sRGB, converting it from the window's color space, which both the WindowServer composite and the AppKit fallback carry.
func pngFromBitmapRep(rep objc.ID) ([]byte, error) {
	if rep == 0 {
		return nil, errors.New("macos: empty bitmap image")
	}
	converted := rep
	space := objc.ID(apple.Class("NSColorSpace")).Send(sel("sRGBColorSpace"))
	if space != 0 {
		if candidate := rep.Send(sel("bitmapImageRepByConvertingToColorSpace:renderingIntent:"), space, uint(0)); candidate != 0 {
			converted = candidate
		}
	}
	const pngType = 4
	data := converted.Send(sel("representationUsingType:properties:"), uint(pngType),
		objc.ID(apple.Class("NSDictionary")).Send(sel("dictionary")))
	if data == 0 {
		return nil, errors.New("macos: cannot encode bitmap image")
	}
	return apple.Bytes(data), nil
}

// Screenshot implements testhost.Surface.
func (t *testSurface) Screenshot() ([]byte, error) {
	res, err := t.ScreenshotDetailed()
	return res.PNG, err
}

// ScreenshotDetailed implements testhost.DetailedScreenshotSurface, returning
// WindowServer's composited frame when available and flagging when it had to
// fall back to AppKit's cacheDisplayInRect: redraw.
func (t *testSurface) ScreenshotDetailed() (testhost.ScreenshotResult, error) {
	t.follow()
	const (
		compositeWait = 500 * time.Millisecond
		compositeStep = 20 * time.Millisecond
	)
	var pngData []byte
	var trySC bool
	deadline := time.Now().Add(compositeWait)
	for {
		var retry bool
		onMainSync(func() {
			v := t.s.native
			if v == 0 {
				return
			}
			bounds := objc.Send[apple.CGRect](v, selBounds)
			pngData, retry = t.compositedAttempt(bounds)
			if pngData == nil {
				w := t.s.window
				trySC = w != 0 && bounds.Size.Width > 0 && bounds.Size.Height > 0 &&
					objc.Send[bool](w, sel("isVisible")) && !objc.Send[bool](w, sel("isMiniaturized"))
			}
		})
		if pngData != nil {
			return testhost.ScreenshotResult{PNG: pngData}, nil
		}
		if !retry || time.Now().After(deadline) {
			break
		}
		time.Sleep(compositeStep)
	}
	if trySC {
		if pngData = t.compositedSC(); pngData != nil {
			return testhost.ScreenshotResult{PNG: pngData}, nil
		}
	}
	onMainSync(func() {
		pngData = t.fallbackScreenshot()
	})
	if pngData == nil {
		return testhost.ScreenshotResult{}, errors.New("the window drew nothing")
	}
	return testhost.ScreenshotResult{PNG: pngData, Fallback: true}, nil
}

// fallbackScreenshot redraws the surface through AppKit's cacheDisplayInRect:
// when WindowServer's composite is unavailable. Must be called on the main thread.
func (t *testSurface) fallbackScreenshot() []byte {
	v := t.s.native
	if v == 0 {
		return nil
	}
	bounds := objc.Send[apple.CGRect](v, selBounds)
	screenshotFallbackOnce.Do(func() {
		log.Printf("nsgo: macOS composited screenshot unavailable; falling back to cacheDisplayInRect: (shadows, animations and visual effects may be missing)")
	})
	rep := v.Send(sel("bitmapImageRepForCachingDisplayInRect:"), bounds)
	if rep == 0 {
		return nil
	}
	v.Send(sel("cacheDisplayInRect:toBitmapImageRep:"), bounds, rep)
	pngData, _ := pngFromBitmapRep(rep)
	return pngData
}

func (t *testSurface) Sync() { t.s.engine.Sync() }

func (t *testSurface) Animating() bool {
	t.follow()
	var busy bool
	onMainSync(func() {
		// Batches waiting for a frame have not shown yet, nor scrolls
		// their offsets.
		busy = t.s.animator.Ending() || len(t.s.motions) > 0 || len(t.s.transitions) > 0 || len(t.s.ghosts) > 0 || t.s.held() || t.s.scrollsMoving() || t.s.geometryEnding()
		// A context menu can consume queued events in its tracking loop, while the step must return so the test can choose the menu item.
		if t.s.tracking {
			t.pressID = 0
		}
		busy = busy || t.pressID > pressHandled
	})
	return busy
}

func (t *testSurface) Resize(size driver.Size) error {
	t.follow()
	onMainSync(func() {
		t.s.window.Send(sel("setContentSize:"), apple.CGSize{Width: float64(size.Width), Height: float64(size.Height)})
	})
	return nil
}

func (t *testSurface) SetEnvironment(env driver.Environment) error {
	t.follow()
	if t.test == nil {
		return errors.New("a running app's window follows the system's settings")
	}
	t.test.pin.Set(&env)
	t.own.engine.Repin()
	// The windows the app opened report their last environment again, which the new pin applies over, as Repin does for the own surface.
	onMainSync(func() {
		for s := range surfaces {
			if s.test == t.test && s != t.own && s.engine != nil && !s.closed {
				s.engine.SetEnvironment(s.env)
			}
		}
	})
	return nil
}

// menuItem returns the item tagged id in menu or its submenus, and the
// menu it is in.
func menuItem(menu objc.ID, id uint32) (item, in objc.ID) {
	if menu == 0 {
		return 0, 0
	}
	n := objc.Send[int](menu, sel("numberOfItems"))
	for i := range n {
		it := menu.Send(sel("itemAtIndex:"), i)
		if objc.Send[int](it, sel("tag")) == int(id) && it.Send(sel("action")) != 0 {
			return it, menu
		}
		if sub := it.Send(sel("submenu")); sub != 0 {
			if found, at := menuItem(sub, id); found != 0 {
				return found, at
			}
		}
	}
	return 0, 0
}

// ChooseMenu implements testhost.Surface: the item of the surface's menu sends its action, as a click on it does. Each surface numbers its menu's items itself, so the menu bar must show the surface's menu, or the default one where the surface set none.
func (t *testSurface) ChooseMenu(id uint32) error {
	t.follow()
	var err error
	onMainSync(func() {
		main := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication")).Send(sel("mainMenu"))
		if t.s.menu != 0 && main != t.s.menu {
			err = errors.New("the menu bar shows another window's menu")
			return
		}
		item, menu := menuItem(main, id)
		if item == 0 {
			err = fmt.Errorf("the main menu has no item %d", id)
			return
		}
		menu.Send(sel("performActionForItemAtIndex:"), objc.Send[int](menu, sel("indexOfItem:"), item))
	})
	return err
}

// ChooseDockMenu implements testhost.Surface: the item of the Dock menu, as the app delegate hands it to AppKit where it is the app's, sends its action, as a click on it does.
func (t *testSurface) ChooseDockMenu(id uint32) error {
	t.follow()
	var err error
	onMainSync(func() {
		menu := dockMenu.menu
		app := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
		if d, dock := app.Send(sel("delegate")), sel("applicationDockMenu:"); d != 0 && objc.Send[bool](d, sel("respondsToSelector:"), dock) {
			menu = d.Send(dock, app)
		}
		item, in := menuItem(menu, id)
		if item == 0 {
			err = fmt.Errorf("the Dock menu has no item %d", id)
			return
		}
		in.Send(sel("performActionForItemAtIndex:"), objc.Send[int](in, sel("indexOfItem:"), item))
	})
	return err
}

// ChoosePopup implements testhost.Surface: the tracking context menu's item sends its action and the menu closes, as a click on it does. For 0 the menu showing or about to show is cancelled, and it returns once AppKit's tracking loop has ended, so the next input reaches a window; with none showing it does nothing.
func (t *testSurface) ChoosePopup(id uint32) error {
	t.follow()
	if id == 0 {
		tracking := true
		onMainSync(func() {
			t.s.closePopup()
			tracking = t.s.tracking
		})
		for deadline := time.Now().Add(3 * time.Second); tracking && time.Now().Before(deadline); {
			time.Sleep(10 * time.Millisecond)
			onMainSync(func() { tracking = t.s.tracking })
		}
		if tracking {
			return errors.New("the context menu kept tracking after it was cancelled")
		}
		return nil
	}
	var err error
	deadline := time.Now().Add(3 * time.Second)
	for {
		shown := false
		onMainSync(func() {
			s := t.s
			if shown = s.popup != 0 && s.tracking; !shown {
				return
			}
			if id != 0 {
				if item, menu := menuItem(s.popup, id); item != 0 {
					menu.Send(sel("performActionForItemAtIndex:"), objc.Send[int](menu, sel("indexOfItem:"), item))
				} else {
					err = fmt.Errorf("the context menu has no item %d", id)
				}
			}
			s.closePopup()
		})
		if shown || time.Now().After(deadline) {
			if !shown {
				return fmt.Errorf("no context menu shows")
			}
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
}
