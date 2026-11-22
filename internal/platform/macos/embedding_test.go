//go:build darwin && !ios

package macos

import (
	"sync"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// TestEmbeddedViewCloseIsIdempotent closes an embedded view twice, while its app runs, after its surface stopped and after its engine ended on a fault, and checks that the native view is released exactly once.
func TestEmbeddedViewCloseIsIdempotent(t *testing.T) {
	for _, end := range []string{"running", "stopped", "destroyed"} {
		t.Run(end, func(t *testing.T) {
			onMain(func() {
				loadAppKit()
				// Each step drains its own pool, so the retain counts below hold only real references.
				drained := func(fn func()) {
					pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
					fn()
					pool.Send(sel("drain"))
				}
				var parent, native objc.ID
				var v *View
				drained(func() {
					parent = objc.ID(classView).Send(selNew)
					// No window is needed to exercise native ownership: the host may close a view before its engine starts, too.
					v = NewView(nil, Options{Width: 160, Height: 100})
					native = objc.ID(v.Handle())
					parent.Send(selAddSubview, native)
					// The test keeps a reference of its own, so a release too many shows in the retain count instead of freeing the view under the test.
					native.Send(sel("retain"))
				})
				defer parent.Send(selRelease)
				retained := func() uint { return uint(native.Send(sel("retainCount"))) }
				inHost := func() bool { return parent.Send(sel("subviews")).Send(sel("count")) == 1 }
				// The view's own reference, the host's and the test's.
				start := retained()
				drained(func() {
					switch end {
					case "stopped":
						v.s.close()
					case "destroyed":
						// The batch an engine that ends on a fault sends its root surface.
						v.s.apply(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemWindow, Window: &driver.WindowCommand{Action: driver.WindowDestroy}}}})
					}
				})
				var hosted bool
				drained(func() { hosted = inHost() })
				want := start
				if !hosted {
					want--
				}
				if got := retained(); got != want {
					// The view is gone or about to go: leak it rather than crash the process.
					t.Errorf("the app's end released the view the host still holds: retain count %d, want %d", got, want)
					return
				}
				defer native.Send(selRelease)
				drained(v.Close)
				// Close drops the view's own reference and the host's.
				want = start - 2
				if got := retained(); got != want {
					t.Errorf("Close left the view's retain count at %d, want %d", got, want)
				}
				if !v.s.closed || surfaces[v.s] || views[native] != nil {
					t.Error("Close left the surface or native view registered")
				}
				if v.Handle() != 0 {
					t.Error("Close left a native handle to the released view")
				}
				var count objc.ID
				drained(func() { count = parent.Send(sel("subviews")).Send(sel("count")) })
				if count != 0 {
					t.Errorf("Close left %d views in the host", count)
				}
				drained(v.Close)
				if got := retained(); got != want {
					t.Errorf("a second Close changed the view's retain count from %d to %d", want, got)
				}
			})
		})
	}
}

// embeddedScrollFixture mounts an embedded NSGo view inside a real AppKit
// NSScrollView in an NSWindow, with a plain region, a scrollable vertical
// Scroll, a scrollable horizontal Scroll and a non-overflowing Scroll.
type embeddedScrollFixture struct {
	window     objc.ID
	hostScroll objc.ID
	hostClip   objc.ID
	hostDoc    objc.ID
	view       *View
	surface    *surface
	vScroll    *view
	hScroll    *view
	fitScroll  *view
}

var (
	testScrollCGOnce         sync.Once
	testCGCreateScrollWheel2 func(source uintptr, units uint32, count uint32, w1, w2, w3 int32) uintptr
	testCGSetIntField        func(event uintptr, field uint32, value int64)
	testCGSetLocation        func(event uintptr, p apple.CGPoint)
)

func loadTestScrollCG() {
	testScrollCGOnce.Do(func() {
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_NOW)
		if err != nil {
			panic(err)
		}
		purego.RegisterLibFunc(&testCGCreateScrollWheel2, cg, "CGEventCreateScrollWheelEvent2")
		purego.RegisterLibFunc(&testCGSetIntField, cg, "CGEventSetIntegerValueField")
		purego.RegisterLibFunc(&testCGSetLocation, cg, "CGEventSetLocation")
	})
}

