//go:build ios && (nsgotest || nsgoautomation)

package ios

import (
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// A surface acting as the user does on an iPhone, an Apple TV or their simulators, for the test host (testhost.go) and for nsgo dev's automation (automation.go), whose live surfaces wrap the running app's scenes with no test options (test is nil). On simulators, touches are IOHID digitizer events handed to UIKit (-[UIApplication _enqueueHIDEvent:]), so UIKit makes the UITouches and runs hit testing, gesture recognizers and scroll views as for a finger, and typing goes through the software keyboard's input manager; IOKit's event functions, BackBoardServices' digitizer info and the private keyboard classes are looked up at run time and link only under nsgotest or nsgoautomation. On physical devices (or when simulator HID is unavailable), presses, scrolls, drags, alerts, typing and keys use public UIKit APIs, accessibility actions, UIControl actions and NSGo's hit testing and event dispatch instead of simulator HID.

func onMainSync(fn func()) {
	done := make(chan struct{})
	apple.OnMain(func() {
		defer close(done)
		fn()
	})
	<-done
}

// scriptedInput tracks scripted IOHID events and synchronous scripted steps
// for test input isolation (installed only by testhost.go; live automation
// leaves testInputFilter nil). Accessed only on the main thread.
var scriptedInput testInputGate

// scriptedMainSync runs fn on the main thread inside a scripted input scope so
// test input isolation accepts synchronous UIKit callbacks triggered by fn.
func scriptedMainSync(fn func()) {
	onMainSync(func() {
		scriptedInput.enter()
		defer scriptedInput.leave()
		fn()
	})
}

func scriptedTestInputAllowed(window, event objc.ID) bool {
	kind := objc.Send[int](event, sel("type"))
	var hidEvent uintptr
	if objc.Send[bool](event, sel("respondsToSelector:"), sel("_hidEvent")) {
		hidEvent = objc.Send[uintptr](event, sel("_hidEvent"))
	}
	var sender, ts uint64
	hasHID := hidEvent != 0
	if hasHID {
		if hidGetSender != nil {
			sender = hidGetSender(hidEvent)
		}
		if hidGetTimeStamp != nil {
			ts = hidGetTimeStamp(hidEvent)
		}
	}
	return scriptedInput.allowEvent(true, kind, uintptr(window), hasHID, sender, ts)
}

type testSurface struct {
	s       *surface
	test    *testOpen
	pressID int // last simulator HID press release marker, main thread only
}

var (
	// deviceKeys holds synthetic KeyDown/KeyUp state per surface on physical devices,
	// accessed only on the main thread.
	deviceKeys = map[*surface]*deviceKeyState{}
	// deviceTouchID numbers synthetic EventTouch contacts on physical devices.
	deviceTouchID int
	// hidPresses tracks simulator HID press releases across NsgoWindow.sendEvent:, main thread only.
	hidPresses hidPressTracker
)

func init() {
	onWindowEvent = func(window, event objc.ID) {
		if event == 0 {
			return
		}
		const eventTypeTouches = 0 // UIEventTypeTouches
		if objc.Send[int](event, sel("type")) != eventTypeTouches {
			return
		}
		touches := event.Send(sel("touchesForWindow:"), window)
		if touches == 0 {
			return
		}
		ended := false
		apple.ForEach(touches, func(t objc.ID) {
			const (
				phaseEnded     = 3 // UITouchPhaseEnded
				phaseCancelled = 4 // UITouchPhaseCancelled
			)
			ph := objc.Send[int](t, sel("phase"))
			if ph == phaseEnded || ph == phaseCancelled {
				ended = true
			}
		})
		if ended {
			hidPresses.delivered(uintptr(window))
		}
	}
}

func (s *surface) deviceKeyState() *deviceKeyState {
	st := deviceKeys[s]
	if st == nil {
		st = &deviceKeyState{}
		deviceKeys[s] = st
	}
	return st
}

// runningOnSimulator reports whether the process runs inside Xcode's iOS or tvOS simulator.
func runningOnSimulator() bool {
	return isSimulatorEnv(os.Getenv)
}

// useSimulatorHID reports whether this surface should post private IOHID events
// (simulators only, with clean fallback to the physical device path if HID fails to load).
func (t *testSurface) useSimulatorHID() bool {
	return shouldUseSimulatorHID(runningOnSimulator(), loadHID)
}

// window returns the surface's current UIWindow, or an error once its scene has no window.
func (t *testSurface) window() (objc.ID, error) {
	var w objc.ID
	onMainSync(func() {
		if t.s != nil && !t.s.closed && !t.s.ending {
			w = t.s.window
		}
	})
	if w == 0 {
		return 0, errors.New("the scene has no window")
	}
	return w, nil
}

// IOKit's digitizer and keyboard events and BackBoardServices' digitizer
// info, from IOHIDEvent.h and BKSHIDEvent.h.
var (
	hidMu            sync.Mutex
	hidLoaded        bool
	hidDigitizer     func(alloc uintptr, time uint64, transducer, index, identity, mask, buttons uint32, x, y, z, tip, barrel float64, inRange, touching uint8, options uint32) uintptr
	hidFinger        func(alloc uintptr, time uint64, index, identity, mask uint32, x, y, z, tip, twist float64, inRange, touching uint8, options uint32) uintptr
	hidKeyboard      func(alloc uintptr, time uint64, page, usage uint32, down uint8, options uint32) uintptr
	hidAppend        func(parent, child uintptr, options uint32)
	hidSetInteger    func(event uintptr, field uint32, value int64)
	hidSetFloat      func(event uintptr, field uint32, value float64)
	hidSetSender     func(event uintptr, sender uint64)
	hidGetSender     func(event uintptr) uint64
	hidGetTimeStamp  func(event uintptr) uint64
	bksDigitizerInfo func(event uintptr, context uint32, gesturePossible, gestureChange uint8, display uintptr, initial float64, force float32)
	machAbsoluteTime func() uint64
)

func loadHID() error {
	hidMu.Lock()
	defer hidMu.Unlock()
	if hidLoaded {
		return nil
	}
	open := func(path string) (uintptr, error) {
		return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	}
	iokit, err := open("/System/Library/Frameworks/IOKit.framework/IOKit")
	if err != nil {
		return err
	}
	bbs, err := open("/System/Library/PrivateFrameworks/BackBoardServices.framework/BackBoardServices")
	if err != nil {
		return err
	}
	system, err := open("/usr/lib/libSystem.B.dylib")
	if err != nil {
		return err
	}
	purego.RegisterLibFunc(&hidDigitizer, iokit, "IOHIDEventCreateDigitizerEvent")
	purego.RegisterLibFunc(&hidFinger, iokit, "IOHIDEventCreateDigitizerFingerEvent")
	purego.RegisterLibFunc(&hidKeyboard, iokit, "IOHIDEventCreateKeyboardEvent")
	purego.RegisterLibFunc(&hidAppend, iokit, "IOHIDEventAppendEvent")
	purego.RegisterLibFunc(&hidSetInteger, iokit, "IOHIDEventSetIntegerValue")
	purego.RegisterLibFunc(&hidSetFloat, iokit, "IOHIDEventSetFloatValue")
	purego.RegisterLibFunc(&hidSetSender, iokit, "IOHIDEventSetSenderID")
	purego.RegisterLibFunc(&hidGetSender, iokit, "IOHIDEventGetSenderID")
	purego.RegisterLibFunc(&hidGetTimeStamp, iokit, "IOHIDEventGetTimeStamp")
	purego.RegisterLibFunc(&bksDigitizerInfo, bbs, "BKSHIDEventSetDigitizerInfo")
	purego.RegisterLibFunc(&machAbsoluteTime, system, "mach_absolute_time")
	hidLoaded = true
	return nil
}

const (
	transducerHand    = 3
	maskRange         = 1 << 0
	maskTouch         = 1 << 1
	maskPosition      = 1 << 2
	fieldIntegrated   = 11<<16 | 25 // kIOHIDEventFieldDigitizerIsDisplayIntegrated
	fieldMajorRadius  = 11<<16 | 20
	fieldMinorRadius  = 11<<16 | 21
	testSender        = testSenderID
	keyboardUsagePage = 7
)

// enqueue hands event to UIKit as window w's, then releases it.
func enqueue(w objc.ID, event uintptr) {
	context := objc.Send[uint32](w, sel("_contextId"))
	hidSetSender(event, testSender)
	if testInputFilter != nil && hidGetTimeStamp != nil {
		scriptedInput.rememberHID(uintptr(w), hidGetTimeStamp(event))
	}
	bksDigitizerInfo(event, context, 0, 0, 0, 0, 0)
	objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication")).Send(sel("_enqueueHIDEvent:"), event)
	apple.Release(event)
}

type phase uint8

const (
	down phase = iota
	move
	up
)

// touch is one finger at p, window w's points, in ph.
func touch(w objc.ID, p driver.Point, ph phase) {
	onMainSync(func() { enqueueTouch(w, p, ph) })
}

func enqueueTouch(w objc.ID, p driver.Point, ph phase) {
	if w == 0 {
		return
	}
	touching := uint8(0)
	if ph != up {
		touching = 1
	}
	mask := uint32(maskRange | maskTouch)
	if ph == move {
		mask = maskPosition
	}
	now := machAbsoluteTime()
	hand := hidDigitizer(0, now, transducerHand, 0, 0, mask, 0, 0, 0, 0, 0, 0, touching, touching, 0)
	if hand == 0 {
		return
	}
	hidSetInteger(hand, fieldIntegrated, 1)
	finger := hidFinger(0, now, 1, 2, mask, float64(p.X), float64(p.Y), 0, float64(touching), 0, touching, touching, 0)
	if finger != 0 {
		hidSetFloat(finger, fieldMajorRadius, 5)
		hidSetFloat(finger, fieldMinorRadius, 5)
		hidSetInteger(finger, fieldIntegrated, 1)
		hidAppend(hand, finger, 0)
		apple.Release(finger)
	}
	enqueue(w, hand)
}

// focusAt gives the text field at p in window w the focus: made touches leave the
// first responder where it was, as UI test tools know.
func focusAt(w objc.ID, p driver.Point) {
	scriptedMainSync(func() { focusAtOnMain(w, p) })
}

func focusAtOnMain(w objc.ID, p driver.Point) {
	if w == 0 {
		return
	}
	hit := w.Send(sel("hitTest:withEvent:"), apple.CGPoint{X: float64(p.X), Y: float64(p.Y)}, objc.ID(0))
	field, text := objc.ID(apple.Class("UITextField")), objc.ID(apple.Class("UITextView"))
	for v := hit; v != 0; v = v.Send(sel("superview")) {
		var ed objc.ID
		switch {
		case objc.Send[bool](v, sel("isKindOfClass:"), field) || objc.Send[bool](v, sel("isKindOfClass:"), text):
			ed = v
		default:
			if nv := views[v]; nv != nil && nv.input != nil && nv.input.field != 0 {
				ed = nv.input.field
			}
		}
		if ed != 0 {
			if !objc.Send[bool](ed, sel("isFirstResponder")) && objc.Send[bool](ed, sel("canBecomeFirstResponder")) {
				ed.Send(sel("becomeFirstResponder"))
			}
			return
		}
	}
}

// focusedEditor returns the active first responder, or the focused NSGo input's editor.
func (t *testSurface) focusedEditor(w objc.ID) objc.ID {
	if f := w.Send(sel("firstResponder")); f != 0 {
		return f
	}
	for _, v := range t.s.views {
		if v != nil && v.input != nil && v.input.focused && v.input.field != 0 {
			return v.input.field
		}
	}
	return 0
}

// surfacePoint converts a point in window w's coordinates to the surface's root view.
func (t *testSurface) surfacePoint(w objc.ID, at driver.Point) driver.Point {
	p := objc.Send[apple.CGPoint](t.s.native, sel("convertPoint:fromView:"), apple.CGPoint{X: float64(at.X), Y: float64(at.Y)}, w)
	return driver.Point{X: driver.Points(p.X), Y: driver.Points(p.Y)}
}

// viewPointAndPage returns at (in window w's coordinates) in v's local coordinates and in v's modal or surface root.
func (t *testSurface) viewPointAndPage(w objc.ID, v *view, at driver.Point) (local, page apple.CGPoint) {
	root := t.s.native
	for n := v.native; n != 0 && n != root; n = n.Send(selSuperview) {
		if m := views[n]; m != nil && m.native == n && m.kind == driver.KindModal {
			root = n
			break
		}
	}
	wp := apple.CGPoint{X: float64(at.X), Y: float64(at.Y)}
	local = objc.Send[apple.CGPoint](v.native, sel("convertPoint:fromView:"), wp, w)
	page = objc.Send[apple.CGPoint](root, sel("convertPoint:fromView:"), wp, w)
	return local, page
}

// pressOutsideAt notifies views listening for EventPressOutside that a press landed outside them.
func (t *testSurface) pressOutsideAt(hit, w objc.ID, at driver.Point) {
	wp := apple.CGPoint{X: float64(at.X), Y: float64(at.Y)}
	surfaceAt := t.surfacePoint(w, at)
	for _, v := range t.s.views {
		if v == nil || !v.events.Has(driver.EventPressOutside) || v.native.Send(sel("window")) == 0 {
			continue
		}
		p := objc.Send[apple.CGPoint](v.native, sel("convertPoint:fromView:"), wp, w)
		b := objc.Send[apple.CGRect](v.native, selBounds)
		inside := p.X >= 0 && p.Y >= 0 && p.X < b.Size.Width && p.Y < b.Size.Height
		if inside || (hit != 0 && objc.Send[bool](hit, sel("isDescendantOfView:"), v.native)) {
			continue
		}
		t.s.engine.Dispatch(driver.Event{Kind: driver.EventPressOutside, ID: v.id, Point: surfaceAt})
	}
}

// dispatchSyntheticTouch sends one synthetic EventTouch through surface.captureTouches on the main thread.
func (t *testSurface) dispatchSyntheticTouch(touchID int, phase driver.GesturePhase, surfaceAt driver.Point) {
	ev := driver.Event{
		Kind:  driver.EventTouch,
		Phase: phase,
		Touches: []driver.Touch{{
			ID:           touchID,
			PagePosition: surfaceAt,
			Changed:      true,
		}},
		Time: time.Since(t.s.epoch),
	}
	t.s.captureTouches(&ev)
	t.s.engine.Dispatch(ev)
}

func (t *testSurface) Press(at driver.Point, hold time.Duration, mods driver.Modifiers) error {
	w, err := t.window()
	if err != nil {
		return err
	}
	if t.useSimulatorHID() {
		return t.pressSimulatorHID(w, at, hold, mods)
	}
	return t.pressDevice(w, at, hold, mods)
}

func (t *testSurface) pressSimulatorHID(w objc.ID, at driver.Point, hold time.Duration, mods driver.Modifiers) error {
	var held []uint32
	for _, key := range []struct {
		usage uint32
		held  bool
	}{{0xe0, mods.Control}, {0xe1, mods.Shift}, {0xe2, mods.Alt}, {0xe3, mods.Platform}} {
		if key.held {
			hidKey(w, key.usage, true)
			held = append(held, key.usage)
		}
	}
	defer func() {
		for i := len(held) - 1; i >= 0; i-- {
			hidKey(w, held[i], false)
		}
	}()
	touch(w, at, down)
	time.Sleep(max(hold+50*time.Millisecond, 60*time.Millisecond))
	onMainSync(func() {
		if hold == 0 && t.s != nil && !t.s.closed && !t.s.ending && len(t.s.alerts) == 0 &&
			objc.Send[bool](w, selIsKindOf, objc.ID(classWindow)) &&
			w.Send(sel("hitTest:withEvent:"), apple.CGPoint{X: float64(at.X), Y: float64(at.Y)}, objc.ID(0)) != 0 {
			t.pressID = hidPresses.mark(uintptr(w))
		}
		enqueueTouch(w, at, up)
		focusAtOnMain(w, at)
	})
	return nil
}

// pressDevice delivers a press on a physical iPhone or Apple TV through UIKit's
// hit testing, accessibility actions, UIControl actions and NSGo's event dispatch.
func (t *testSurface) pressDevice(w objc.ID, at driver.Point, hold time.Duration, mods driver.Modifiers) error {
	var (
		pressedView    *view
		listensTouches bool
		touchID        int
	)
	scriptedMainSync(func() {
		if w == 0 || t.s == nil || t.s.engine == nil {
			return
		}
		t.s.engine.InputModality(driver.ModalityPointer)
		wp := apple.CGPoint{X: float64(at.X), Y: float64(at.Y)}
		hit := w.Send(sel("hitTest:withEvent:"), wp, objc.ID(0))
		t.pressOutsideAt(hit, w, at)
		listensTouches = t.s.listensTouches()
		if listensTouches {
			deviceTouchID++
			touchID = deviceTouchID
			t.dispatchSyntheticTouch(touchID, driver.GestureBegan, t.surfacePoint(w, at))
		}
		fieldClass := objc.ID(apple.Class("UITextField"))
		textViewClass := objc.ID(apple.Class("UITextView"))
		switchClass := objc.ID(apple.Class("UISwitch"))
		tabBarClass := objc.ID(apple.Class("UITabBar"))
		controlClass := objc.ID(apple.Class("UIControl"))
		var chain []deviceHitNode
		var ids []objc.ID
		for n := hit; n != 0; n = n.Send(selSuperview) {
			isText := objc.Send[bool](n, selIsKindOf, fieldClass) || objc.Send[bool](n, selIsKindOf, textViewClass)
			isSw := objc.Send[bool](n, selIsKindOf, switchClass)
			isTab := objc.Send[bool](n, selIsKindOf, tabBarClass)
			isCtrl := !isText && !isSw && objc.Send[bool](n, selIsKindOf, controlClass)
			node := deviceHitNode{
				index:        len(ids),
				isTextEditor: isText,
				isSwitch:     isSw,
				isTabBar:     isTab,
				isControl:    isCtrl,
				isAxElement:  objc.Send[bool](n, sel("isAccessibilityElement")),
			}
			if v := views[n]; v != nil && v.surface == t.s && v.id != 0 {
				if v.input != nil && v.input.field != 0 {
					node.isTextEditor = true
				}
				if _, ok := v.component.(*uiSwitch); ok {
					node.isSwitch = true
				}
				if _, ok := v.component.(*tabBar); ok {
					node.isTabBar = true
				}
				node.isNSGoView = true
				node.pressable = v.pressable()
				node.hasActivate = v.props.HasAction(driver.ActionActivate) && v.events.Has(driver.EventAccessibilityAction)
				node.hasPressDownUp = v.events.Has(driver.EventPressDown) || v.events.Has(driver.EventPressUp) || v.events.Has(driver.EventLongPress)
				local, _ := t.viewPointAndPage(w, v, at)
				node.hasLinkAtPoint = v.indexAt(local) >= 0
			}
			chain = append(chain, node)
			ids = append(ids, n)
		}
		target := resolveDevicePress(chain, hold > 0, mods)
		switch target.kind {
		case devicePressFocusText:
			ed := ids[target.index]
			if v := views[ed]; v != nil && v.input != nil && v.input.field != 0 {
				ed = v.input.field
			}
			if !objc.Send[bool](ed, sel("isFirstResponder")) && objc.Send[bool](ed, sel("canBecomeFirstResponder")) {
				ed.Send(sel("becomeFirstResponder"))
			}
		case devicePressSwitch:
			targetID := ids[target.index]
			if v := views[targetID]; v != nil {
				if sw, ok := v.component.(*uiSwitch); ok {
					sw.Command("activate", nil)
					return
				}
			}
			if objc.Send[bool](targetID, sel("isEnabled")) {
				on := !objc.Send[bool](targetID, sel("isOn"))
				targetID.Send(sel("setOn:animated:"), on, true)
				const valueChanged = 1 << 12
				targetID.Send(sel("sendActionsForControlEvents:"), uint(valueChanged))
			}
		case devicePressTabBar:
			targetID := ids[target.index]
			tb := tabBars[targetID]
			if tb == nil {
				if v := views[targetID]; v != nil {
					tb, _ = v.component.(*tabBar)
				}
			}
			if tb != nil && len(tb.items) > 0 {
				local := objc.Send[apple.CGPoint](tb.bar, sel("convertPoint:fromView:"), wp, w)
				b := objc.Send[apple.CGRect](tb.bar, selBounds)
				if b.Size.Width > 0 {
					idx := min(max(int(local.X/b.Size.Width*float64(len(tb.items))), 0), len(tb.items)-1)
					tb.selected = idx
					tb.showSelected()
					tb.cx.Emit("change", float64(idx))
				}
			}
		case devicePressControl:
			targetID := ids[target.index]
			if objc.Send[bool](targetID, sel("isEnabled")) {
				const touchUpInside = 1 << 6
				targetID.Send(sel("sendActionsForControlEvents:"), uint(touchUpInside))
			}
		case devicePressAccessibility:
			targetID := ids[target.index]
			objc.Send[bool](targetID, sel("accessibilityActivate"))
		case devicePressView:
			v := views[ids[target.index]]
			if v == nil {
				return
			}
			if onTV() && v.focusable() {
				t.s.focusView(v)
			}
			local, page := t.viewPointAndPage(w, v, at)
			v.pressing.Store(true)
			v.longFired.Store(false)
			v.pressAt = driver.Event{
				Point:     driver.Point{X: driver.Points(local.X), Y: driver.Points(local.Y)},
				Page:      driver.Point{X: driver.Points(page.X), Y: driver.Points(page.Y)},
				Modifiers: mods,
				Count:     1,
				Index:     v.indexAt(local),
			}
			v.beginPress()
			pressedView = v
		}
	})
	if pressedView != nil || listensTouches {
		time.Sleep(max(hold+50*time.Millisecond, 60*time.Millisecond))
		scriptedMainSync(func() {
			if w == 0 || t.s == nil || t.s.engine == nil {
				return
			}
			if listensTouches {
				t.dispatchSyntheticTouch(touchID, driver.GestureEnded, t.surfacePoint(w, at))
			}
			if pressedView != nil && int(pressedView.id) < len(t.s.views) && t.s.views[pressedView.id] == pressedView {
				pressed := pressedView.pressing.Swap(false)
				pressedView.pressed(driver.EventPressUp)
				if pressed && !pressedView.longFired.Load() {
					e := pressedView.pressAt
					e.Kind = driver.EventPress
					pressedView.report(e)
				}
			}
		})
	}
	focusAt(w, at)
	return nil
}

func (t *testSurface) RightClick(driver.Point) error {
	return errors.New("a phone has no secondary click: use LongPress")
}

// fingerAlong moves a finger along path in window w, 10 points a frame, then rests
// still reporting where it is before it lifts, so it ends without a
// fling.
func fingerAlong(w objc.ID, path []driver.Point) {
	touch(w, path[0], down)
	time.Sleep(100 * time.Millisecond)
	at := path[0]
	for _, to := range path[1:] {
		n := int(math.Max(math.Abs(float64(to.X-at.X)), math.Abs(float64(to.Y-at.Y)))/10) + 1
		for i := 1; i <= n; i++ {
			f := driver.Points(i) / driver.Points(n)
			touch(w, driver.Point{X: at.X + (to.X-at.X)*f, Y: at.Y + (to.Y-at.Y)*f}, move)
			time.Sleep(16 * time.Millisecond)
		}
		at = to
	}
	for range 6 {
		touch(w, at, move)
		time.Sleep(16 * time.Millisecond)
	}
	touch(w, at, up)
}

func (t *testSurface) Drag(path []driver.Point) error {
	if len(path) == 0 {
		return errors.New("an empty drag")
	}
	w, err := t.window()
	if err != nil {
		return err
	}
	if t.useSimulatorHID() {
		fingerAlong(w, path)
		return nil
	}
	return t.dragDevice(w, path)
}

// dragDevice delivers a drag on a physical iPhone or Apple TV through EventTouch,
// EventPan (respecting PanActivation) or UIScrollView contentOffset updates.
func (t *testSurface) dragDevice(w objc.ID, path []driver.Point) error {
	steps := interpolateDragPath(path)
	var (
		panView        *view
		panning        bool
		listensTouches bool
		touchID        int
		startSurface   driver.Point
		lastSurface    driver.Point
		lastTime       time.Time
		vel            driver.Point
	)
	scriptedMainSync(func() {
		if w == 0 || t.s == nil || t.s.engine == nil {
			return
		}
		t.s.engine.InputModality(driver.ModalityPointer)
		wp := apple.CGPoint{X: float64(steps[0].X), Y: float64(steps[0].Y)}
		hit := w.Send(sel("hitTest:withEvent:"), wp, objc.ID(0))
		for n := hit; n != 0; n = n.Send(selSuperview) {
			if v := views[n]; v != nil && v.surface == t.s && v.id != 0 && v.events.Has(driver.EventPan) {
				panView = v
				break
			}
		}
		startSurface = t.surfacePoint(w, steps[0])
		lastSurface = startSurface
		lastTime = time.Now()
		listensTouches = t.s.listensTouches()
		if listensTouches {
			deviceTouchID++
			touchID = deviceTouchID
			t.dispatchSyntheticTouch(touchID, driver.GestureBegan, startSurface)
		}
	})
	for _, pt := range steps[1:] {
		time.Sleep(16 * time.Millisecond)
		scriptedMainSync(func() {
			if w == 0 || t.s == nil || t.s.engine == nil {
				return
			}
			curSurface := t.surfacePoint(w, pt)
			now := time.Now()
			if listensTouches {
				t.dispatchSyntheticTouch(touchID, driver.GestureChanged, curSurface)
			}
			if panView != nil && int(panView.id) < len(t.s.views) && t.s.views[panView.id] == panView {
				d := driver.Point{X: curSurface.X - startSurface.X, Y: curSurface.Y - startSurface.Y}
				if !panning {
					switch panView.props.PanActivation.Decide(d) {
					case 1:
						panning = true
						local, page := t.viewPointAndPage(w, panView, pt)
						panView.report(driver.Event{
							Kind:         driver.EventPan,
							Phase:        driver.GestureBegan,
							Point:        d,
							Position:     driver.Point{X: driver.Points(local.X), Y: driver.Points(local.Y)},
							PagePosition: driver.Point{X: driver.Points(page.X), Y: driver.Points(page.Y)},
						})
					case -1:
						panView = nil
					}
				}
				if panning {
					if dt := now.Sub(lastTime).Seconds(); dt > 0 {
						inst := driver.Point{X: (curSurface.X - lastSurface.X) / driver.Points(dt), Y: (curSurface.Y - lastSurface.Y) / driver.Points(dt)}
						vel = driver.Point{X: inst.X*0.7 + vel.X*0.3, Y: inst.Y*0.7 + vel.Y*0.3}
					}
					local, page := t.viewPointAndPage(w, panView, pt)
					panView.report(driver.Event{
						Kind:         driver.EventPan,
						Phase:        driver.GestureChanged,
						Point:        d,
						Position:     driver.Point{X: driver.Points(local.X), Y: driver.Points(local.Y)},
						PagePosition: driver.Point{X: driver.Points(page.X), Y: driver.Points(page.Y)},
						Velocity:     vel,
					})
				}
			}
			lastSurface, lastTime = curSurface, now
		})
	}
	last := steps[len(steps)-1]
	scriptedMainSync(func() {
		if w == 0 || t.s == nil || t.s.engine == nil {
			return
		}
		curSurface := t.surfacePoint(w, last)
		if listensTouches {
			t.dispatchSyntheticTouch(touchID, driver.GestureEnded, curSurface)
		}
		if panning && panView != nil && int(panView.id) < len(t.s.views) && t.s.views[panView.id] == panView {
			d := driver.Point{X: curSurface.X - startSurface.X, Y: curSurface.Y - startSurface.Y}
			local, page := t.viewPointAndPage(w, panView, last)
			panView.report(driver.Event{
				Kind:         driver.EventPan,
				Phase:        driver.GestureEnded,
				Point:        d,
				Position:     driver.Point{X: driver.Points(local.X), Y: driver.Points(local.Y)},
				PagePosition: driver.Point{X: driver.Points(page.X), Y: driver.Points(page.Y)},
			})
		}
	})
	if !panning && !listensTouches && len(steps) > 1 {
		delta := driver.Point{X: steps[0].X - last.X, Y: steps[0].Y - last.Y}
		return t.scrollDevice(w, steps[0], delta)
	}
	return nil
}

// Scroll drags the content the other way from delta: a finger moving up
// shows what is below.
func (t *testSurface) Scroll(at driver.Point, delta driver.Point) error {
	w, err := t.window()
	if err != nil {
		return err
	}
	if t.useSimulatorHID() {
		fingerAlong(w, []driver.Point{at, {X: at.X - delta.X, Y: at.Y - delta.Y}})
		return nil
	}
	return t.scrollDevice(w, at, delta)
}

// scrollDevice moves the innermost UIScrollView under at that can scroll toward
// delta (or dispatches EventWheel to a view listening for wheel input).
func (t *testSurface) scrollDevice(w objc.ID, at, delta driver.Point) error {
	scriptedMainSync(func() {
		if w == 0 || t.s == nil || t.s.engine == nil {
			return
		}
		wp := apple.CGPoint{X: float64(at.X), Y: float64(at.Y)}
		hit := w.Send(sel("hitTest:withEvent:"), wp, objc.ID(0))
		scrollClass := objc.ID(apple.Class("UIScrollView"))
		var firstScroll objc.ID
		var firstTo driver.Point
		for n := hit; n != 0; n = n.Send(selSuperview) {
			if !objc.Send[bool](n, selIsKindOf, scrollClass) || !objc.Send[bool](n, sel("isScrollEnabled")) {
				continue
			}
			cur := objc.Send[apple.CGPoint](n, selContentOffset)
			size := objc.Send[apple.CGSize](n, sel("contentSize"))
			in := objc.Send[edgeInsets](n, sel("adjustedContentInset"))
			seen := objc.Send[apple.CGRect](n, selBounds).Size
			to, canMove := clampScrollOffset(
				driver.Point{X: driver.Points(cur.X), Y: driver.Points(cur.Y)},
				delta,
				driver.Size{Width: driver.Points(size.Width), Height: driver.Points(size.Height)},
				driver.Size{Width: driver.Points(seen.Width), Height: driver.Points(seen.Height)},
				driver.Points(in.Top), driver.Points(in.Left), driver.Points(in.Bottom), driver.Points(in.Right),
			)
			if firstScroll == 0 {
				firstScroll, firstTo = n, to
			}
			if canMove {
				n.Send(sel("setContentOffset:animated:"), apple.CGPoint{X: float64(to.X), Y: float64(to.Y)}, false)
				return
			}
		}
		if firstScroll != 0 {
			firstScroll.Send(sel("setContentOffset:animated:"), apple.CGPoint{X: float64(firstTo.X), Y: float64(firstTo.Y)}, false)
			return
		}
		for n := hit; n != 0; n = n.Send(selSuperview) {
			if v := views[n]; v != nil && v.surface == t.s && v.id != 0 && v.events.Has(driver.EventWheel) {
				local, page := t.viewPointAndPage(w, v, at)
				v.report(driver.Event{
					Kind:         driver.EventWheel,
					Point:        delta,
					Position:     driver.Point{X: driver.Points(local.X), Y: driver.Points(local.Y)},
					PagePosition: driver.Point{X: driver.Points(page.X), Y: driver.Points(page.Y)},
				})
				return
			}
		}
	})
	return nil
}

func (t *testSurface) Hover(driver.Point) error { return nil }

// keyboardImpl is the software keyboard's input manager (private), which
// types into the first responder as its keys do, asking the field's
// delegate first.
func keyboardImpl() objc.ID {
	class := apple.Class("UIKeyboardImpl")
	if class == 0 {
		return 0
	}
	return objc.ID(class).Send(sel("activeInstance"))
}

// typeLatin switches the keyboard to US English as typing starts, so the
// keys a test presses type their letters whatever the simulator's language
// makes the keyboard, as a pinyin one would compose them into characters.
func typeLatin() {
	ctl := objc.ID(apple.Class("UIKeyboardInputModeController"))
	mode := objc.ID(apple.Class("UIKeyboardInputMode"))
	if ctl == 0 || mode == 0 {
		return
	}
	ctl = ctl.Send(sel("sharedInputModeController"))
	en := mode.Send(sel("keyboardInputModeWithIdentifier:"), apple.NSString("en_US@sw=QWERTY;hw=Automatic"))
	if ctl != 0 && en != 0 {
		ctl.Send(sel("setCurrentInputMode:"), en)
	}
}

func (t *testSurface) Type(text string) error {
	w, err := t.window()
	if err != nil {
		return err
	}
	if !t.useSimulatorHID() {
		return t.typeDevice(w, text)
	}
	onMainSync(typeLatin)
	for _, r := range text {
		scriptedMainSync(func() {
			if kb := keyboardImpl(); kb != 0 {
				kb.Send(sel("addInputString:"), apple.NSString(string(r)))
				return
			}
			if f := t.focusedEditor(w); f != 0 {
				f.Send(sel("insertText:"), apple.NSString(string(r)))
			}
		})
		time.Sleep(30 * time.Millisecond)
	}
	return nil
}

// typeDevice enters text into the focused field on a physical device through
// public UIKeyInput/UITextInput methods and the field's own validation.
func (t *testSurface) typeDevice(w objc.ID, text string) error {
	for _, r := range text {
		s := string(r)
		scriptedMainSync(func() {
			f := t.focusedEditor(w)
			if f == 0 {
				return
			}
			if in := inputOf(f); in != nil {
				selRange, ok := in.selection()
				if !ok {
					n := driver.UTF16Len(in.text())
					selRange = driver.TextRange{Start: n, End: n}
				}
				nsRange := apple.NSRange{Location: uint(selRange.Start), Length: uint(selRange.End - selRange.Start)}
				if in.shouldChange(nsRange, s) {
					f.Send(sel("insertText:"), apple.NSString(s))
				}
				return
			}
			if objc.Send[bool](f, sel("respondsToSelector:"), sel("insertText:")) {
				f.Send(sel("insertText:"), apple.NSString(s))
			}
		})
		time.Sleep(15 * time.Millisecond)
	}
	return nil
}

// Compose marks text in the first responder as an input method does,
// through UITextInput.
func (t *testSurface) Compose(text string) error {
	w, err := t.window()
	if err != nil {
		return err
	}
	scriptedMainSync(func() {
		f := t.focusedEditor(w)
		if f == 0 || !objc.Send[bool](f, sel("respondsToSelector:"), sel("setMarkedText:selectedRange:")) {
			err = errors.New("no text input has the keyboard")
			return
		}
		n := uint(len(utf16.Encode([]rune(text))))
		f.Send(sel("setMarkedText:selectedRange:"), apple.NSString(text), apple.NSRange{Location: n})
		if text == "" {
			f.Send(sel("unmarkText"))
		}
	})
	time.Sleep(30 * time.Millisecond)
	return err
}

// Commit marks text and unmarks it, which keeps it, as an input method
// commits.
func (t *testSurface) Commit(text string) error {
	w, err := t.window()
	if err != nil {
		return err
	}
	scriptedMainSync(func() {
		f := t.focusedEditor(w)
		if f == 0 || !objc.Send[bool](f, sel("respondsToSelector:"), sel("unmarkText")) {
			err = errors.New("no text input has the keyboard")
			return
		}
		n := uint(len(utf16.Encode([]rune(text))))
		f.Send(sel("setMarkedText:selectedRange:"), apple.NSString(text), apple.NSRange{Location: n})
		f.Send(sel("unmarkText"))
	})
	time.Sleep(30 * time.Millisecond)
	return err
}

// usages are the USB HID keyboard usages of the keys tests name.
var usages = map[string]uint32{
	"enter": 0x28, "return": 0x28, "escape": 0x29, "backspace": 0x2a, "tab": 0x2b, "space": 0x2c,
	"right": 0x4f, "left": 0x50, "down": 0x51, "up": 0x52, "delete": 0x4c,
	"shift": 0xe1, "ctrl": 0xe0, "control": 0xe0, "alt": 0xe2, "option": 0xe2, "mod": 0xe3, "cmd": 0xe3, "command": 0xe3, "meta": 0xe3,
}

func keyUsage(name string) (uint32, bool) {
	if u, ok := usages[name]; ok {
		return u, true
	}
	if len(name) == 1 {
		switch c := name[0]; {
		case c >= 'a' && c <= 'z':
			return 0x04 + uint32(c-'a'), true
		case c >= '1' && c <= '9':
			return 0x1e + uint32(c-'1'), true
		case c == '0':
			return 0x27, true
		}
	}
	return 0, false
}

func hidKey(w objc.ID, usage uint32, isDown bool) {
	onMainSync(func() {
		if w == 0 {
			return
		}
		d := uint8(0)
		if isDown {
			d = 1
		}
		if ev := hidKeyboard(0, machAbsoluteTime(), keyboardUsagePage, usage, d, 0); ev != 0 {
			enqueue(w, ev)
		}
	})
	time.Sleep(10 * time.Millisecond)
}

// softwareKeyBinding gives keymap bindings the same first chance as a real key before the test host directly asks UITextInput to edit. It leaves unbound editing keys to UIKit.
func (t *testSurface) softwareKeyBinding(name string, flags uint) bool {
	if t.s == nil || t.s.engine == nil || len(t.s.keys) == 0 {
		return false
	}
	id, ok := driver.FindKey(t.s.keys, apple.KeyNames(name, flags), t.s.focusKeyUse(), false)
	if !ok {
		return false
	}
	var editor objc.ID
	onMainSync(func() {
		for _, v := range t.s.views {
			if v != nil && v.input != nil && v.input.focused {
				editor = v.input.field
				break
			}
		}
		t.s.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: id})
		if editor != 0 && !objc.Send[bool](editor, sel("isFirstResponder")) {
			editor.Send(selBecomeFirst)
		}
	})
	return true
}

