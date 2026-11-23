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
// stops the capture session on a worker. With photo the scanner is TakePhoto's camera: its output is an AVCapturePhotoOutput and a shutter takes the photo.
type scanner struct {
	s       *surface
	id      uint32
	request driver.ScanRequest
	photo   *driver.PickRequest

	parent, window, delegate objc.ID
	session, output          objc.ID
	shutter                  objc.ID // TakePhoto's button, enabled once the session runs
	shooting                 bool
	stop                     func()
}

func (s *surface) scanCode(r driver.ScanRequest, id uint32) {
	s.openCamera(&scanner{s: s, id: id, request: r}, "scanning a code")
}

// takePhoto shows the scanner's camera with a shutter, which answers id with the photo it takes.
func (s *surface) takePhoto(r driver.PickRequest, id uint32) {
	s.openCamera(&scanner{s: s, id: id, photo: &r}, "taking a photo")
}

// openCamera has the user allow the camera and shows scan's, or answers with why it cannot; doing names what it is for in the log.
func (s *surface) openCamera(scan *scanner, doing string) {
	if s.scanner != nil {
		s.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: scan.id, Text: "a camera is already open for this surface"})
		return
	}
	s.scanner = scan
	if apple.ScanCamera() == 0 {
		why := driver.NoScanCamera
		if scan.photo != nil {
			why = driver.NoCamera
		}
		scan.finish(nil, why)
		return
	}
	if !privacy.DeclaresUsage("NSCameraUsageDescription") {
		log.Printf("nsgo: %s needs NSCameraUsageDescription in the app's Info.plist (nsgo.yaml's apple.info)", doing)
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
		why := driver.NoScanCamera
		if sc.photo != nil {
			why = driver.NoCamera
		}
		sc.finish(nil, why)
		return
	}
	var session, output objc.ID
	var why string
	if sc.photo != nil {
		session, output, why = apple.NewPhotoCapture(device)
	} else {
		session, output, why = apple.NewScanCapture(device, sc.request)
	}
	if why != "" {
		sc.finish(nil, why)
		return
	}
	sc.session, sc.output = session, output
	scannerOnce.Do(registerScanner)
	sc.delegate = objc.ID(scannerDelegate).Send(selNew)
	scanners[sc.delegate] = sc
	if sc.photo == nil {
		output.Send(sel("setMetadataObjectsDelegate:queue:"), sc.delegate, apple.MainQueue())
	}
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
	title := msgs.ScanCode()
	if sc.photo != nil {
		title = msgs.TakePhoto()
	}
	w.Send(sel("setTitle:"), apple.NSString(title))
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

	// Buttons line up from the trailing edge: TakePhoto's shutter, then Cancel.
	right := float64(width - buttonInset)
	button := func(title, action, key string) objc.ID {
		b := objc.ID(apple.Class("NSButton")).Send(sel("buttonWithTitle:target:action:"), apple.NSString(title), sc.delegate, sel(action))
		b.Send(sel("setKeyEquivalent:"), apple.NSString(key))
		b.Send(sel("sizeToFit"))
		size := objc.Send[apple.CGRect](b, sel("frame")).Size
		b.Send(selSetFrame, apple.Rect(right-size.Width, (footer-size.Height)/2, size.Width, size.Height))
		right -= size.Width + 8
		content.Send(sel("addSubview:"), b)
		return b
	}
	if sc.photo != nil {
		sc.shutter = button(msgs.TakePhoto(), "shutter:", "\r")
		sc.shutter.Send(sel("setEnabled:"), false)
		w.Send(sel("setDefaultButtonCell:"), sc.shutter.Send(sel("cell")))
	}
	cancel := button(msgs.Cancel(), "cancel:", "\x1b")
	w.Send(sel("setInitialFirstResponder:"), cancel)
	return w
}

func (sc *scanner) start() {
	if sc.s.scanner != sc || sc.stop != nil {
		return
	}
	running := func() {
		if sc.s.scanner == sc && sc.shutter != 0 && !sc.shooting {
			sc.shutter.Send(sel("setEnabled:"), true)
		}
	}
	sc.stop = apple.StartCapture(sc.session, running, func() { sc.finish(nil, "the camera cannot be used") })
}

// shoot asks the photo output for a photo, which photoTaken answers.
func (sc *scanner) shoot() {
	if sc.photo == nil || sc.shooting || sc.s.scanner != sc {
		return
	}
	sc.shooting = true
	sc.shutter.Send(sel("setEnabled:"), false)
	sc.output.Send(sel("capturePhotoWithSettings:delegate:"), objc.ID(apple.Class("AVCapturePhotoSettings")).Send(sel("photoSettings")), sc.delegate)
}

// photoTaken ends the camera and answers TakePhoto with data, the photo's file, upright and prepared as it asks, or with why it failed.
func (sc *scanner) photoTaken(data []byte, why string) {
	if sc.s.scanner != sc {
		return
	}
	if why == "" && len(data) == 0 {
		why = "the camera took no photo"
	}
	r, id, closed := *sc.photo, sc.id, sc.s.closed
	sc.end()
	if closed {
		return
	}
	if why != "" {
		sc.s.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: id, Text: why})
		return
	}
	// PreparePicked turns it upright as it encodes it again at the quality asked for.
	r.Quality = r.JPEGQuality()
	f := apple.PreparePicked(r, driver.PickedFile{Name: "photo.jpg", Data: data})
	sc.s.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: id, Data: []driver.PickedFile{f}})
}

func (sc *scanner) finish(code *driver.ScannedCode, why string) {
	if sc.s.scanner != sc {
		return
	}
	sc.end()
	if !sc.s.closed {
		ev := driver.Event{Kind: driver.EventPick, Count: sc.id, Text: why}
		if code != nil {
			ev.Data = code
		}
		sc.s.engine.Dispatch(ev)
	}
}

// end closes the camera's window and stops its session.
func (sc *scanner) end() {
	sc.s.scanner = nil
	if sc.delegate != 0 {
		delete(scanners, sc.delegate)
	}
	if sc.output != 0 {
		if sc.photo == nil {
			sc.output.Send(sel("setMetadataObjectsDelegate:queue:"), objc.ID(0), objc.ID(0))
		}
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
}

func registerScanner() {
	var protocols []*objc.Protocol
	for _, name := range []string{"AVCaptureMetadataOutputObjectsDelegate", "AVCapturePhotoCaptureDelegate", "NSWindowDelegate"} {
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
	}, {
		Cmd: sel("shutter:"),
		Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			if sc := scanners[self]; sc != nil {
				sc.shoot()
			}
		},
	}, {
		// AVFoundation answers on a queue of its own.
		Cmd: sel("captureOutput:didFinishProcessingPhoto:error:"),
		Fn: func(self objc.ID, _ objc.SEL, _, photo, err objc.ID) {
			var data []byte
			why := ""
			if err != 0 {
				if why = apple.GoString(err.Send(sel("localizedDescription"))); why == "" {
					why = "the camera could not take the photo"
				}
			} else if photo != 0 {
				data = apple.Bytes(photo.Send(sel("fileDataRepresentation")))
			}
			apple.OnMain(func() {
				if sc := scanners[self]; sc != nil {
					sc.photoTaken(data, why)
				}
			})
		},
	}})
}