func newEmbeddedScrollFixture(t *testing.T) *embeddedScrollFixture {
	t.Helper()
	var f *embeddedScrollFixture
	onMain(func() {
		loadAppKit()
		loadTestScrollCG()
		f = &embeddedScrollFixture{}
		const titled, backingBuffered = 1, 2
		f.window = objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(
			sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(200, 200, 300, 300), uint(titled), uint(backingBuffered), false)
		f.window.Send(sel("setReleasedWhenClosed:"), false)

		f.hostScroll = objc.ID(apple.Class("NSScrollView")).Send(selAlloc).Send(
			sel("initWithFrame:"), apple.Rect(0, 0, 300, 300))
		f.hostScroll.Send(sel("setHasVerticalScroller:"), true)
		f.hostScroll.Send(sel("setHasHorizontalScroller:"), false)
		// Flipped document view so origin.y grows downward as the host scrolls down.
		f.hostDoc = objc.ID(classContainer).Send(selAlloc).Send(
			sel("initWithFrame:"), apple.Rect(0, 0, 300, 1200))
		f.hostScroll.Send(sel("setDocumentView:"), f.hostDoc)
		f.hostClip = f.hostScroll.Send(sel("contentView"))

		app := func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(300), Height: core.Pt(500)}, Children: []core.Node{
				core.View{TestID: "plain", Style: core.Style{Width: core.Pt(300), Height: core.Pt(100)}},
				core.Scroll{TestID: "vscroll", Style: core.Style{Width: core.Pt(300), Height: core.Pt(150)}, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(300), Height: core.Pt(500)}},
				}},
				core.Scroll{TestID: "hscroll", Horizontal: true, Style: core.Style{Width: core.Pt(300), Height: core.Pt(120)}, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(800), Height: core.Pt(120)}},
				}},
				core.Scroll{TestID: "fitscroll", Style: core.Style{Width: core.Pt(300), Height: core.Pt(130)}, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(300), Height: core.Pt(80)}},
				}},
			}}
		}
		f.view = NewView(app, Options{Width: 300, Height: 500})
		f.surface = f.view.s
		native := objc.ID(f.view.Handle())
		native.Send(selSetFrame, apple.Rect(0, 100, 300, 500))
		f.hostDoc.Send(selAddSubview, native)
		f.window.Send(sel("setContentView:"), f.hostScroll)
		f.window.Send(sel("layoutIfNeeded"))
		// Prime NSApplication's event queue once so queued scroll events are
		// not discarded on the very first nextEventMatchingMask: call.
		appObj := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
		past := objc.ID(apple.Class("NSDate")).Send(sel("distantPast"))
		appObj.Send(sel("nextEventMatchingMask:untilDate:inMode:dequeue:"), ^uint64(0), past, apple.NSString("kCFRunLoopDefaultMode"), true)
	})
	t.Cleanup(func() {
		onMain(func() {
			if f.view != nil {
				f.view.Close()
			}
			if f.window != 0 {
				f.window.Send(sel("close"))
				f.window.Send(selRelease)
			}
			if f.hostScroll != 0 {
				f.hostScroll.Send(selRelease)
			}
			if f.hostDoc != 0 {
				f.hostDoc.Send(selRelease)
			}
		})
	})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var ready bool
		onMain(func() {
			until := objc.ID(apple.Class("NSDate")).Send(sel("dateWithTimeIntervalSinceNow:"), 0.02)
			objc.ID(apple.Class("NSRunLoop")).Send(sel("mainRunLoop")).Send(sel("runUntilDate:"), until)
			for _, v := range f.surface.views {
				if v == nil || !v.framed {
					continue
				}
				switch v.props.TestID {
				case "vscroll":
					f.vScroll = v
				case "hscroll":
					f.hScroll = v
				case "fitscroll":
					f.fitScroll = v
				}
			}
			ready = f.vScroll != nil && f.hScroll != nil && f.fitScroll != nil &&
				f.vScroll.sc.content.Height > 400 && f.hScroll.sc.content.Width > 700
		})
		if ready {
			return f
		}
	}
	t.Fatal("embedded scroll fixture did not lay out its scroll views")
	return nil
}

func (f *embeddedScrollFixture) hostY() float64 {
	return objc.Send[apple.CGRect](f.hostClip, selBounds).Origin.Y
}

func (f *embeddedScrollFixture) setHostY(y float64) {
	f.hostClip.Send(sel("scrollToPoint:"), apple.CGPoint{X: 0, Y: y})
	f.hostScroll.Send(sel("reflectScrolledClipView:"), f.hostClip)
}

func (f *embeddedScrollFixture) screenPointInEmbedded(x, y float64) (apple.CGPoint, apple.CGPoint) {
	winPt := objc.Send[apple.CGPoint](f.surface.native, sel("convertPoint:toView:"), apple.CGPoint{X: x, Y: y}, objc.ID(0))
	sp := objc.Send[apple.CGPoint](f.window, sel("convertPointToScreen:"), winPt)
	screens := objc.ID(apple.Class("NSScreen")).Send(sel("screens"))
	main := screens.Send(sel("objectAtIndex:"), uint(0))
	h := objc.Send[apple.CGRect](main, sel("frame")).Size.Height
	return winPt, apple.CGPoint{X: sp.X, Y: h - sp.Y}
}