// Key presses a hardware keyboard's keys, as UIKit takes a real one's:
// the modifiers first. The edit keys a field takes from the software
// keyboard go through its input manager on simulators, and public UITextInput
// and engine dispatch on physical devices.
func (t *testSurface) Key(key string) error {
	w, err := t.window()
	if err != nil {
		return err
	}
	if !t.useSimulatorHID() {
		return t.keyDevice(w, key)
	}
	name := strings.ToLower(key)
	switch name {
	case "mod+a":
		_, _, flags, _ := parseDeviceKey(name)
		if t.softwareKeyBinding("a", flags) {
			return nil
		}
		scriptedMainSync(func() {
			if f := t.focusedEditor(w); f != 0 {
				f.Send(sel("selectAll:"), objc.ID(0))
			}
		})
		return nil
	case "backspace":
		_, _, flags, _ := parseDeviceKey(name)
		if t.softwareKeyBinding("backspace", flags) {
			return nil
		}
		var done bool
		scriptedMainSync(func() {
			if kb := keyboardImpl(); kb != 0 && t.s.editing() && w.Send(sel("firstResponder")) != 0 {
				kb.Send(sel("deleteFromInput"))
				done = true
			}
		})
		if done {
			return nil
		}
	case "enter", "return":
		var done bool
		scriptedMainSync(func() {
			// Only an editor's Return goes through the software keyboard; a focused pressable receives a hardware key through UIKit.
			if !t.s.editing() {
				return
			}
			if kb := keyboardImpl(); kb != 0 && t.s.editing() && w.Send(sel("firstResponder")) != 0 {
				kb.Send(sel("addInputString:"), apple.NSString("\n"))
				done = true
			}
		})
		if done {
			return nil
		}
	}
	parts := strings.Split(name, "+")
	var codes []uint32
	for _, p := range parts {
		u, ok := keyUsage(p)
		if !ok {
			return fmt.Errorf("unknown key %q in %q", p, key)
		}
		codes = append(codes, u)
	}
	for _, u := range codes {
		hidKey(w, u, true)
	}
	for i := len(codes) - 1; i >= 0; i-- {
		hidKey(w, codes[i], false)
	}
	return nil
}

