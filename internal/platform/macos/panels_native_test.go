//go:build darwin && !ios

package macos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestPanelsRemainVisibleWithoutAnOnscreenOwnerNative(t *testing.T) {
	if os.Getenv("NSGO_TEST_PANELS_PROCESS") != t.Name() {
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_PANELS_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		nsApp().Send(sel("setActivationPolicy:"), policyRegular)
		nsApp().Send(sel("finishLaunching"))
		states := []string{"visible", "hidden", "minimized", "offscreen", "windowless"}
		// A raw test binary launched over SSH cannot necessarily activate or
		// hide itself. Exercise application hiding in the registered .app
		// fixture; every window visibility state runs in ordinary tests too.
		if os.Getenv("NSGO_TEST_PANELS_APP_HIDDEN") == "1" {
			states = append(states, "app-hidden")
		} else {
			t.Log("application-hide cases require the registered Launch Services fixture")
		}
		for _, state := range states {
			for _, kind := range []string{"open", "save", "alert"} {
				t.Log(state + "/" + kind)
				testNativePanel(t, state, kind, false)
			}
		}
		for _, kind := range []string{"images", "directories", "open", "alert"} {
			t.Log("close/" + kind)
			testNativePanel(t, "hidden", kind, true)
		}
		testQueuedNativeAlerts(t)
	})
}

func panelTestPump(until func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for !until() && time.Now().Before(deadline) {
		date := objc.ID(apple.Class("NSDate")).Send(sel("dateWithTimeIntervalSinceNow:"), 0.01)
		objc.ID(apple.Class("NSRunLoop")).Send(sel("mainRunLoop")).Send(sel("runMode:beforeDate:"), apple.NSString("kCFRunLoopDefaultMode"), date)
	}
	return until()
}

func panelTestOwner(t *testing.T, state string) (*surface, <-chan driver.Event, func()) {
	t.Helper()
	frame := screenFrames("visibleFrame")[0]
	w := objc.ID(unconstrainedTestWindow()).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
		apple.Rect(frame.Origin.X+80, frame.Origin.Y+80, 320, 180), uint(styleTitled|styleMiniaturizable|styleClosable), uint(2), false)
	w.Send(sel("setReleasedWhenClosed:"), false)
	w.Send(sel("setTitle:"), apple.NSString("NSGo 1395 owned fixture"))
	s := &surface{window: w}
	events := make(chan driver.Event, 16)
	s.engine = engine.Start(actionEngine{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{
		Event: func(_ uint32, e driver.Event) { events <- e },
	})
	cleanup := func() {
		s.close()
		nsApp().Send(sel("unhideWithoutActivation"))
		w.Send(sel("close"))
		w.Send(selRelease)
	}
	if state != "hidden" && state != "windowless" {
		w.Send(sel("orderFrontRegardless"))
	}
	switch state {
	case "minimized":
		w.Send(sel("miniaturize:"), objc.ID(0))
		if !panelTestPump(func() bool { return objc.Send[bool](w, sel("isMiniaturized")) }, 3*time.Second) {
			cleanup()
			t.Fatal("fixture did not minimize its own window")
		}
	case "offscreen":
		w.Send(sel("setFrame:display:"), apple.Rect(100000, 100000, 320, 180), true)
	case "windowless":
		s.window = 0
	case "app-hidden":
		nsApp().Send(sel("hide:"), objc.ID(0))
		if !panelTestPump(func() bool { return objc.Send[bool](nsApp(), sel("isHidden")) }, time.Second) {
			cleanup()
			t.Fatal("fixture could not hide its own app")
		}
	}
	return s, events, cleanup
}