// makeScrollEvent builds a synthetic scroll wheel NSEvent at (x, y) inside the
// embedded view.
func (f *embeddedScrollFixture) makeScrollEvent(x, y float64, dx, dy int32, continuous bool, phase, momentum int64) objc.ID {
	const unitPixel, unitLine = 0, 1
	const fieldContinuous, fieldScrollPhase, fieldMomentumPhase = 88, 99, 123
	unit := uint32(unitLine)
	if continuous {
		unit = unitPixel
	}
	ev := testCGCreateScrollWheel2(0, unit, 2, dy, dx, 0)
	defer apple.Release(ev)
	if continuous {
		testCGSetIntField(ev, fieldContinuous, 1)
	}
	if phase != 0 {
		testCGSetIntField(ev, fieldScrollPhase, phase)
	}
	if momentum != 0 {
		testCGSetIntField(ev, fieldMomentumPhase, momentum)
	}
	_, screenPt := f.screenPointInEmbedded(x, y)
	testCGSetLocation(ev, screenPt)
	return objc.ID(apple.Class("NSEvent")).Send(sel("eventWithCGEvent:"), ev)
}

// sendScroll dispatches a synthetic scroll wheel NSEvent through the surface's
// event monitor (filterEvent) and then to the hit view's scrollWheel:, matching
// AppKit's real delivery order.
func (f *embeddedScrollFixture) sendScroll(x, y float64, dx, dy int32, continuous bool, phase, momentum int64) {
	ns := f.makeScrollEvent(x, y, dx, dy, continuous, phase, momentum)
	f.deliverScroll(x, y, ns)
}

func (f *embeddedScrollFixture) deliverScroll(x, y float64, ns objc.ID) {
	if f.surface.filterEvent(ns) == 0 {
		return
	}
	winPt, _ := f.screenPointInEmbedded(x, y)
	hit := f.hostScroll.Send(sel("hitTest:"), objc.Send[apple.CGPoint](f.hostScroll.Send(selSuperview), selConvertPoint, winPt, objc.ID(0)))
	if hit == 0 {
		hit = f.surface.native
	}
	hit.Send(sel("scrollWheel:"), ns)
}

// dispatchTrackpadGesture posts a full trackpad gesture through NSApp's event
// queue so AppKit's NSScrollView tracking loop receives phaseChanged and
// phaseEnded just as with a real trackpad.
func (f *embeddedScrollFixture) dispatchTrackpadGesture(x, y float64, deltas [][2]int32, momentum [][2]int32, mayBegin bool) {
	// CoreGraphics kCGScrollPhase* and kCGMomentumScrollPhase* values (AppKit
	// translates these into NSEventPhase* bitmasks inside eventWithCGEvent:).
	const cgMayBegin, cgBegan, cgChanged, cgEnded = 128, 1, 2, 4
	const cgMomBegan, cgMomContinue, cgMomEnd = 1, 2, 3
	app := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
	add := func(dx, dy int32, phase, mom int64) {
		ev := f.makeScrollEvent(x, y, dx, dy, true, phase, mom)
		app.Send(sel("postEvent:atStart:"), ev, false)
	}
	if mayBegin {
		add(0, 0, cgMayBegin, 0)
	}
	add(0, 0, cgBegan, 0)
	for _, d := range deltas {
		add(d[0], d[1], cgChanged, 0)
	}
	add(0, 0, cgEnded, 0)
	for i, m := range momentum {
		mp := int64(cgMomContinue)
		if i == 0 {
			mp = cgMomBegan
		}
		add(m[0], m[1], 0, mp)
	}
	if len(momentum) > 0 {
		add(0, 0, 0, cgMomEnd)
	}
	const anyEventMask = ^uint64(0)
	const modeDefault = "kCFRunLoopDefaultMode"
	distantPast := objc.ID(apple.Class("NSDate")).Send(sel("distantPast"))
	for {
		ev := app.Send(sel("nextEventMatchingMask:untilDate:inMode:dequeue:"), anyEventMask, distantPast, apple.NSString(modeDefault), true)
		if ev == 0 {
			break
		}
		const typeScrollWheel = 22
		if objc.Send[uint](ev, sel("type")) == typeScrollWheel {
			f.deliverScroll(x, y, ev)
		} else {
			app.Send(sel("sendEvent:"), ev)
		}
	}
	// Let AppKit's NSScrollView flush any display-synced scroll updates.
	until := objc.ID(apple.Class("NSDate")).Send(sel("dateWithTimeIntervalSinceNow:"), 0.05)
	objc.ID(apple.Class("NSRunLoop")).Send(sel("mainRunLoop")).Send(sel("runUntilDate:"), until)
}