// keyDevice delivers a key press or chord on a physical iPhone or Apple TV
// through public UITextInput methods, tvOS focus activation and engine dispatch.
func (t *testSurface) keyDevice(w objc.ID, key string) error {
	name := strings.ToLower(key)
	switch name {
	case "mod+a":
		_, _, flags, _ := parseDeviceKey(name)
		if t.softwareKeyBinding("a", flags) {
			return nil
		}
		scriptedMainSync(func() {
			if f := t.focusedEditor(w); f != 0 && objc.Send[bool](f, sel("respondsToSelector:"), sel("selectAll:")) {
				f.Send(sel("selectAll:"), objc.ID(0))
			}
		})
		return nil
	case "backspace":
		_, _, flags, _ := parseDeviceKey(name)
		if t.softwareKeyBinding("backspace", flags) {
			return nil
		}
		var done bool
		scriptedMainSync(func() {
			f := t.focusedEditor(w)
			if f == 0 || !t.s.editing() {
				return
			}
			if in := inputOf(f); in != nil && in.fs.Config.Readonly {
				done = true
				return
			}
			if objc.Send[bool](f, sel("respondsToSelector:"), sel("deleteBackward")) {
				f.Send(sel("deleteBackward"))
				done = true
			}
		})
		if done {
			return nil
		}
	case "enter", "return":
		var done bool
		scriptedMainSync(func() {
			if t.s.editing() {
				f := t.focusedEditor(w)
				if in := inputOf(f); in != nil {
					if !in.multiline {
						in.returnPressed()
						done = true
						return
					}
					selRange, ok := in.selection()
					if !ok {
						n := driver.UTF16Len(in.text())
						selRange = driver.TextRange{Start: n, End: n}
					}
					nsRange := apple.NSRange{Location: uint(selRange.Start), Length: uint(selRange.End - selRange.Start)}
					if in.shouldChange(nsRange, "\n") {
						f.Send(sel("insertText:"), apple.NSString("\n"))
					}
					done = true
					return
				}
			}
		})
		if done {
			return nil
		}
	}
	parsed, mods, flags, ok := parseDeviceKey(key)
	if !ok {
		return fmt.Errorf("unknown key %q", key)
	}
	scriptedMainSync(func() {
		if t.s == nil || t.s.engine == nil {
			return
		}
		t.s.engine.InputModality(driver.ModalityKeyboard)
		consumed := false
		var keyDownTarget driver.NodeID
		keyDownSent := false
		if !t.s.editing() {
			v := t.s.focused
			if v != nil && (int(v.id) >= len(t.s.views) || t.s.views[v.id] != v || !v.events.Has(driver.EventKeyDown)) {
				v = nil
			}
			if v != nil {
				keyDownTarget, keyDownSent = v.id, true
				consumed = t.s.engine.DispatchKeyDown(driver.Event{Kind: driver.EventKeyDown, ID: v.id, Text: parsed, Modifiers: mods})
			} else if t.s.heldKeys {
				keyDownTarget, keyDownSent = 0, true
				t.s.engine.Dispatch(driver.Event{Kind: driver.EventKeyDown, Text: parsed, Modifiers: mods})
			}
		}
		if !consumed && len(t.s.keys) > 0 {
			if id, ok := driver.FindKey(t.s.keys, apple.KeyNames(parsed, flags), t.s.focusKeyUse(), false); ok {
				t.s.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: id})
				consumed = true
			}
		}
		if !consumed {
			consumed = t.s.activateKey(parsed, mods)
		}
		if !consumed && onTV() && !t.s.editing() {
			switch parsed {
			case "escape":
				if tvMenuGoesBack(t.s.history, t.s.topModal() != nil, t.s.topmostCapture() != nil) {
					t.s.engine.Dispatch(driver.Event{Kind: driver.EventBack})
				}
			}
		}
		if keyDownSent {
			upMods, _ := applyDeviceModifier(mods, flags, parsed, false)
			t.s.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: keyDownTarget, Text: parsed, Modifiers: upMods})
		}
	})
	return nil
}