func testNativePanel(t *testing.T, state, kind string, closeOwner bool) {
	t.Helper()
	s, events, cleanup := panelTestOwner(t, state)
	defer cleanup()
	wantSheet := state == "visible"
	if got := s.panelWindow() != 0; got != wantSheet {
		t.Fatalf("%s: sheet eligibility %v, want %v", state, got, wantSheet)
	}
	var observed, finished bool
	var failure error
	var inspect func()
	deadline := time.Now().Add(8 * time.Second)
	inspect = func() {
		if finished {
			return
		}
		var window objc.ID
		for panel := range s.filePanels {
			window = panel
		}
		if shown, ok := s.alerts[1]; ok {
			window = shown.alert.Send(selWindow)
			if nsApp().Send(sel("modalWindow")) != window && window.Send(sel("sheetParent")) == 0 {
				window = 0
			}
		}
		if window != 0 && objc.Send[bool](window, sel("isVisible")) {
			observed = true
			if got := window.Send(sel("sheetParent")); (got != 0) != wantSheet || wantSheet && got != s.window {
				failure = fmt.Errorf("%s/%s: sheet parent %x, owner %x, want sheet %v", state, kind, got, s.window, wantSheet)
			}
			if closeOwner {
				s.close()
			} else if kind == "alert" {
				s.alerts[1].alert.Send(sel("buttons")).Send(sel("objectAtIndex:"), uint(0)).Send(sel("performClick:"), objc.ID(0))
			} else {
				window.Send(sel("cancel:"), objc.ID(0))
			}
			return
		}
		if time.Now().After(deadline) {
			failure = fmt.Errorf("%s/%s never became visible", state, kind)
			s.closeDialogs()
			finished = true
			return
		}
		time.AfterFunc(10*time.Millisecond, func() { apple.OnMain(inspect) })
	}
	switch kind {
	case "open":
		s.pickFiles(driver.FilesRequest{Title: "NSGo 1395 open"}, 1)
	case "directories":
		s.pickDirectories(driver.DirectoriesRequest{Title: "NSGo 1395 directories"}, 1)
	case "images":
		s.pickImages(driver.PickRequest{Limit: 1}, 1)
	case "save":
		s.saveFile(driver.SaveRequest{Name: "nsgo1395.txt"}, 1)
	case "alert":
		s.alert(&driver.AlertConfig{ID: 1, Title: "NSGo 1395 alert", Actions: []driver.AlertAction{{Label: "Close"}}})
	}
	apple.OnMain(inspect)
	var response driver.Event
	if !panelTestPump(func() bool {
		if closeOwner && observed && len(s.filePanels) == 0 && len(s.alerts) == 0 {
			finished = true
		}
		select {
		case response = <-events:
			finished = true
		default:
		}
		return finished
	}, 10*time.Second) {
		t.Fatal("dialog did not complete; a modal loop may have blocked the main dispatch queue")
	}
	finished = true
	if failure != nil || !observed {
		t.Fatalf("visible=%v: %v", observed, failure)
	}
	if !closeOwner && (response.Count != 1 || kind == "alert" && response.Index != 0) {
		t.Fatalf("unexpected dialog response %+v", response)
	}
	if len(s.filePanels) != 0 || len(s.alerts) != 0 {
		t.Fatal("completed dialog remains registered")
	}
}

func testQueuedNativeAlerts(t *testing.T) {
	s, events, cleanup := panelTestOwner(t, "hidden")
	defer cleanup()
	for _, id := range []uint32{1, 2} {
		s.alert(&driver.AlertConfig{ID: id, Title: fmt.Sprintf("NSGo 1395 queued %d", id), Actions: []driver.AlertAction{{Label: "Close"}}})
	}
	var finished bool
	var inspect func()
	var order []uint32
	inspect = func() {
		if finished {
			return
		}
		modal := nsApp().Send(sel("modalWindow"))
		for id, shown := range s.alerts {
			if modal != 0 && shown.alert.Send(selWindow) == modal {
				order = append(order, id)
				shown.alert.Send(sel("buttons")).Send(sel("objectAtIndex:"), uint(0)).Send(sel("performClick:"), objc.ID(0))
			}
		}
		time.AfterFunc(10*time.Millisecond, func() { apple.OnMain(inspect) })
	}
	apple.OnMain(inspect)
	if !panelTestPump(func() bool { return len(s.alerts) == 0 }, 10*time.Second) {
		t.Fatal("standalone alerts did not drain in order")
	}
	finished = true
	if len(order) != 2 || order[0] != 1 || order[1] != 2 || len(events) != 2 {
		t.Fatalf("modal order %v, responses %d", order, len(events))
	}
}