// TestEmbeddedScrollMouseWheelPassthrough verifies that mouse wheel events an
// embedded macOS view cannot consume pass up to an enclosing AppKit NSScrollView,
// while events an inner scroll view can take stay inside the embedded view.
func TestEmbeddedScrollMouseWheelPassthrough(t *testing.T) {
	f := newEmbeddedScrollFixture(t)

	t.Run("plain area passes wheel to host", func(t *testing.T) {
		onMain(func() {
			f.setHostY(100)
			f.surface.wheels.latch = nil
			// Point (150, 50) is inside the plain view; negative AppKit dy scrolls down.
			f.sendScroll(150, 50, 0, -4, false, 0, 0)
			if got := f.hostY(); got <= 100 {
				t.Fatalf("host NSScrollView did not scroll down over plain area: y=%v, want > 100", got)
			}
		})
	})

	t.Run("scrollable inner view consumes wheel", func(t *testing.T) {
		onMain(func() {
			f.setHostY(100)
			f.vScroll.scrollTo(driver.Point{Y: 0}, false)
			f.surface.wheels.latch = nil
			// Point (150, 175) is inside vscroll (y=100..250 in embedded view).
			f.sendScroll(150, 175, 0, -4, false, 0, 0)
			if got := f.vScroll.pos(); got <= 0 {
				t.Fatalf("inner vertical scroll view did not scroll: pos=%v, want > 0", got)
			}
			if got := f.hostY(); got != 100 {
				t.Fatalf("host NSScrollView scrolled when inner view consumed wheel: y=%v, want 100", got)
			}
		})
	})

	t.Run("inner view at top boundary passes upward wheel to host", func(t *testing.T) {
		onMain(func() {
			f.setHostY(200)
			f.vScroll.scrollTo(driver.Point{Y: 0}, false)
			f.surface.wheels.latch = nil
			// Positive AppKit dy scrolls up.
			f.sendScroll(150, 175, 0, 4, false, 0, 0)
			if got := f.vScroll.pos(); got != 0 {
				t.Fatalf("inner scroll moved past top: pos=%v, want 0", got)
			}
			if got := f.hostY(); got >= 200 {
				t.Fatalf("host NSScrollView did not scroll up: y=%v, want < 200", got)
			}
		})
	})

	t.Run("inner view at bottom boundary passes downward wheel to host", func(t *testing.T) {
		onMain(func() {
			f.setHostY(100)
			_, hi := f.vScroll.limits()
			f.vScroll.scrollTo(driver.Point{Y: hi}, false)
			f.surface.wheels.latch = nil
			f.sendScroll(150, 175, 0, -4, false, 0, 0)
			if got := f.vScroll.pos(); got != hi {
				t.Fatalf("inner scroll moved past bottom: pos=%v, want %v", got, hi)
			}
			if got := f.hostY(); got <= 100 {
				t.Fatalf("host NSScrollView did not scroll down: y=%v, want > 100", got)
			}
		})
	})

	t.Run("vertical wheel over inner horizontal scroll passes to vertical host", func(t *testing.T) {
		onMain(func() {
			f.setHostY(200)
			f.hScroll.scrollTo(driver.Point{X: 100}, false)
			f.surface.wheels.latch = nil
			// Point (150, 310) is inside hscroll (y=250..370 in embedded view).
			f.sendScroll(150, 310, 0, -4, false, 0, 0)
			if got := f.hScroll.pos(); got != 100 {
				t.Fatalf("horizontal inner scroll took vertical wheel: pos=%v, want 100", got)
			}
			if got := f.hostY(); got <= 200 {
				t.Fatalf("host NSScrollView did not scroll down over horizontal scroll: y=%v, want > 200", got)
			}
		})
	})

	t.Run("horizontal wheel over inner horizontal scroll stays inside", func(t *testing.T) {
		onMain(func() {
			f.setHostY(200)
			f.hScroll.scrollTo(driver.Point{X: 100}, false)
			f.surface.wheels.latch = nil
			f.sendScroll(150, 310, -4, 0, false, 0, 0)
			if got := f.hScroll.pos(); got <= 100 {
				t.Fatalf("horizontal inner scroll did not scroll: pos=%v, want > 100", got)
			}
			if got := f.hostY(); got != 200 {
				t.Fatalf("host NSScrollView moved during horizontal inner scroll: y=%v, want 200", got)
			}
		})
	})
}