// KeyDown implements testhost.KeyHoldSurface: a hardware keyboard's key goes down and stays held, a modifier joining the keys after it, as UIKit tracks the keyboard's state, until its KeyUp.
func (t *testSurface) KeyDown(key string) error { return t.holdKey(key, true) }

// KeyUp implements testhost.KeyHoldSurface: the key KeyDown held comes up.
func (t *testSurface) KeyUp(key string) error { return t.holdKey(key, false) }

// holdKey sends one key of a hardware keyboard, not a chord, going down or coming up.
func (t *testSurface) holdKey(key string, isDown bool) error {
	name := strings.ToLower(key)
	if strings.Contains(name, "+") {
		return fmt.Errorf("%q is a chord: a key goes down or comes up alone", key)
	}
	w, err := t.window()
	if err != nil {
		return err
	}
	if !t.useSimulatorHID() {
		return t.holdKeyDevice(key, isDown)
	}
	u, ok := keyUsage(name)
	if !ok {
		return fmt.Errorf("unknown key %q", key)
	}
	hidKey(w, u, isDown)
	return nil
}

func (t *testSurface) holdKeyDevice(key string, isDown bool) error {
	parsed, _, _, ok := parseDeviceKey(key)
	if !ok {
		return fmt.Errorf("unknown key %q", key)
	}
	scriptedMainSync(func() {
		if t.s == nil || t.s.engine == nil {
			return
		}
		st := t.s.deviceKeyState()
		if isDown {
			t.s.engine.InputModality(driver.ModalityKeyboard)
			var targetID driver.NodeID
			reportKey := false
			if !t.s.editing() {
				v := t.s.focused
				if v != nil && int(v.id) < len(t.s.views) && t.s.views[v.id] == v && v.events.Has(driver.EventKeyDown) {
					targetID, reportKey = v.id, true
				} else if t.s.heldKeys {
					reportKey = true
				}
			}
			mods, flags, prevID, wasHeld := st.down(parsed, targetID)
			consumed := false
			if reportKey {
				if wasHeld {
					upMods, _ := applyDeviceModifier(mods, flags, parsed, false)
					t.s.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: prevID, Text: parsed, Modifiers: upMods})
				}
				if targetID != 0 {
					consumed = t.s.engine.DispatchKeyDown(driver.Event{Kind: driver.EventKeyDown, ID: targetID, Text: parsed, Modifiers: mods})
				} else {
					t.s.engine.Dispatch(driver.Event{Kind: driver.EventKeyDown, Text: parsed, Modifiers: mods})
				}
			}
			if !consumed && len(t.s.keys) > 0 {
				if id, ok := driver.FindKey(t.s.keys, apple.KeyNames(parsed, flags), t.s.focusKeyUse(), false); ok {
					t.s.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: id})
					consumed = true
				}
			}
			if !consumed && !wasHeld {
				t.s.activateKey(parsed, mods)
			}
			return
		}
		prevID, wasHeld, mods, _ := st.up(parsed)
		if wasHeld && !t.s.editing() && (prevID != 0 || t.s.heldKeys) {
			t.s.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: prevID, Text: parsed, Modifiers: mods})
		}
	})
	return nil
}

