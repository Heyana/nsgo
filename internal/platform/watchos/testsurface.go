//go:build ios && (nsgotest || nsgoautomation)

package watchos

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// A surface acting as the user does on an Apple Watch or its simulator, for
// the test host (testhost.go) and for nsgo dev's automation (automation.go),
// whose live surface wraps the running app's surface with no test options
// (test is nil). On the watchOS simulator, touches are posted as IOHID
// digitizer events through -[UIApplication _enqueueHIDEvent:], so SwiftUI's
// own ScrollView, TextField and DragGesture handlers see presses, drags and
// scrolls as a finger does. On physical watches or when simulator HID is
// unavailable, input falls back cleanly to the hostTouch bridge and
// SwiftUI-synced focus/scroll commands.

func onMainSync(fn func()) {
	done := make(chan struct{})
	apple.OnMain(func() {
		defer close(done)
		fn()
	})
	<-done
}

// scriptedOnMain runs fn on the main thread inside a scripted input scope so test input isolation accepts the input callbacks fn triggers.
func scriptedOnMain(fn func()) {
	onMainSync(func() {
		s.inputGate.enter()
		defer s.inputGate.leave()
		fn()
	})
}

type testSurface struct{ test *testOpen }

// touchAt sends a touch's phase at at directly through hostTouch on the main
// thread, used when simulator HID posting is unavailable.
func touchAt(phase uint8, at driver.Point) {
	t := time.Since(s.epoch).Seconds()
	scriptedOnMain(func() { hostTouch(phase, float64(at.X), float64(at.Y), t) })
}

