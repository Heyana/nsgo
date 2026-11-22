//go:build darwin && !ios

package macos

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

type recordedWebTracking struct {
	kind string
	at   apple.CGPoint
}

var (
	testTrackingObserverClass objc.Class
	testTrackingMu            sync.Mutex
	testTrackingEvents        []recordedWebTracking
)

func registerTestTrackingObserver() objc.Class {
	if testTrackingObserverClass != 0 {
		return testTrackingObserverClass
	}
	record := func(kind string) func(objc.ID, objc.SEL, objc.ID) {
		return func(_ objc.ID, _ objc.SEL, ev objc.ID) {
			pt := objc.Send[apple.CGPoint](ev, selLocationInWindow)
			testTrackingMu.Lock()
			testTrackingEvents = append(testTrackingEvents, recordedWebTracking{kind: kind, at: pt})
			testTrackingMu.Unlock()
		}
	}
	testTrackingObserverClass = register("NsgoTestWebTrackingObserver", apple.Class("NSObject"), nil, []objc.MethodDef{
		{Cmd: sel("mouseEntered:"), Fn: record("enter")},
		{Cmd: sel("mouseMoved:"), Fn: record("move")},
		{Cmd: sel("mouseExited:"), Fn: record("exit")},
	})
	return testTrackingObserverClass
}

func takeTestTrackingEvents() []recordedWebTracking {
	testTrackingMu.Lock()
	out := append([]recordedWebTracking(nil), testTrackingEvents...)
	testTrackingEvents = nil
	testTrackingMu.Unlock()
	return out
}