// Back is the escape the screen reader's gesture makes: the core goes
// back from the topmost overlay, modal or stack.
func (t *testSurface) Back() error {
	t.s.engine.Dispatch(driver.Event{Kind: driver.EventBack})
	return nil
}

// SelectTab selects the native tab bar's item at index, as tapping it would.
func (t *testSurface) SelectTab(id driver.NodeID, index int) error {
	var err error
	scriptedMainSync(func() {
		v := t.s.views[id]
		if v == nil {
			err = fmt.Errorf("no view has ID %d", id)
			return
		}
		bar, ok := v.component.(*tabBar)
		if !ok || !bar.selectIndex(index) {
			err = fmt.Errorf("view %d has no tab at index %d", id, index)
			return
		}
	})
	return err
}

// Answer taps the alert's button with the action's title on simulators, or
// dismisses the alert controller and dispatches the action on physical devices.
func (t *testSurface) Answer(id uint32, index int) error {
	if !t.useSimulatorHID() {
		var err error
		scriptedMainSync(func() {
			shown, ok := t.s.alerts[id]
			if !ok {
				err = fmt.Errorf("no alert %d shows", id)
				return
			}
			_, dispatchIdx, resolveErr := resolveAlertAction(shown.actions, index)
			if resolveErr != nil {
				err = resolveErr
				return
			}
			delete(t.s.alerts, id)
			if shown.ctrl != 0 {
				shown.ctrl.Send(sel("dismissViewControllerAnimated:completion:"), false, objc.ID(0))
			}
			t.s.engine.Dispatch(driver.Event{Kind: driver.EventAlert, Count: id, Index: dispatchIdx})
		})
		return err
	}
	var label string
	var err error
	var at driver.Point
	onMainSync(func() {
		shown, ok := t.s.alerts[id]
		if !ok {
			err = fmt.Errorf("no alert %d shows", id)
			return
		}
		var resolveErr error
		label, _, resolveErr = resolveAlertAction(shown.actions, index)
		if resolveErr != nil {
			err = resolveErr
			return
		}
		v := findLabel(shown.ctrl.Send(sel("view")), label)
		if v == 0 {
			err = fmt.Errorf("the alert shows no %q", label)
			return
		}
		b := objc.Send[apple.CGRect](v, sel("bounds"))
		p := objc.Send[apple.CGPoint](v, sel("convertPoint:toView:"), apple.CGPoint{X: b.Size.Width / 2, Y: b.Size.Height / 2}, objc.ID(0))
		at = driver.Point{X: driver.Points(p.X), Y: driver.Points(p.Y)}
	})
	if err != nil {
		return err
	}
	return t.Press(at, 0, driver.Modifiers{})
}