// waitNativeTouchEnd waits up to timeout for SwiftUI's DragGesture to report
// touchEnded/touchCancelled for seq.
func waitNativeTouchEnd(seq uint64, timeout time.Duration) (seen, ended bool) {
	deadline := time.Now().Add(timeout)
	for {
		onMainSync(func() {
			seen = s.inputGate.touchSeenSince(seq)
			ended = s.inputGate.touchEndedSince(seq)
		})
		if ended || time.Now().After(deadline) {
			return seen, ended
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (t *testSurface) Press(at driver.Point, hold time.Duration, mods driver.Modifiers) error {
	if mods != (driver.Modifiers{}) {
		return errors.New("watchOS has no keyboard modifier input")
	}
	if watchHIDReady() {
		if t.pressNativeHID(at, hold) {
			return nil
		}
	}
	t.pressFallback(at, hold)
	return nil
}

// pressNativeHID posts a real digitizer down/up touch to SwiftUI's hosting
// UIWindow and settles any TextField focus change. Returns false if HID
// delivery did not reach SwiftUI so Press can fall back seamlessly.
func (t *testSurface) pressNativeHID(at driver.Point, hold time.Duration) bool {
	var (
		seq      uint64
		posted   bool
		hitField driver.NodeID
		canFocus bool
		wasFocus driver.NodeID
	)
	onMainSync(func() {
		seq = s.inputGate.beginNativeTouch()
		if s.focused != nil {
			wasFocus = s.focused.id
		}
		if root := s.view(s.root); root != nil {
			if v := hit(root, at, func(v *view) bool { return v.field != nil }); v != nil {
				hitField = v.id
				canFocus = fieldCanFocus(v.field.Config)
			}
		}
		posted = postWatchHIDTouchOnMain(at, watchHIDDown)
	})
	if !posted {
		onMainSync(func() { s.inputGate.endNativeTouch() })
		watchHIDBroken.Store(true)
		return false
	}
	time.Sleep(max(hold+50*time.Millisecond, 60*time.Millisecond))
	onMainSync(func() {
		posted = postWatchHIDTouchOnMain(at, watchHIDUp)
	})
	if !posted {
		onMainSync(func() { s.inputGate.endNativeTouch() })
		watchHIDBroken.Store(true)
		return false
	}
	seen, _ := waitNativeTouchEnd(seq, 250*time.Millisecond)
	if !seen {
		onMainSync(func() { s.inputGate.endNativeTouch() })
		watchHIDBroken.Store(true)
		return false
	}
	if hold == 0 {
		want := resolvePressFocus(wasFocus, hitField, canFocus)
		if want.focus != 0 || want.blur != 0 {
			onMainSync(func() { s.inputGate.setNativeFocusWindow(true) })
			deadline := time.Now().Add(80 * time.Millisecond)
			for {
				var matched bool
				onMainSync(func() {
					var cur driver.NodeID
					if s.focused != nil {
						cur = s.focused.id
					}
					if want.focus != 0 {
						matched = cur == want.focus
					} else {
						matched = cur != want.blur
					}
				})
				if matched || time.Now().After(deadline) {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			// Reconcile if SwiftUI did not complete the focus change natively.
			scriptedOnMain(func() {
				s.inputGate.setNativeFocusWindow(false)
				var cur driver.NodeID
				if s.focused != nil {
					cur = s.focused.id
				}
				act := resolvePressFocus(cur, hitField, canFocus)
				if act.blur != 0 {
					if prev := s.view(act.blur); prev != nil {
						s.blurField(prev)
						s.w.op(opBlur, prev.hostID)
					}
				}
				if act.focus != 0 {
					if next := s.view(act.focus); next != nil {
						s.focusField(next)
						s.w.op(opFocus, next.hostID)
					}
				}
				s.flush()
			})
		}
	}
	onMainSync(func() { s.inputGate.endNativeTouch() })
	return true
}

func (t *testSurface) pressFallback(at driver.Point, hold time.Duration) {
	touchAt(touchBegan, at)
	time.Sleep(max(hold+50*time.Millisecond, 60*time.Millisecond))
	endedAt := time.Since(s.epoch).Seconds()
	scriptedOnMain(func() {
		// Resolve the tap's own focus change before touchEnded dispatches
		// EventPress, so an OnPress handler's Ref.Focus() is never blurred
		// afterward, and keep SwiftUI's FieldNode focus in sync.
		if root := s.view(s.root); root != nil {
			var focusedID, hitID driver.NodeID
			var canFocus bool
			if s.focused != nil {
				focusedID = s.focused.id
			}
			if v := hit(root, at, func(v *view) bool { return v.field != nil }); v != nil {
				hitID = v.id
				canFocus = fieldCanFocus(v.field.Config)
			}
			act := resolvePressFocus(focusedID, hitID, canFocus)
			if act.blur != 0 {
				if prev := s.view(act.blur); prev != nil {
					s.blurField(prev)
					s.w.op(opBlur, prev.hostID)
				}
			}
			if act.focus != 0 {
				if next := s.view(act.focus); next != nil {
					s.focusField(next)
					s.w.op(opFocus, next.hostID)
				}
			}
		}
		hostTouch(touchEnded, float64(at.X), float64(at.Y), endedAt)
		s.flush()
	})
}

func (t *testSurface) RightClick(driver.Point) error {
	return errors.New("a watch has no secondary click: use LongPress")
}

// Drag moves a touch along path, 10 points a frame, then rests still
// before it lifts, so it ends without a fling.
func (t *testSurface) Drag(path []driver.Point) error {
	if len(path) == 0 {
		return errors.New("an empty drag")
	}
	if watchHIDReady() {
		if t.dragNativeHID(path) {
			return nil
		}
	}
	t.dragFallback(path)
	return nil
}

// dragNativeHID posts a real finger drag along path into SwiftUI's hosting
// UIWindow. Returns false if HID delivery did not reach SwiftUI.
func (t *testSurface) dragNativeHID(path []driver.Point) bool {
	steps := watchDragSteps(path)
	if len(steps) == 0 {
		return false
	}
	var (
		seq    uint64
		posted bool
	)
	onMainSync(func() {
		seq = s.inputGate.beginNativeTouch()
		posted = postWatchHIDTouchOnMain(steps[0], watchHIDDown)
	})
	if !posted {
		onMainSync(func() { s.inputGate.endNativeTouch() })
		watchHIDBroken.Store(true)
		return false
	}
	time.Sleep(100 * time.Millisecond)
	for _, pt := range steps[1:] {
		onMainSync(func() {
			posted = postWatchHIDTouchOnMain(pt, watchHIDMove)
		})
		if !posted {
			onMainSync(func() { s.inputGate.endNativeTouch() })
			watchHIDBroken.Store(true)
			return false
		}
		time.Sleep(16 * time.Millisecond)
	}
	last := steps[len(steps)-1]
	for range 6 {
		onMainSync(func() {
			postWatchHIDTouchOnMain(last, watchHIDMove)
		})
		time.Sleep(16 * time.Millisecond)
	}
	onMainSync(func() {
		posted = postWatchHIDTouchOnMain(last, watchHIDUp)
	})
	if !posted {
		onMainSync(func() { s.inputGate.endNativeTouch() })
		watchHIDBroken.Store(true)
		return false
	}
	seen, _ := waitNativeTouchEnd(seq, 250*time.Millisecond)
	onMainSync(func() { s.inputGate.endNativeTouch() })
	if !seen {
		watchHIDBroken.Store(true)
		return false
	}
	return true
}

func (t *testSurface) dragFallback(path []driver.Point) {
	steps := watchDragSteps(path)
	if len(steps) == 0 {
		return
	}
	touchAt(touchBegan, steps[0])
	time.Sleep(100 * time.Millisecond)
	for _, pt := range steps[1:] {
		touchAt(touchMoved, pt)
		time.Sleep(16 * time.Millisecond)
	}
	last := steps[len(steps)-1]
	for range 6 {
		touchAt(touchMoved, last)
		time.Sleep(16 * time.Millisecond)
	}
	var panActive, blocked bool
	onMainSync(func() {
		panActive = s.touch.panning
		blocked = len(s.blocked) > 0
	})
	touchAt(touchEnded, last)
	if delta, ok := resolveWatchDragFallback(panActive, blocked, steps[0], last); ok {
		scriptedOnMain(func() {
			root := s.view(s.root)
			if root == nil {
				return
			}
			if v := hit(root, steps[0], func(v *view) bool { return v.kind == driver.KindScroll && !v.scroll.Behavior.NoScroll }); v != nil {
				to := clampOffset(v, driver.Point{X: v.offset.X + delta.X, Y: v.offset.Y + delta.Y})
				s.inputGate.expectScroll(uint32(v.hostID))
				s.w.op(opScrollTo, v.hostID)
				s.w.f64(float64(to.X), float64(to.Y))
				s.w.flag(false)
				s.flush()
			}
		})
		time.Sleep(100 * time.Millisecond)
	}
}

// Scroll moves the innermost wheel-handling view or scroll view under at by
// delta, using native simulator HID finger drags when available so SwiftUI's
// ScrollView responds as to a finger.
func (t *testSurface) Scroll(at driver.Point, delta driver.Point) error {
	var (
		err          error
		handledWheel bool
		scrollID     driver.NodeID
		scrollBounds driver.Bounds
		surfaceSize  driver.Size
		wantOffset   driver.Point
		rem          driver.Point
	)
	scriptedOnMain(func() {
		root := s.view(s.root)
		if root == nil {
			err = errors.New("nothing shows")
			return
		}
		v := hit(root, at, func(v *view) bool {
			return v.events.Has(driver.EventWheel) || v.kind == driver.KindScroll
		})
		if v == nil {
			err = fmt.Errorf("no scroll view at %v", at)
			return
		}
		if v.kind != driver.KindScroll && v.events.Has(driver.EventWheel) {
			local := s.local(v, at)
			v.report(driver.Event{Kind: driver.EventWheel, Phase: driver.GestureBegan, Point: local, Page: at, Precise: true})
			v.report(driver.Event{
				Kind:    driver.EventWheel,
				Phase:   driver.GestureChanged,
				Point:   local,
				Page:    at,
				Delta:   driver.Point{X: -delta.X, Y: -delta.Y},
				Precise: true,
			})
			v.report(driver.Event{Kind: driver.EventWheel, Phase: driver.GestureEnded, Point: local, Page: at, Precise: true})
			s.flush()
			handledWheel = true
			return
		}
		scrollID = v.id
		surfaceSize = s.size
		scrollBounds, _ = s.surfaceBounds(v.watchNode)
		wantOffset = clampOffset(v, driver.Point{X: v.offset.X + delta.X, Y: v.offset.Y + delta.Y})
		rem = driver.Point{X: wantOffset.X - v.offset.X, Y: wantOffset.Y - v.offset.Y}
	})
	if err != nil || handledWheel || (rem.X == 0 && rem.Y == 0) {
		return err
	}
	if watchHIDReady() {
		segs := planWatchScrollDrags(surfaceSize, scrollBounds, at, rem)
		for _, seg := range segs {
			var before driver.Point
			onMainSync(func() {
				if v := s.view(scrollID); v != nil {
					before = v.offset
				}
			})
			if !t.dragNativeHID(seg) {
				return t.scrollFallback(scrollID, wantOffset)
			}
			// Wait briefly for SwiftUI's scroll phase to settle before checking progress.
			deadline := time.Now().Add(250 * time.Millisecond)
			for {
				var busy bool
				onMainSync(func() {
					if v := s.view(scrollID); v != nil {
						busy = v.phase == phaseDragging || v.phase == phaseGliding || v.phase == phaseAnimating
					}
				})
				if !busy || time.Now().After(deadline) {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			var after driver.Point
			var gone bool
			onMainSync(func() {
				v := s.view(scrollID)
				if v == nil || v.deleted {
					gone = true
					return
				}
				after = v.offset
			})
			if gone {
				return nil
			}
			if math.Abs(float64(after.X-wantOffset.X)) <= 2 && math.Abs(float64(after.Y-wantOffset.Y)) <= 2 {
				return nil
			}
			if math.Abs(float64(after.X-before.X)) < 0.5 && math.Abs(float64(after.Y-before.Y)) < 0.5 {
				return nil
			}
		}
		return nil
	}
	return t.scrollFallback(scrollID, wantOffset)
}

func (t *testSurface) scrollFallback(scrollID driver.NodeID, to driver.Point) error {
	scriptedOnMain(func() {
		v := s.view(scrollID)
		if v == nil || v.deleted {
			return
		}
		clamped := clampOffset(v, to)
		s.inputGate.expectScroll(uint32(v.hostID))
		s.w.op(opScrollTo, v.hostID)
		s.w.f64(float64(clamped.X), float64(clamped.Y))
		s.w.flag(false)
		s.flush()
	})
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (t *testSurface) Hover(driver.Point) error { return nil }

// Type enters text into the focused field, a character at a time.
func (t *testSurface) Type(text string) error {
	for _, r := range text {
		var err error
		scriptedOnMain(func() {
			v := s.focused
			if v == nil {
				err = errors.New("no field has the focus")
				return
			}
			s.enter(v, v.value+string(r))
			s.sendInput(v)
			s.flush()
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *testSurface) Compose(string) error { return testhost.ErrNoInputMethod }
func (t *testSurface) Commit(string) error  { return testhost.ErrNoInputMethod }

// Key presses Enter, which submits the focused field, or Backspace,
// which deletes its last character; a watch has no other keys.
func (t *testSurface) Key(key string) error {
	var err error
	scriptedOnMain(func() {
		v := s.focused
		if v == nil {
			err = errors.New("no field has the focus")
			return
		}
		switch strings.ToLower(key) {
		case "enter", "return":
			v.report(driver.Event{Kind: driver.EventSubmitEditing, Text: v.value})
		case "backspace":
			if _, n := utf8.DecodeLastRuneInString(v.value); n > 0 {
				s.enter(v, v.value[:len(v.value)-n])
				s.sendInput(v)
			}
		default:
			err = fmt.Errorf("a watch has no %s key", key)
		}
		s.flush()
	})
	return err
}

func (t *testSurface) Back() error {
	s.engine.Dispatch(driver.Event{Kind: driver.EventBack})
	return nil
}

// Answer answers the alert as its button would, and takes it away.
func (t *testSurface) Answer(id uint32, index int) error {
	var err error
	scriptedOnMain(func() {
		actions, ok := s.alerts[id]
		if !ok {
			err = fmt.Errorf("no alert %d shows", id)
			return
		}
		if index < -1 || index >= len(actions) {
			err = fmt.Errorf("the alert has no button %d", index)
			return
		}
		s.w.op(opDismiss, driver.NodeID(id))
		hostAlert(id, int32(index))
		s.flush()
	})
	return err
}

func (t *testSurface) ChooseMenu(uint32) error     { return testhost.ErrNoMenu }
func (t *testSurface) ChooseDockMenu(uint32) error { return testhost.ErrNoMenu }
func (t *testSurface) ChoosePopup(uint32) error    { return testhost.ErrNoMenu }

// Screenshot has the host render the surface, as a PNG.
func (t *testSurface) Screenshot() ([]byte, error) {
	var png []byte
	onMainSync(func() {
		snapshot = nil
		purego.SyscallN(snapshotFn)
		png, snapshot = snapshot, nil
	})
	if len(png) == 0 {
		return nil, errors.New("the host drew nothing")
	}
	return png, nil
}

func (t *testSurface) Animating() bool {
	var busy bool
	onMainSync(func() {
		scrollBusy := s.inputGate.nativeTouchActive > 0
		if !scrollBusy {
			for _, v := range s.views {
				if v != nil && (v.phase == phaseDragging || v.phase == phaseGliding || v.phase == phaseAnimating) {
					scrollBusy = true
					break
				}
			}
		}
		busy = testSurfaceBusy(s.batches.Len(), s.animator.Ending(), len(s.motions), scrollBusy)
	})
	return busy
}

// Resize does nothing: a watch's screen is the size it is.
func (t *testSurface) Resize(driver.Size) error { return nil }

func (t *testSurface) SetEnvironment(env driver.Environment) error {
	if t.test == nil {
		return errors.New("a running app follows the system's settings")
	}
	t.test.pin.Set(&env)
	s.engine.Repin()
	return nil
}

func (t *testSurface) Sync() { s.engine.Sync() }