// TestWebViewUnderOverlayPickAndTracking verifies surface.pick, hitAt, cursorAt, and pickWeb against a real WKWebView covered by a sibling overlay view: covered points hit and take the overlay's cursor without hovering the web view, covering a hovered web view exits it outside its bounds, uncovering it under a resting pointer re-enters and moves it, and deleting a hovered web view clears webOver.
func TestWebViewUnderOverlayPickAndTracking(t *testing.T) {
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))

		window := objc.ID(classWindow).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(0, 0, 300, 200), uint(0), uint(2), false)
		window.Send(sel("setReleasedWhenClosed:"), false)
		s := newSurface(Options{Width: 300, Height: 200}, false)
		s.views = make([]*view, 4)
		s.animator = driver.NewAnimator(func(driver.Event) {})
		s.window = window
		window.Send(sel("setContentView:"), s.native)
		defer func() {
			s.close()
			delete(views, s.native)
			s.native.Send(selRelease)
			window.Send(sel("close"))
			window.Send(selRelease)
		}()

		// Mount a KindWeb view filling (0..300, 0..200).
		webV := s.create(driver.KindWeb, 1)
		s.views[1] = webV
		webV.native.Send(selSetFrame, apple.Rect(0, 0, 300, 200))
		webV.frame = driver.Bounds{Size: driver.Size{Width: 300, Height: 200}}
		webV.framed = true
		s.native.Send(selAddSubview, webV.native)
		webV.web.Configure(&driver.WebConfig{Source: driver.WebSource{HTML: "<p>hi</p>"}})
		webNative := webV.web.View()
		views[webNative] = webV
		defer func() {
			if s.views[1] != nil {
				s.delete(webV, false)
			}
		}()

		// Mount a sibling overlay covering the left half (0..150, 0..200).
		overlayV := s.create(driver.KindView, 2)
		s.views[2] = overlayV
		overlayV.style.Cursor = driver.CursorCrosshair
		overlayV.native.Send(selSetFrame, apple.Rect(0, 0, 150, 200))
		overlayV.frame = driver.Bounds{Size: driver.Size{Width: 150, Height: 200}}
		overlayV.framed = true
		s.native.Send(selAddSubview, overlayV.native)
		defer func() {
			if s.views[2] != nil {
				s.delete(overlayV, false)
			}
		}()

		// Attach a recording tracking area to the WKWebView so we observe the exact synthetic events pickWeb delivers to WKWebView's tracking area owners without depending on WindowServer screen-lock state.
		obs := objc.ID(registerTestTrackingObserver()).Send(selNew)
		defer obs.Send(selRelease)
		const enteredAndExited, mouseMovedOpt, activeAlways, inVisibleRect = 0x01, 0x02, 0x80, 0x200
		area := objc.ID(apple.Class("NSTrackingArea")).Send(selAlloc).Send(
			sel("initWithRect:options:owner:userInfo:"),
			apple.Rect(0, 0, 0, 0),
			uint(enteredAndExited|mouseMovedOpt|activeAlways|inVisibleRect),
			obs, objc.ID(0))
		webNative.Send(sel("addTrackingArea:"), area)
		defer func() {
			webNative.Send(sel("removeTrackingArea:"), area)
			area.Send(selRelease)
		}()
		takeTestTrackingEvents()

		up := s.native.Send(selSuperview)
		toWin := func(x, y float64) apple.CGPoint {
			return objc.Send[apple.CGPoint](s.native, sel("convertPoint:toView:"), apple.CGPoint{X: x, Y: y}, objc.ID(0))
		}
		coveredPt := toWin(75, 100)
		uncoveredPt := toWin(225, 100)

		// 1. Covered by overlay: hitAt lands on overlay, cursorAt owns Crosshair, webViewAt is nil, and pick sends nothing to WKWebView.
		hitCov := s.hitAt(coveredPt)
		if hitCov != overlayV.native {
			t.Fatalf("covered hitAt = %#x, want overlay %#x", uintptr(hitCov), uintptr(overlayV.native))
		}
		if got := webViewAt(hitCov, up); got != nil {
			t.Fatalf("covered webViewAt = %v, want nil", got)
		}
		if cur, _, own := cursorAt(hitCov, up, coveredPt); !own || cur != driver.CursorCrosshair {
			t.Fatalf("covered cursorAt = (%v, own=%v), want (Crosshair, true)", cur, own)
		}
		s.pick(coveredPt, true, true)
		if s.webOver != nil {
			t.Fatalf("covered webOver = %v, want nil", s.webOver)
		}
		if evs := takeTestTrackingEvents(); len(evs) != 0 {
			t.Fatalf("covered pick dispatched %+v, want none", evs)
		}

		// 2. Uncovered half (moving=true): hitAt lands in WKWebView, cursorAt yields to WebKit (own=false), and pick primes mouseEntered: without a duplicate mouseMoved:.
		hitUncov := s.hitAt(uncoveredPt)
		if got := webViewAt(hitUncov, up); got != webV {
			t.Fatalf("uncovered webViewAt = %v, want webV", got)
		}
		if _, _, own := cursorAt(hitUncov, up, uncoveredPt); own {
			t.Fatalf("uncovered cursorAt reported own=true, want false (WebKit owns cursor)")
		}
		s.pick(uncoveredPt, true, true)
		if s.webOver != webV {
			t.Fatalf("uncovered webOver = %v, want webV", s.webOver)
		}
		if evs := takeTestTrackingEvents(); len(evs) != 1 || evs[0].kind != "enter" || evs[0].at != uncoveredPt {
			t.Fatalf("uncovered moving pick dispatched %+v, want [enter at %+v]", evs, uncoveredPt)
		}

		// 3. Move back onto covering overlay: pick clears webOver and sends mouseExited: at a point outside WKWebView's bounds.
		s.pick(coveredPt, true, true)
		if s.webOver != nil {
			t.Fatalf("re-covered webOver = %v, want nil", s.webOver)
		}
		evs := takeTestTrackingEvents()
		if len(evs) != 1 || evs[0].kind != "exit" {
			t.Fatalf("re-covered pick dispatched %+v, want [exit]", evs)
		}
		localExit := objc.Send[apple.CGPoint](webNative, selConvertPoint, evs[0].at, objc.ID(0))
		if localExit.X >= 0 && localExit.Y >= 0 {
			t.Fatalf("mouseExited: local point = %+v, want outside WKWebView bounds", localExit)
		}

		// 4. Overlay closes under a resting pointer at (75, 100) (pickHere): pick sets webOver and sends both mouseEntered: and mouseMoved: at coveredPt so the page restores :hover and its cursor without a move.
		overlayV.native.Send(selSetHidden, true)
		p := coveredPt
		s.testPointer = &p
		s.pickHere()
		if s.webOver != webV {
			t.Fatalf("after overlay hidden: webOver = %v, want webV", s.webOver)
		}
		evs = takeTestTrackingEvents()
		if len(evs) != 2 || evs[0].kind != "enter" || evs[1].kind != "move" || evs[0].at != coveredPt || evs[1].at != coveredPt {
			t.Fatalf("uncovered resting pick dispatched %+v, want [enter, move at %+v]", evs, coveredPt)
		}

		// 5. Overlay opens again under the resting pointer: pick exits webV, and a cursorUpdate: sent to WKWebView routes through its superview back to surface.pickHere without re-hovering webV.
		overlayV.native.Send(selSetHidden, false)
		s.pickHere()
		if s.webOver != nil {
			t.Fatalf("after overlay shown again: webOver = %v, want nil", s.webOver)
		}
		if evs := takeTestTrackingEvents(); len(evs) != 1 || evs[0].kind != "exit" {
			t.Fatalf("re-covered resting pick dispatched %+v, want [exit]", evs)
		}
		curEv := objc.ID(apple.Class("NSEvent")).Send(
			sel("enterExitEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:trackingNumber:userData:"),
			uint(17), coveredPt, uint(0), uptime(), objc.Send[int](window, sel("windowNumber")), objc.ID(0), 0, 0, uintptr(0))
		webNative.Send(sel("cursorUpdate:"), curEv)
		if s.webOver != nil {
			t.Fatalf("after covered cursorUpdate: webOver = %v, want nil", s.webOver)
		}
		if evs := takeTestTrackingEvents(); len(evs) != 0 {
			t.Fatalf("covered cursorUpdate: dispatched %+v, want none", evs)
		}

		// 6. Deleting a hovered web view clears webOver cleanly.
		overlayV.native.Send(selSetHidden, true)
		s.pickHere()
		takeTestTrackingEvents()
		if s.webOver != webV {
			t.Fatalf("before delete: webOver = %v, want webV", s.webOver)
		}
		s.delete(webV, false)
		if s.webOver != nil {
			t.Fatalf("after delete: webOver = %v, want nil", s.webOver)
		}
	})
}