// findLabel finds the label reading text in v.
func findLabel(v objc.ID, text string) objc.ID {
	if objc.Send[bool](v, sel("isKindOfClass:"), objc.ID(apple.Class("UILabel"))) &&
		apple.GoString(v.Send(sel("text"))) == text {
		return v
	}
	subs := v.Send(sel("subviews"))
	for i := range objc.Send[uint](subs, sel("count")) {
		if f := findLabel(subs.Send(sel("objectAtIndex:"), i), text); f != 0 {
			return f
		}
	}
	return 0
}

func (t *testSurface) Screenshot() ([]byte, error) {
	var png []byte
	onMainSync(func() {
		if t.s.window == 0 {
			return
		}
		bounds := objc.Send[apple.CGRect](t.s.window, selBounds)
		format := objc.ID(apple.Class("UIGraphicsImageRendererFormat")).Send(sel("preferredFormat"))
		const standardRange = 2 // UIGraphicsImageRendererFormatRangeStandard
		format.Send(sel("setPreferredRange:"), standardRange)
		renderer := objc.ID(apple.Class("UIGraphicsImageRenderer")).Send(selAlloc).Send(sel("initWithBounds:format:"), bounds, format)
		draw := objcblock.New(func(_ objc.Block, _ objc.ID) {
			objc.Send[bool](t.s.window, sel("drawViewHierarchyInRect:afterScreenUpdates:"), bounds, true)
		})
		data := renderer.Send(sel("PNGDataWithActions:"), draw)
		draw.Release()
		if data != 0 {
			png = apple.Bytes(data)
		}
		renderer.Send(selRelease)
	})
	if png == nil {
		return nil, errors.New("the window drew nothing")
	}
	return png, nil
}