// TestEmbeddedScrollTrackpadPassthrough verifies trackpad gestures and momentum
// inside an embedded macOS view hosted in an AppKit NSScrollView.
func TestEmbeddedScrollTrackpadPassthrough(t *testing.T) {
	f := newEmbeddedScrollFixture(t)

	t.Run("plain area passes gesture to host", func(t *testing.T) {
		onMain(func() {
			f.setHostY(100)
			f.dispatchTrackpadGesture(150, 50, [][2]int32{{0, -25}, {0, -25}}, nil, true)
			if got := f.hostY(); got <= 100 {
				t.Fatalf("host NSScrollView did not scroll during trackpad gesture (with mayBegin): y=%v, want > 100", got)
			}
			f.setHostY(100)
			f.dispatchTrackpadGesture(150, 50, [][2]int32{{0, -25}, {0, -25}}, nil, false)
			if got := f.hostY(); got <= 100 {
				t.Fatalf("host NSScrollView did not scroll during trackpad gesture: y=%v, want > 100", got)
			}
		})
	})

	t.Run("scrollable inner view consumes gesture and momentum without scrolling host", func(t *testing.T) {
		onMain(func() {
			f.setHostY(150)
			f.vScroll.scrollTo(driver.Point{Y: 0}, false)
			f.vScroll.sc.resampled.Reset()
			f.dispatchTrackpadGesture(150, 175, [][2]int32{{0, -25}, {0, -25}}, [][2]int32{{0, -15}, {0, -10}}, true)
			f.vScroll.showGoal()
			if got := f.vScroll.pos(); got <= 0 {
				t.Fatalf("inner scroll view did not move: pos=%v, want > 0", got)
			}
			if got := f.hostY(); got != 150 {
				t.Fatalf("host NSScrollView moved while inner scroll consumed gesture: y=%v, want 150", got)
			}
		})
	})

	t.Run("inner view at top boundary passes upward trackpad gesture to host without rubber-banding", func(t *testing.T) {
		onMain(func() {
			f.setHostY(200)
			f.vScroll.scrollTo(driver.Point{Y: 0}, false)
			f.vScroll.sc.resampled.Reset()
			f.dispatchTrackpadGesture(150, 175, [][2]int32{{0, 25}, {0, 25}}, [][2]int32{{0, 15}}, true)
			f.vScroll.showGoal()
			if got := f.vScroll.pos(); got != 0 {
				t.Fatalf("inner scroll view rubber-banded instead of passing to host: pos=%v, want 0", got)
			}
			if got := f.hostY(); got >= 200 {
				t.Fatalf("host NSScrollView did not scroll up: y=%v, want < 200", got)
			}
		})
	})

	t.Run("non-overflowing inner scroll view passes trackpad gesture to host", func(t *testing.T) {
		onMain(func() {
			f.setHostY(300)
			f.fitScroll.scrollTo(driver.Point{Y: 0}, false)
			f.fitScroll.sc.resampled.Reset()
			// Point (150, 420) is inside fitscroll (y=370..500 in embedded view).
			f.dispatchTrackpadGesture(150, 420, [][2]int32{{0, -30}, {0, -30}}, nil, false)
			f.fitScroll.showGoal()
			if got := f.fitScroll.pos(); got != 0 {
				t.Fatalf("non-overflowing scroll moved: pos=%v, want 0", got)
			}
			if got := f.hostY(); got <= 300 {
				t.Fatalf("host NSScrollView did not scroll over non-overflowing inner scroll: y=%v, want > 300", got)
			}
		})
	})

	t.Run("vertical trackpad gesture over inner horizontal scroll passes to vertical host", func(t *testing.T) {
		onMain(func() {
			f.setHostY(200)
			f.hScroll.scrollTo(driver.Point{X: 100}, false)
			f.hScroll.sc.resampled.Reset()
			f.dispatchTrackpadGesture(150, 310, [][2]int32{{0, -30}, {0, -20}}, nil, false)
			f.hScroll.showGoal()
			if got := f.hScroll.pos(); got != 100 {
				t.Fatalf("horizontal inner scroll moved on vertical trackpad gesture: pos=%v, want 100", got)
			}
			if got := f.hostY(); got <= 200 {
				t.Fatalf("host NSScrollView did not scroll down over horizontal inner scroll: y=%v, want > 200", got)
			}
		})
	})

	t.Run("passed gesture stays locked to host on direction reversal", func(t *testing.T) {
		onMain(func() {
			f.setHostY(200)
			f.vScroll.scrollTo(driver.Point{Y: 0}, false)
			f.vScroll.sc.resampled.Reset()
			// Begin upward (host scrolls up), then reverse downward in the same gesture:
			// inner scroll must stay locked out until the gesture ends.
			f.dispatchTrackpadGesture(150, 175, [][2]int32{{0, 30}, {0, -20}}, nil, false)
			f.vScroll.showGoal()
			if got := f.vScroll.pos(); got != 0 {
				t.Fatalf("inner scroll hijacked host gesture on direction reversal: pos=%v, want 0", got)
			}
		})
	})

	t.Run("inner gesture hitting edge swallows its own momentum instead of flinging host", func(t *testing.T) {
		onMain(func() {
			f.setHostY(200)
			f.vScroll.scrollTo(driver.Point{Y: 10}, false)
			f.vScroll.sc.resampled.Reset()
			// Pull upward from y=10 past y=0 so inner enters spring-back on release,
			// then send momentum events for that same flick.
			f.dispatchTrackpadGesture(150, 175, [][2]int32{{0, 30}, {0, 30}}, [][2]int32{{0, 20}, {0, 15}}, false)
			f.vScroll.showGoal()
			if got := f.hostY(); got != 200 {
				t.Fatalf("host NSScrollView received swallowed momentum from inner spring-back: y=%v, want 200", got)
			}
			if f.surface.wheels.momentum != nil || f.surface.wheels.momentumSwallowed {
				t.Fatalf("stale momentum state left after momentum end: momentum=%v swallowed=%v",
					f.surface.wheels.momentum != nil, f.surface.wheels.momentumSwallowed)
			}
		})
	})
}

