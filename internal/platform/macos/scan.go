//go:build darwin && !ios

package macos

import (
	"log"
	"sync"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/desktop/locales"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
	"github.com/nsgo-dev/nsgo/internal/platform/privacy"
)

var (
	scannerOnce     sync.Once
	scannerDelegate objc.Class
	scanners        = map[objc.ID]*scanner{}
)

// AppKit and metadata callbacks run on main. AVFoundation starts and
// stops the capture session on a worker.
type scanner struct {
	s       *surface
	id      uint32
	request driver.ScanRequest

	parent, window, delegate objc.ID
	session, output          objc.ID
	stop                     func()
}

func (s *surface) scanCode(r driver.ScanRequest, id uint32) {
	if s.scanner != nil {
		s.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: id, Text: "a scanner is already open for this surface"})
		return
	}
	scan := &scanner{s: s, id: id, request: r}
	s.scanner = scan
	if apple.ScanCamera() == 0 {
		scan.finish(nil, driver.NoScanCamera)
		return
	}
	if !privacy.DeclaresUsage("NSCameraUsageDescription") {
		log.Print("nsgo: scanning a code needs NSCameraUsageDescription in the app's Info.plist (nsgo.yaml's apple.info)")
		scan.finish(nil, "the app declares no NSCameraUsageDescription")
		return
	}
	privacy.Request(driver.PermissionCamera, func(status int) {
		apple.OnMain(func() {
			if s.scanner != scan {
				return
			}
			if status == driver.PermissionGranted {
				scan.present()
			} else {
				scan.finish(nil, "camera access denied")
			}
		})
	})
}

func (sc *scanner) present() {
	device := apple.ScanCamera()
	if device == 0 {
		sc.finish(nil, driver.NoScanCamera)
		return
	}
	session, output, why := apple.NewScanCapture(device, sc.request)
	if why != "" {
		sc.finish(nil, why)
		return
	}
	sc.session, sc.output = session, output
	scannerOnce.Do(registerScanner)
	sc.delegate = objc.ID(scannerDelegate).Send(selNew)
	scanners[sc.delegate] = sc
	output.Send(sel("setMetadataObjectsDelegate:queue:"), sc.delegate, apple.MainQueue())
	sc.showWindow()
}

func (sc *scanner) showWindow() {
	sc.window = sc.makeWindow()

	parent := sc.s.window
	if parent != 0 && sc.s.kind != driver.SurfaceTrayIcon && objc.Send[bool](parent, sel("isVisible")) &&
		!objc.Send[bool](parent, sel("isMiniaturized")) {
		sc.parent = parent.Send(sel("retain"))
		done := objcblock.New(func(_ objc.Block, _ int) {
			if sc.s.scanner == sc {
				sc.parent.Send(selRelease)
				sc.parent = 0
				sc.finish(nil, "")
			}
		})
		parent.Send(sel("beginSheet:completionHandler:"), sc.window, done)
		done.Release()
		// A sheet queued behind another one starts when it becomes key.
		if parent.Send(sel("attachedSheet")) == sc.window {
			sc.start()
		}
		return
	}

	activateApp()
	sc.window.Send(sel("center"))
	sc.window.Send(sel("makeKeyAndOrderFront:"), objc.ID(0))
	sc.start()
}