// TestWebViewUnderOverlayIgnoresHoverAndCursor checks in a live macOS window that a WKWebView covered by an overlay takes no hover events or cursor updates from its own tracking area or async page cursor changes, clears :hover when covered, and restores hover and its page cursor once uncovered. Run owns AppKit's process lifetime, so the test runs in a process of its own.
func TestWebViewUnderOverlayIgnoresHoverAndCursor(t *testing.T) {
	if os.Getenv("NSGO_TEST_WEB_PROCESS") != t.Name() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_WEB_PROCESS="+t.Name(), "HOME="+t.TempDir())
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		if strings.Contains(string(out), "--- SKIP:") {
			t.Skipf("live WebKit WindowServer assertions skipped in subprocess:\n%s", out)
		}
		return
	}

	page := `<!doctype html><html><body style="margin:0;width:100vw;height:100vh;display:flex">
<a id="left" href="#x" style="display:block;width:150px;height:200px;cursor:pointer">left</a>
<div id="right" style="width:150px;height:200px;cursor:default">right</div>
<script>
['left','right'].forEach(function(id) {
  var el = document.getElementById(id);
  ['mouseenter','mousemove','mouseleave'].forEach(function(ev) {
    el.addEventListener(ev, function(e) {
      window.nsgo.postMessage(id + ':' + ev + ':' + Math.round(e.clientX) + ',' + Math.round(e.clientY));
    });
  });
});
window.checkHover = function(tag) {
  var l = document.getElementById('left').matches(':hover');
  var r = document.getElementById('right').matches(':hover');
  window.nsgo.postMessage('hover:' + tag + ':L=' + l + ',R=' + r);
};
window.setLeftCursor = function(c) {
  document.getElementById('left').style.cursor = c;
  window.nsgo.postMessage('setCursor:' + c);
};
window.nsgo.postMessage('ready');
</script></body></html>`

	var mu sync.Mutex
	var msgs []string
	popMsgs := func() []string {
		mu.Lock()
		out := append([]string(nil), msgs...)
		msgs = nil
		mu.Unlock()
		return out
	}
	hasPrefix := func(prefix string) (string, bool) {
		mu.Lock()
		defer mu.Unlock()
		for _, m := range msgs {
			if strings.HasPrefix(m, prefix) {
				return m, true
			}
		}
		return "", false
	}
	ready := make(chan struct{}, 1)
	var showOverlay *core.Signal[bool]
	var appScope *core.Scope

	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetMainView(func(cx *core.Scope) core.Node {
			appScope = cx
			showOverlay = core.NewSignal(cx, true)
			return core.View{
				Style: core.Style{Width: core.Pt(300), Height: core.Pt(200)},
				Children: []core.Node{
					core.WebView{
						Source: core.WebHTML(page, "https://localhost/"),
						Style:  core.Style{Width: core.Pt(300), Height: core.Pt(200)},
						OnMessage: func(m string) {
							mu.Lock()
							msgs = append(msgs, m)
							mu.Unlock()
							if m == "ready" {
								select {
								case ready <- struct{}{}:
								default:
								}
							}
						},
					},
					core.Show{When: showOverlay.Get, Then: func(*core.Scope) core.Node {
						return core.Overlay{
							Style: core.Style{
								Top: core.Pt(0), Left: core.Pt(0),
								Width: core.Pt(150), Height: core.Pt(200),
								Background: core.RGB(0x224466),
								Cursor:     core.CursorCrosshair,
							},
						}
					}},
				},
			}
		})
	})

	type result struct {
		skip string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		defer apple.OnMain(finish)
		select {
		case <-ready:
		case <-time.After(15 * time.Second):
			done <- result{err: fmt.Errorf("web view did not load")}
			return
		}

		onMainSync := func(fn func()) {
			ch := make(chan struct{})
			apple.OnMain(func() {
				defer close(ch)
				fn()
			})
			<-ch
		}

		var webView objc.ID
		var surf *surface
		var webViewOwner *view
		onMainSync(func() {
			for id, v := range views {
				if v.web != nil && v.web.View() == id {
					webView = id
					surf = v.surface
					webViewOwner = v
					break
				}
			}
			appObj := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
			appObj.Send(sel("setActivationPolicy:"), int(0))
			appObj.Send(sel("activateIgnoringOtherApps:"), true)
			surf.window.Send(sel("setLevel:"), int(levelStatus))
			surf.window.Send(sel("makeKeyAndOrderFront:"), objc.ID(0))
			surf.window.Send(sel("orderFrontRegardless"))
		})
		if webView == 0 || surf == nil {
			done <- result{err: fmt.Errorf("WKWebView or surface not found")}
			return
		}
		for range 12 {
			var active bool
			onMainSync(func() {
				appObj := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
				active = objc.Send[bool](appObj, sel("isActive")) && objc.Send[bool](surf.window, sel("isKeyWindow"))
			})
			if active {
				break
			}
			time.Sleep(25 * time.Millisecond)
		}

		cursorName := func(cur objc.ID) string {
			switch cur {
			case nsCursor(driver.CursorAuto):
				return "arrow"
			case nsCursor(driver.CursorPointer):
				return "pointingHand"
			case nsCursor(driver.CursorCrosshair):
				return "crosshair"
			case nsCursor(driver.CursorGrab):
				return "openHand"
			default:
				return fmt.Sprintf("other(%#x)", uintptr(cur))
			}
		}

		toWindow := func(x, y float64) apple.CGPoint {
			return objc.Send[apple.CGPoint](surf.native, sel("convertPoint:toView:"), apple.CGPoint{X: x, Y: y}, objc.ID(0))
		}

		placePointer := func(x, y float64) (pt apple.CGPoint, topmost, liveCursor bool) {
			mouse := objc.Send[apple.CGPoint](objc.ID(apple.Class("NSEvent")), sel("mouseLocation"))
			contentRect := apple.Rect(mouse.X-x, mouse.Y-(200-y), 300, 200)
			frameRect := objc.Send[apple.CGRect](surf.window, sel("frameRectForContentRect:"), contentRect)
			surf.window.Send(sel("setFrame:display:"), frameRect, true)
			pt = toWindow(x, y)
			mLoc := objc.Send[apple.CGPoint](surf.window, sel("mouseLocationOutsideOfEventStream"))
			aligned := math.Abs(mLoc.X-pt.X) <= 2 && math.Abs(mLoc.Y-pt.Y) <= 2
			myWin := objc.Send[int](surf.window, sel("windowNumber"))
			topWin := objc.Send[int](objc.ID(apple.Class("NSWindow")), sel("windowNumberAtPoint:belowWindowWithWindowNumber:"), mouse, 0)
			topmost = aligned && myWin == topWin
			appObj := objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
			liveCursor = topmost && objc.Send[bool](appObj, sel("isActive")) && objc.Send[bool](surf.window, sel("isKeyWindow"))
			if !aligned {
				p := pt
				surf.testPointer = &p
			} else {
				surf.testPointer = nil
			}
			return pt, topmost, liveCursor
		}

		deliverTracking := func(pt apple.CGPoint, enter, move, curUpd bool) {
			winNum := objc.Send[int](surf.window, sel("windowNumber"))
			nsEvent := objc.ID(apple.Class("NSEvent"))
			if enter {
				ev := nsEvent.Send(
					sel("enterExitEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:trackingNumber:userData:"),
					uint(8), pt, uint(0), uptime(), winNum, objc.ID(0), 0, 0, uintptr(0))
				sendWebTracking(webView, 0x01, sel("mouseEntered:"), ev)
			}
			if move {
				ev := nsEvent.Send(
					sel("mouseEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:clickCount:pressure:"),
					uint(evMoved), pt, uint(0), uptime(), winNum, objc.ID(0), 0, 0, float32(0))
				sendWebTracking(webView, 0x02, sel("mouseMoved:"), ev)
			}
			if curUpd {
				ev := nsEvent.Send(
					sel("enterExitEventWithType:location:modifierFlags:timestamp:windowNumber:context:eventNumber:trackingNumber:userData:"),
					uint(17), pt, uint(0), uptime(), winNum, objc.ID(0), 0, 0, uintptr(0))
				webView.Send(sel("cursorUpdate:"), ev)
			}
		}

		waitMsg := func(prefix string) (string, error) {
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if m, ok := hasPrefix(prefix); ok {
					return m, nil
				}
				time.Sleep(15 * time.Millisecond)
			}
			return "", fmt.Errorf("timed out waiting for %q; msgs=%q", prefix, popMsgs())
		}

		checkHover := func(tag string) (string, error) {
			onMainSync(func() {
				webView.Send(sel("evaluateJavaScript:completionHandler:"), apple.NSString("window.checkHover('"+tag+"')"), objc.ID(0))
			})
			return waitMsg("hover:" + tag + ":")
		}

		waitCursor := func(want driver.Cursor) error {
			wantCur := nsCursor(want)
			deadline := time.Now().Add(2 * time.Second)
			var last objc.ID
			for time.Now().Before(deadline) {
				onMainSync(func() {
					last = objc.ID(apple.Class("NSCursor")).Send(sel("currentCursor"))
				})
				if last == wantCur {
					return nil
				}
				time.Sleep(20 * time.Millisecond)
			}
			return fmt.Errorf("cursor = %s, want %s", cursorName(last), cursorName(wantCur))
		}

		waitCovered := func(x, y float64, wantCovered bool) error {
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				var covered bool
				onMainSync(func() {
					hit := surf.hitAt(toWindow(x, y))
					covered = hit != 0 && hit != webView && !objc.Send[bool](hit, sel("isDescendantOf:"), webView)
				})
				if covered == wantCovered {
					return nil
				}
				time.Sleep(15 * time.Millisecond)
			}
			return fmt.Errorf("point (%.0f,%.0f) did not reach covered=%v", x, y, wantCovered)
		}

		if err := waitCovered(75, 100, true); err != nil {
			done <- result{err: err}
			return
		}
		popMsgs()

		// 1. Covered by overlay from the start: WKWebView's own tracking area must take no hover events and must not override the overlay's cursor.
		var topmost, liveCursor bool
		var stepErr error
		onMainSync(func() {
			var pt apple.CGPoint
			pt, topmost, liveCursor = placePointer(75, 100)
			surf.pick(pt, true, true)
			deliverTracking(pt, true, true, true)
			if surf.webOver != nil {
				stepErr = fmt.Errorf("step 1: webOver = %v, want nil under overlay", surf.webOver)
			}
			cur, _, own := cursorAt(surf.hitAt(pt), surf.native.Send(selSuperview), pt)
			if !own || cur != driver.CursorCrosshair {
				stepErr = fmt.Errorf("step 1: cursorAt = (%v, own=%v), want (Crosshair, true)", cur, own)
			}
		})
		if stepErr != nil {
			done <- result{err: stepErr}
			return
		}
		time.Sleep(150 * time.Millisecond)
		if got, err := checkHover("step1"); err != nil {
			done <- result{err: err}
			return
		} else if got != "hover:step1:L=false,R=false" {
			done <- result{err: fmt.Errorf("step 1 (covered): %s, want L=false,R=false", got)}
			return
		}
		for _, m := range popMsgs() {
			if strings.HasPrefix(m, "left:") || strings.HasPrefix(m, "right:") {
				done <- result{err: fmt.Errorf("step 1 (covered): unexpected page pointer event %q", m)}
				return
			}
		}
		if liveCursor {
			if err := waitCursor(driver.CursorCrosshair); err != nil {
				done <- result{err: fmt.Errorf("step 1 (covered): %w", err)}
				return
			}
		}
		if !topmost {
			done <- result{skip: "test window is occluded by another window or locked screen; WebKit skips live tracking-area hovers while occluded"}
			return
		}

		// 2. Move onto uncovered right half (default cursor = arrow) without a new AppKit mouseEntered: (the pointer stayed inside WKWebView's frame): pickWeb primes mouseEntered:, page hovers, and cursor resets from crosshair to arrow.
		onMainSync(func() {
			var pt apple.CGPoint
			pt, topmost, liveCursor = placePointer(225, 100)
			surf.pick(pt, true, true)
			deliverTracking(pt, false, true, false)
			if surf.webOver != webViewOwner {
				stepErr = fmt.Errorf("step 2: webOver = %v, want web view", surf.webOver)
			}
		})
		if stepErr != nil {
			done <- result{err: stepErr}
			return
		}
		if _, err := waitMsg("right:mouseenter:"); err != nil {
			done <- result{err: fmt.Errorf("step 2 (uncovered): %w", err)}
			return
		}
		if got, err := checkHover("step2"); err != nil {
			done <- result{err: err}
			return
		} else if got != "hover:step2:L=false,R=true" {
			done <- result{err: fmt.Errorf("step 2 (uncovered): %s, want L=false,R=true", got)}
			return
		}
		if liveCursor {
			if err := waitCursor(driver.CursorAuto); err != nil {
				done <- result{err: fmt.Errorf("step 2 (uncovered default cursor): %w", err)}
				return
			}
		}
		popMsgs()

		// 3. Move from uncovered WKWebView back onto the covering overlay: WKWebView must leave :hover on right, take no hover on left, and keep the overlay's crosshair cursor.
		onMainSync(func() {
			var pt apple.CGPoint
			pt, topmost, liveCursor = placePointer(75, 100)
			surf.pick(pt, true, true)
			deliverTracking(pt, false, true, true)
			if surf.webOver != nil {
				stepErr = fmt.Errorf("step 3: webOver = %v, want nil", surf.webOver)
			}
		})
		if stepErr != nil {
			done <- result{err: stepErr}
			return
		}
		if _, err := waitMsg("right:mouseleave:"); err != nil {
			done <- result{err: fmt.Errorf("step 3 (moved onto overlay): %w", err)}
			return
		}
		if got, err := checkHover("step3"); err != nil {
			done <- result{err: err}
			return
		} else if got != "hover:step3:L=false,R=false" {
			done <- result{err: fmt.Errorf("step 3 (moved onto overlay): %s, want L=false,R=false", got)}
			return
		}
		for _, m := range popMsgs() {
			if strings.HasPrefix(m, "left:") {
				done <- result{err: fmt.Errorf("step 3 (moved onto overlay): covered left got %q", m)}
				return
			}
		}
		if liveCursor {
			if err := waitCursor(driver.CursorCrosshair); err != nil {
				done <- result{err: fmt.Errorf("step 3 (moved onto overlay): %w", err)}
				return
			}
		}

		// 4. Close overlay while the pointer rests at (75, 100) with no mouse move: left (<a style="cursor:pointer">) must automatically hover and take the pointing hand cursor.
		appScope.Post(func() { showOverlay.Set(false) })
		if err := waitCovered(75, 100, false); err != nil {
			done <- result{err: err}
			return
		}
		if _, err := waitMsg("left:mouseenter:"); err != nil {
			done <- result{err: fmt.Errorf("step 4 (overlay closed under resting pointer): %w", err)}
			return
		}
		if got, err := checkHover("step4"); err != nil {
			done <- result{err: err}
			return
		} else if got != "hover:step4:L=true,R=false" {
			done <- result{err: fmt.Errorf("step 4 (overlay closed): %s, want L=true,R=false", got)}
			return
		}
		if liveCursor {
			if err := waitCursor(driver.CursorPointer); err != nil {
				done <- result{err: fmt.Errorf("step 4 (overlay closed): %w", err)}
				return
			}
		}
		popMsgs()

		// 5. Re-open overlay over the resting hovered link with no mouse move: WKWebView must clear :hover, switch back to the overlay's crosshair, and ignore both JS cursor mutations and tracking-area moves while covered.
		appScope.Post(func() { showOverlay.Set(true) })
		if err := waitCovered(75, 100, true); err != nil {
			done <- result{err: err}
			return
		}
		if _, err := waitMsg("left:mouseleave:"); err != nil {
			done <- result{err: fmt.Errorf("step 5 (overlay opened over hovered web view): %w", err)}
			return
		}
		if liveCursor {
			if err := waitCursor(driver.CursorCrosshair); err != nil {
				done <- result{err: fmt.Errorf("step 5 (overlay opened): %w", err)}
				return
			}
		}
		onMainSync(func() {
			webView.Send(sel("evaluateJavaScript:completionHandler:"), apple.NSString("window.setLeftCursor('grab')"), objc.ID(0))
		})
		if _, err := waitMsg("setCursor:grab"); err != nil {
			done <- result{err: err}
			return
		}
		onMainSync(func() {
			pt := toWindow(75, 100)
			deliverTracking(pt, true, true, true)
		})
		time.Sleep(150 * time.Millisecond)
		if got, err := checkHover("step5"); err != nil {
			done <- result{err: err}
			return
		} else if got != "hover:step5:L=false,R=false" {
			done <- result{err: fmt.Errorf("step 5 (covered after JS cursor + move): %s, want L=false,R=false", got)}
			return
		}
		for _, m := range popMsgs() {
			if strings.HasPrefix(m, "left:mouseenter:") || strings.HasPrefix(m, "left:mousemove:") {
				done <- result{err: fmt.Errorf("step 5 (covered): unexpected page event %q", m)}
				return
			}
		}
		if liveCursor {
			if err := waitCursor(driver.CursorCrosshair); err != nil {
				done <- result{err: fmt.Errorf("step 5 (covered after JS cursor + move): %w", err)}
				return
			}
		}

		// 6. Close overlay again under the resting pointer: WKWebView resumes hover and shows the updated 'grab' (openHand) cursor.
		appScope.Post(func() { showOverlay.Set(false) })
		if err := waitCovered(75, 100, false); err != nil {
			done <- result{err: err}
			return
		}
		if _, err := waitMsg("left:mouseenter:"); err != nil {
			done <- result{err: fmt.Errorf("step 6 (overlay closed again): %w", err)}
			return
		}
		if got, err := checkHover("step6"); err != nil {
			done <- result{err: err}
			return
		} else if got != "hover:step6:L=true,R=false" {
			done <- result{err: fmt.Errorf("step 6 (overlay closed again): %s, want L=true,R=false", got)}
			return
		}
		if liveCursor {
			if err := waitCursor(driver.CursorGrab); err != nil {
				done <- result{err: fmt.Errorf("step 6 (overlay closed again): %w", err)}
				return
			}
		}

		done <- result{}
	}()

	onMain(func() { Run(app, Options{Title: "Web overlay hover", Width: 300, Height: 200}) })
	res := <-done
	if res.err != nil {
		t.Fatal(res.err)
	}
	if res.skip != "" {
		t.Skip(res.skip)
	}
}
