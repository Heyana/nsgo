//go:build darwin && !ios

package macos

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
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

// TestTakePhotoSheetShootsAndAnswers shows TakePhoto's sheet over an empty capture session: its shutter, after Cancel, is the default button Return presses and is disabled until the session runs, and the photo the output answers with comes back upright as a JPEG scaled to MaxDimension.
func TestTakePhotoSheetShootsAndAnswers(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a native window")
	}
	var shot bytes.Buffer
	if err := jpeg.Encode(&shot, image.NewRGBA(image.Rect(0, 0, 8, 4)), nil); err != nil {
		t.Fatal(err)
	}
	events := make(chan driver.Event, 1)
	eng := engine.Start(actionEngine{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{Event: func(_ uint32, e driver.Event) {
		if e.Kind == driver.EventPick {
			events <- e
		}
	}})
	defer eng.Close()
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
		session := objc.ID(apple.Class("AVCaptureSession")).Send(selNew)
		s := &surface{window: parent, engine: eng, language: "en"}
		sc := &scanner{s: s, id: 7, photo: &driver.PickRequest{Limit: 1, MaxDimension: 4}, session: session, stop: func() { session.Send(selRelease) }}
		s.scanner = sc
		scannerOnce.Do(registerScanner)
		sc.delegate = objc.ID(scannerDelegate).Send(selNew)
		scanners[sc.delegate] = sc
		sc.showWindow()
		buttons := sc.window.Send(sel("contentView")).Send(sel("subviews"))
		var titles []string
		apple.ForEach(buttons, func(b objc.ID) {
			if objc.Send[bool](b, sel("isKindOfClass:"), objc.ID(apple.Class("NSButton"))) {
				titles = append(titles, apple.GoString(b.Send(sel("title"))))
			}
		})
		if len(titles) != 2 || titles[0] != "Take Photo" || titles[1] != "Cancel" {
			t.Errorf("the sheet's buttons are %q", titles)
		}
		if sc.window.Send(sel("defaultButtonCell")) != sc.shutter.Send(sel("cell")) {
			t.Error("the shutter is not the sheet's default button, which Return presses")
		}
		if objc.Send[bool](sc.shutter, sel("isEnabled")) {
			t.Error("the shutter is enabled before the session runs")
		}
		sc.photoTaken(shot.Bytes(), "")
		if s.scanner != nil || scanners[sc.delegate] != nil {
			t.Error("the photo left the sheet open")
		}
	})
	select {
	case ev := <-events:
		files, _ := ev.Data.([]driver.PickedFile)
		if ev.Count != 7 || ev.Text != "" || len(files) != 1 || files[0].MIME != "image/jpeg" || files[0].Size != (driver.Size{Width: 4, Height: 2}) {
			t.Fatalf("the photo answers %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the photo was not answered")
	}
}