func (sc *scanner) makeWindow() objc.ID {
	const (
		width       = 480
		height      = 320
		footer      = 48
		titled      = 1 << 0
		closable    = 1 << 1
		buttonInset = 16
	)
	msgs := locales.In(sc.s.language)
	w := objc.ID(apple.Class("NSWindow")).Send(selAlloc).Send(sel("initWithContentRect:styleMask:backing:defer:"),
		apple.Rect(0, 0, width, height+footer), uint(titled|closable), uint(nsBackingStoreBuffered), false)
	w.Send(sel("setReleasedWhenClosed:"), false)
	w.Send(sel("setTitle:"), apple.NSString(msgs.ScanCode()))
	w.Send(selSetDelegate, sc.delegate)
	content := w.Send(sel("contentView"))

	view := objc.ID(apple.Class("NSView")).Send(selAlloc).Send(sel("initWithFrame:"), apple.Rect(0, footer, width, height))
	view.Send(sel("setWantsLayer:"), true)
	preview := objc.ID(apple.Class("AVCaptureVideoPreviewLayer")).Send(sel("layerWithSession:"), sc.session)
	preview.Send(sel("setVideoGravity:"), apple.NSString("AVLayerVideoGravityResizeAspect"))
	preview.Send(sel("setBackgroundColor:"), systemColor("blackColor").Send(sel("CGColor")))
	preview.Send(selSetFrame, apple.Rect(0, 0, width, height))
	view.Send(sel("layer")).Send(sel("addSublayer:"), preview)
	content.Send(sel("addSubview:"), view)
	view.Send(selRelease)

	cancel := objc.ID(apple.Class("NSButton")).Send(sel("buttonWithTitle:target:action:"), apple.NSString(msgs.Cancel()),
		sc.delegate, sel("cancel:"))
	cancel.Send(sel("setKeyEquivalent:"), apple.NSString("\x1b"))
	cancel.Send(sel("sizeToFit"))
	size := objc.Send[apple.CGRect](cancel, sel("frame")).Size
	cancel.Send(selSetFrame, apple.Rect(width-buttonInset-size.Width, (footer-size.Height)/2, size.Width, size.Height))
	content.Send(sel("addSubview:"), cancel)
	w.Send(sel("setInitialFirstResponder:"), cancel)
	return w
}

func (sc *scanner) start() {
	if sc.s.scanner != sc || sc.stop != nil {
		return
	}
	sc.stop = apple.StartScanCapture(sc.session, func() { sc.finish(nil, "the camera cannot be used") })
}

func (sc *scanner) finish(code *driver.ScannedCode, why string) {
	if sc.s.scanner != sc {
		return
	}
	sc.s.scanner = nil
	if sc.delegate != 0 {
		delete(scanners, sc.delegate)
	}
	if sc.output != 0 {
		sc.output.Send(sel("setMetadataObjectsDelegate:queue:"), objc.ID(0), objc.ID(0))
		sc.output.Send(selRelease)
	}
	if sc.window != 0 {
		sc.window.Send(selSetDelegate, objc.ID(0))
		if sc.parent != 0 {
			sc.parent.Send(sel("endSheet:"), sc.window)
			sc.parent.Send(selRelease)
		}
		sc.window.Send(sel("close"))
		sc.window.Send(selRelease)
	}
	if sc.delegate != 0 {
		sc.delegate.Send(selRelease)
	}
	if sc.stop != nil {
		sc.stop()
	} else if sc.session != 0 {
		sc.session.Send(selRelease)
	}
	if !sc.s.closed {
		ev := driver.Event{Kind: driver.EventPick, Count: sc.id, Text: why}
		if code != nil {
			ev.Data = code
		}
		sc.s.engine.Dispatch(ev)
	}
}

func registerScanner() {
	var protocols []*objc.Protocol
	for _, name := range []string{"AVCaptureMetadataOutputObjectsDelegate", "NSWindowDelegate"} {
		if p := objc.GetProtocol(name); p != nil {
			protocols = append(protocols, p)
		}
	}
	scannerDelegate = register("NsgoScannerDelegate", apple.Class("NSObject"), protocols, []objc.MethodDef{{
		Cmd: sel("captureOutput:didOutputMetadataObjects:fromConnection:"),
		Fn: func(self objc.ID, _ objc.SEL, _, objects, _ objc.ID) {
			if sc := scanners[self]; sc != nil {
				if code := apple.CodeFromMetadata(objects, sc.request); code != nil {
					sc.finish(code, "")
				}
			}
		},
	}, {
		Cmd: sel("windowDidBecomeKey:"),
		Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			if sc := scanners[self]; sc != nil {
				sc.start()
			}
		},
	}, {
		Cmd: sel("windowShouldClose:"),
		Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) bool {
			if sc := scanners[self]; sc != nil {
				sc.finish(nil, "")
			}
			return false
		},
	}, {
		Cmd: sel("cancel:"),
		Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			if sc := scanners[self]; sc != nil {
				sc.finish(nil, "")
			}
		},
	}})
}