type recordedScroll struct {
	phase    uint
	momentum uint
	dx, dy   float64
}

var (
	customHostScrollOnce  sync.Once
	classCustomScrollHost objc.Class
	customHostRecords     = map[objc.ID]*[]recordedScroll{}
)

func customScrollHostClass() objc.Class {
	customHostScrollOnce.Do(func() {
		var err error
		classCustomScrollHost, err = objc.RegisterClass("NsgoTestScrollHostView", apple.Class("NSView"), nil, nil, []objc.MethodDef{
			{Cmd: sel("isFlipped"), Fn: func(objc.ID, objc.SEL) bool { return true }},
			{Cmd: sel("scrollWheel:"), Fn: func(self objc.ID, _ objc.SEL, event objc.ID) {
				if rec := customHostRecords[self]; rec != nil {
					*rec = append(*rec, recordedScroll{
						phase:    objc.Send[uint](event, sel("phase")),
						momentum: objc.Send[uint](event, sel("momentumPhase")),
						dx:       objc.Send[float64](event, sel("scrollingDeltaX")),
						dy:       objc.Send[float64](event, sel("scrollingDeltaY")),
					})
				}
			}},
		})
		if err != nil {
			panic(err)
		}
	})
	return classCustomScrollHost
}

// TestEmbeddedScrollCustomHostResponderAndMomentum checks exact phase and
// momentum forwarding order to a custom macOS host responder, including
// replaying zero-delta phaseBegan before the first forwarded phaseChanged,
// swallowing inner spring-back momentum, and clearing stale momentum on a new
// gesture.
func TestEmbeddedScrollCustomHostResponderAndMomentum(t *testing.T) {
	var window, host objc.ID
	var viewHandle *View
	var s *surface
	var vScroll *view
	var got []recordedScroll

	onMain(func() {
		loadAppKit()
		loadTestScrollCG()
		const titled, backingBuffered = 1, 2
		window = objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(
			sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(200, 200, 200, 200), uint(titled), uint(backingBuffered), false)
		window.Send(sel("setReleasedWhenClosed:"), false)

		host = objc.ID(customScrollHostClass()).Send(selAlloc).Send(
			sel("initWithFrame:"), apple.Rect(0, 0, 200, 200))
		customHostRecords[host] = &got

		app := func(cx *core.Scope) core.Node {
			return core.Scroll{TestID: "inner", Style: core.Style{Width: core.Pt(200), Height: core.Pt(200)}, Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(200), Height: core.Pt(600)}},
			}}
		}
		viewHandle = NewView(app, Options{Width: 200, Height: 200})
		s = viewHandle.s
		native := objc.ID(viewHandle.Handle())
		host.Send(selAddSubview, native)
		window.Send(sel("setContentView:"), host)
		window.Send(sel("layoutIfNeeded"))
	})
	t.Cleanup(func() {
		onMain(func() {
			delete(customHostRecords, host)
			if viewHandle != nil {
				viewHandle.Close()
			}
			if window != 0 {
				window.Send(sel("close"))
				window.Send(selRelease)
			}
			if host != 0 {
				host.Send(selRelease)
			}
		})
	})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var ready bool
		onMain(func() {
			until := objc.ID(apple.Class("NSDate")).Send(sel("dateWithTimeIntervalSinceNow:"), 0.02)
			objc.ID(apple.Class("NSRunLoop")).Send(sel("mainRunLoop")).Send(sel("runUntilDate:"), until)
			for _, v := range s.views {
				if v != nil && v.framed && v.props.TestID == "inner" && v.sc.content.Height > 500 {
					vScroll = v
					ready = true
				}
			}
		})
		if ready {
			break
		}
	}
	if vScroll == nil {
		t.Fatal("custom host fixture did not lay out inner scroll view")
	}

	makeEv := func(dx, dy int32, phase, mom int64) objc.ID {
		const fieldContinuous, fieldScrollPhase, fieldMomentumPhase = 88, 99, 123
		ev := testCGCreateScrollWheel2(0, 0, 2, dy, dx, 0)
		defer apple.Release(ev)
		testCGSetIntField(ev, fieldContinuous, 1)
		if phase != 0 {
			testCGSetIntField(ev, fieldScrollPhase, phase)
		}
		if mom != 0 {
			testCGSetIntField(ev, fieldMomentumPhase, mom)
		}
		winPt := objc.Send[apple.CGPoint](s.native, sel("convertPoint:toView:"), apple.CGPoint{X: 100, Y: 100}, objc.ID(0))
		sp := objc.Send[apple.CGPoint](window, sel("convertPointToScreen:"), winPt)
		screens := objc.ID(apple.Class("NSScreen")).Send(sel("screens"))
		main := screens.Send(sel("objectAtIndex:"), uint(0))
		h := objc.Send[apple.CGRect](main, sel("frame")).Size.Height
		testCGSetLocation(ev, apple.CGPoint{X: sp.X, Y: h - sp.Y})
		return objc.ID(apple.Class("NSEvent")).Send(sel("eventWithCGEvent:"), ev)
	}

	// deliver simulates both the window's local event monitor (wheelContinues
	// check + stale momentum clearing) and responder chain scrollWheel: delivery.
	deliver := func(ev objc.ID) {
		phase, mom := objc.Send[uint](ev, sel("phase")), objc.Send[uint](ev, sel("momentumPhase"))
		if mom == 0 && phase&(phaseBegan|phaseMayBegin) != 0 {
			s.endMomentum()
			s.wheels.taker = nil
		}
		if s.wheelContinues(phase, mom) && s.wheelEvent(ev) {
			return
		}
		vScroll.native.Send(sel("scrollWheel:"), ev)
	}

	onMain(func() {
		const cgBegan, cgChanged, cgEnded = 1, 2, 4
		const cgMomBegan, cgMomContinue, cgMomEnd = 1, 2, 3

		// 1. Inner scroll at top boundary: upward gesture + momentum must replay
		//    phaseBegan before the first phaseChanged, then forward every event
		//    in order including phaseEnded and all momentum phases.
		vScroll.scrollTo(driver.Point{Y: 0}, false)
		got = nil
		deliver(makeEv(0, 0, cgBegan, 0))
		deliver(makeEv(0, 25, cgChanged, 0))
		deliver(makeEv(0, 15, cgChanged, 0))
		deliver(makeEv(0, 0, cgEnded, 0))
		deliver(makeEv(0, 12, 0, cgMomBegan))
		deliver(makeEv(0, 6, 0, cgMomContinue))
		deliver(makeEv(0, 0, 0, cgMomEnd))

		wantPhases := [][2]uint{
			{phaseBegan, 0},
			{phaseChanged, 0},
			{phaseChanged, 0},
			{phaseEnded, 0},
			{0, phaseBegan},
			{0, phaseChanged},
			{0, phaseEnded},
		}
		if len(got) != len(wantPhases) {
			t.Fatalf("forwarded events = %+v (len %d), want len %d", got, len(got), len(wantPhases))
		}
		for i, want := range wantPhases {
			if got[i].phase != want[0] || got[i].momentum != want[1] {
				t.Errorf("event[%d] = (phase=%d, mom=%d), want (phase=%d, mom=%d)",
					i, got[i].phase, got[i].momentum, want[0], want[1])
			}
		}
		if got[1].dy != 25 || got[2].dy != 15 || got[4].dy != 12 || got[5].dy != 6 {
			t.Errorf("forwarded deltas out of order: %+v", got)
		}

		// 2. Slow gesture consumed by inner scroll (no momentum follows), then a
		//    new host-owned gesture with momentum: stale inner momentum must NOT
		//    steal the host's momentum.
		got = nil
		vScroll.scrollTo(driver.Point{Y: 0}, false)
		deliver(makeEv(0, 0, cgBegan, 0))
		deliver(makeEv(0, -20, cgChanged, 0))
		deliver(makeEv(0, 0, cgEnded, 0))
		if len(got) != 0 {
			t.Fatalf("host received events for inner-consumed gesture: %+v", got)
		}
		// Reset inner to top and start a new upward gesture with momentum.
		vScroll.scrollTo(driver.Point{Y: 0}, false)
		deliver(makeEv(0, 0, cgBegan, 0))
		deliver(makeEv(0, 20, cgChanged, 0))
		deliver(makeEv(0, 0, cgEnded, 0))
		deliver(makeEv(0, 10, 0, cgMomBegan))
		deliver(makeEv(0, 0, 0, cgMomEnd))
		if len(got) != 5 || got[3].momentum != phaseBegan || got[4].momentum != phaseEnded {
			t.Fatalf("host did not receive momentum after prior inner slow gesture: %+v", got)
		}

		// 3. Inner scroll pulls past top edge and springs back: its momentum must
		//    be swallowed, not forwarded to the host.
		got = nil
		vScroll.scrollTo(driver.Point{Y: 10}, false)
		deliver(makeEv(0, 0, cgBegan, 0))
		deliver(makeEv(0, 30, cgChanged, 0))
		deliver(makeEv(0, 0, cgEnded, 0))
		deliver(makeEv(0, 15, 0, cgMomBegan))
		deliver(makeEv(0, 10, 0, cgMomContinue))
		deliver(makeEv(0, 0, 0, cgMomEnd))
		if len(got) != 0 {
			t.Fatalf("host received events from inner spring-back gesture/momentum: %+v", got)
		}
	})
}

