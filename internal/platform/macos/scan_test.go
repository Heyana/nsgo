//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestScannerCancelClosesTheAppKitSheet(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a native window")
	}
	onMain(func() {
		loadAppKit()
		pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(selNew)
		defer pool.Send(sel("drain"))
		apple.ScanCamera() // load AVFoundation without starting capture
		if apple.Class("AVCaptureSession") == 0 {
			t.Skip("AVFoundation is unavailable")
		}
		parent := objc.ID(classWindow).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
			apple.Rect(0, 0, 500, 400), uint(1), uint(nsBackingStoreBuffered), false)
		parent.Send(sel("setReleasedWhenClosed:"), false)
		defer parent.Send(selRelease)
		defer parent.Send(sel("close"))
		parent.Send(sel("orderFront:"), objc.ID(0))
		if !objc.Send[bool](parent, sel("isVisible")) {
			t.Skip("AppKit could not show the test window")
		}

		// Give the sheet an empty capture session. No permission prompt or
		// camera stream is needed to exercise AppKit's cancellation path.
		session := objc.ID(apple.Class("AVCaptureSession")).Send(selNew)
		s := &surface{window: parent, closed: true, language: "en"}
		stops := 0
		sc := &scanner{s: s, session: session, stop: func() {
			stops++
			session.Send(selRelease)
		}}
		s.scanner = sc
		defer sc.finish(nil, "")
		scannerOnce.Do(registerScanner)
		sc.delegate = objc.ID(scannerDelegate).Send(selNew)
		scanners[sc.delegate] = sc
		sc.showWindow()
		if parent.Send(sel("attachedSheet")) != sc.window {
			t.Fatal("the scanner did not attach to the requesting window")
		}
		cancel := sc.window.Send(sel("contentView")).Send(sel("subviews")).Send(sel("lastObject"))
		if apple.GoString(cancel.Send(sel("title"))) != "Cancel" || apple.GoString(cancel.Send(sel("keyEquivalent"))) != "\x1b" {
			t.Fatal("the sheet has no Escape cancel button")
		}
		cancel.Send(sel("performClick:"), objc.ID(0))
		if s.scanner != nil || scanners[sc.delegate] != nil || stops != 1 {
			t.Fatalf("cancel left scanner %v and stopped capture %d times", s.scanner, stops)
		}
	})
}