// Animating counts finite Core Animation animations (a screen pushed, a
// sheet presented, an alert fading in) and scroll views gliding.
func (t *testSurface) Sync() { t.s.engine.Sync() }

func (t *testSurface) Animating() bool {
	var busy bool
	onMainSync(func() {
		if t.s == nil {
			return
		}
		// An alert or a closing scene can take queued touches outside NsgoWindow.
		if t.s.closed || t.s.ending || len(t.s.alerts) > 0 {
			hidPresses.clear()
			t.pressID = 0
		}
		windowAnimating := t.s.window != 0 && (layersAnimating(t.s.window.Send(sel("layer"))) || gliding(t.s.window))
		busy = testSurfaceBusy(t.s.batches.Len(), t.s.animator.Ending(), len(t.s.motions), len(t.s.ghosts), hidPresses.pending(t.pressID), windowAnimating)
	})
	return busy
}

func layersAnimating(layer objc.ID) bool {
	if keys := layer.Send(sel("animationKeys")); keys != 0 {
		for i := range objc.Send[uint](keys, sel("count")) {
			a := layer.Send(sel("animationForKey:"), keys.Send(sel("objectAtIndex:"), i))
			if a == 0 {
				continue
			}
			// A caret's blink repeats "forever" as a huge count.
			repeat, duration := objc.Send[float32](a, sel("repeatCount")), objc.Send[float64](a, sel("duration"))
			if repeat < 1e6 && duration < 1e6 && !math.IsInf(duration, 0) {
				return true
			}
		}
	}
	subs := layer.Send(sel("sublayers"))
	if subs == 0 {
		return false
	}
	for i := range objc.Send[uint](subs, sel("count")) {
		if layersAnimating(subs.Send(sel("objectAtIndex:"), i)) {
			return true
		}
	}
	return false
}