// TestEmbeddedScrollInNonScrollingHostStillBounces checks that an embedded view
// inside a plain non-scrolling host NSWindow keeps rubber-banding at edges.
func TestEmbeddedScrollInNonScrollingHostStillBounces(t *testing.T) {
	onMain(func() {
		loadAppKit()
		const titled, backingBuffered = 1, 2
		win := objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(
			sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(200, 200, 200, 200), uint(titled), uint(backingBuffered), false)
		win.Send(sel("setReleasedWhenClosed:"), false)
		defer func() {
			win.Send(sel("close"))
			win.Send(selRelease)
		}()
		s := newSurface(Options{Width: 200, Height: 200}, true)
		defer func() {
			s.closed = true
			delete(surfaces, s)
			delete(views, s.native)
			s.native.Send(sel("removeFromSuperview"))
			s.native.Send(selRelease)
		}()
		win.Send(sel("contentView")).Send(selAddSubview, s.native)
		v := s.create(driver.KindScroll, 1)
		defer v.native.Send(selRelease)
		defer delete(views, v.native)
		s.views = []*view{nil, v}
		v.frame = driver.Bounds{Size: driver.Size{Width: 200, Height: 200}}
		v.sc.content = driver.Size{Width: 200, Height: 600}
		s.wheels.chain = []*view{v}
		if s.hostScrolls(false) {
			t.Fatal("plain non-scrolling host NSWindow reported hostScrolls(false) = true")
		}
		if got := s.gestureTarget(0, -20); got != v {
			t.Fatalf("embedded gestureTarget in non-scrolling host = %v, want %v", got, v)
		}
	})
}

// TestStandaloneScrollStillBouncesAtEdge checks that non-embedded standalone
// windows keep rubber-banding at scroll boundaries.
func TestStandaloneScrollStillBouncesAtEdge(t *testing.T) {
	onMain(func() {
		loadAppKit()
		s := &surface{native: objc.ID(classView).Send(selNew), animator: driver.NewAnimator(nil)}
		defer s.native.Send(selRelease)
		defer func() { s.closed = true }()
		v := s.create(driver.KindScroll, 1)
		defer v.native.Send(selRelease)
		defer delete(views, v.native)
		s.views = []*view{nil, v}
		v.frame = driver.Bounds{Size: driver.Size{Width: 100, Height: 200}}
		v.sc.content = driver.Size{Width: 100, Height: 600}
		s.wheels.chain = []*view{v}
		if got := s.gestureTarget(0, -20); got != v {
			t.Fatalf("standalone gestureTarget at top edge = %v, want %v", got, v)
		}
	})
}