func gliding(v objc.ID) bool {
	if objc.Send[bool](v, sel("isKindOfClass:"), objc.ID(apple.Class("UIScrollView"))) && objc.Send[bool](v, sel("isDecelerating")) {
		return true
	}
	subs := v.Send(sel("subviews"))
	for i := range objc.Send[uint](subs, sel("count")) {
		if gliding(subs.Send(sel("objectAtIndex:"), i)) {
			return true
		}
	}
	return false
}

// Resize does nothing: a phone's screen is the size it is.
func (t *testSurface) Resize(driver.Size) error { return nil }

func (t *testSurface) SetEnvironment(env driver.Environment) error {
	if t.test == nil {
		return errors.New("a running app follows the system's settings")
	}
	t.test.pin.Set(&env)
	t.s.engine.Repin()
	return nil
}

// ChooseMenu implements testhost.Surface: no native menu bar shows here.
func (t *testSurface) ChooseMenu(uint32) error { return testhost.ErrNoMenu }

// OpenLinks delivers the URLs supplied by the test to the app's engine.
func (t *testSurface) OpenLinks(links []string) {
	t.s.engine.Dispatch(driver.Event{Kind: driver.EventOpenLinks, Data: links})
}

// ChooseDockMenu implements testhost.Surface: iOS has no app icon menu.
func (t *testSurface) ChooseDockMenu(uint32) error { return testhost.ErrNoMenu }

// ChoosePopup implements testhost.Surface: the core shows no native
// context menus here.
func (t *testSurface) ChoosePopup(uint32) error { return testhost.ErrNoMenu }
