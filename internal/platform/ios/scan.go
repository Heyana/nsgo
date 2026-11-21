//go:build ios

package ios

import (
	"sync"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// Scanning a code: an AVCaptureSession of the back camera whose AVCaptureMetadataOutput reads the kinds of code asked for, shown full screen in its AVCaptureVideoPreviewLayer under a navigation bar with the system's Cancel button, which UIKit words in the user's language.

const (
	// UIBarButtonSystemItemCancel, UIModalPresentationFullScreen and UIUserInterfaceStyleDark.
	barButtonCancel        = 1
	presentationFullScreen = 0
	interfaceStyleDark     = 2
)

var (
	scanOnce            sync.Once
	classScanController objc.Class
	classScanDelegate   objc.Class
	scans               = map[objc.ID]*scan{}
)

// scan is a scanner on screen: the request it answers, the session reading codes and the navigation controller showing it.
type scan struct {
	s      *surface
	id     uint32
	r      driver.ScanRequest
	output objc.ID
	nav    objc.ID
	stop   func()
}

// scanCode has the user allow the camera, then shows the scanner, or answers with why it cannot (withCamera).
func (s *surface) scanCode(r driver.ScanRequest, id uint32) {
	fail := func(why string) { s.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: id, Text: why}) }
	withCamera("scanning a code", fail, func() {
		if why := s.presentScanner(r, id); why != "" {
			fail(why)
		}
	})
}

// presentScanner starts a session of the back camera reading r's codes and presents it, returning why it could not.
func (s *surface) presentScanner(r driver.ScanRequest, id uint32) string {
	device := apple.ScanCamera()
	if device == 0 {
		return driver.NoScanCamera
	}
	scanOnce.Do(registerScanner)
	session, output, why := apple.NewScanCapture(device, r)
	if why != "" {
		return why
	}
	delegate := objc.ID(classScanDelegate).Send(selNew)
	output.Send(sel("setMetadataObjectsDelegate:queue:"), delegate, apple.MainQueue())

	controller := objc.ID(classScanController).Send(selNew)
	defer controller.Send(selRelease)
	view := controller.Send(sel("view"))
	view.Send(sel("setBackgroundColor:"), objc.ID(apple.Class("UIColor")).Send(sel("blackColor")))
	preview := objc.ID(apple.Class("AVCaptureVideoPreviewLayer")).Send(sel("layerWithSession:"), session)
	preview.Send(sel("setVideoGravity:"), apple.NSString("AVLayerVideoGravityResizeAspectFill"))
	view.Send(sel("layer")).Send(sel("addSublayer:"), preview)
	cancel := objc.ID(apple.Class("UIBarButtonItem")).Send(selAlloc).Send(sel("initWithBarButtonSystemItem:target:action:"), barButtonCancel, delegate, sel("cancel:"))
	controller.Send(sel("navigationItem")).Send(sel("setLeftBarButtonItem:"), cancel)
	cancel.Send(selRelease)
	nav := objc.ID(apple.Class("UINavigationController")).Send(selAlloc).Send(sel("initWithRootViewController:"), controller)
	nav.Send(sel("setModalPresentationStyle:"), presentationFullScreen)
	// The bar sits over the camera's picture: dark, so its button reads in either appearance.
	nav.Send(sel("setOverrideUserInterfaceStyle:"), interfaceStyleDark)
	sc := &scan{s: s, id: id, r: r, output: output, nav: nav}
	scans[delegate] = sc
	s.topController().Send(sel("presentViewController:animated:completion:"), nav, true, objc.ID(0))
	sc.stop = apple.StartScanCapture(session, nil)
	return ""
}

// registerScanner registers the scanner's view controller, which keeps the camera's picture filling its view, and its delegate, which hears the codes read and the Cancel button.
func registerScanner() {
	classScanController = register("NsgoScanController", apple.Class("UIViewController"), nil, []objc.MethodDef{{
		Cmd: sel("viewDidLayoutSubviews"),
		Fn: func(self objc.ID, _ objc.SEL) {
			view := self.Send(sel("view"))
			bounds := objc.Send[apple.CGRect](view, selBounds)
			apple.ForEach(view.Send(sel("layer")).Send(sel("sublayers")), func(l objc.ID) { l.Send(selSetFrame, bounds) })
		},
	}})
	var protocols []*objc.Protocol
	if p := objc.GetProtocol("AVCaptureMetadataOutputObjectsDelegate"); p != nil {
		protocols = append(protocols, p)
	}
	classScanDelegate = register("NsgoScanDelegate", apple.Class("NSObject"), protocols, []objc.MethodDef{{
		Cmd: sel("captureOutput:didOutputMetadataObjects:fromConnection:"),
		Fn: func(self objc.ID, _ objc.SEL, _, objects, _ objc.ID) {
			sc := scans[self]
			if sc == nil {
				return
			}
			if code := apple.CodeFromMetadata(objects, sc.r); code != nil {
				endScan(self, code)
			}
		},
	}, {
		Cmd: sel("cancel:"),
		Fn:  func(self objc.ID, _ objc.SEL, _ objc.ID) { endScan(self, nil) },
	}})
}

// endScan stops the scanner of delegate, dismisses it and answers its request with code, nil for a cancel.
func endScan(delegate objc.ID, code *driver.ScannedCode) {
	sc := scans[delegate]
	if sc == nil {
		return
	}
	delete(scans, delegate)
	// No more codes come to the delegate, which goes now.
	sc.output.Send(sel("setMetadataObjectsDelegate:queue:"), objc.ID(0), objc.ID(0))
	sc.output.Send(selRelease)
	sc.stop()
	sc.nav.Send(sel("dismissViewControllerAnimated:completion:"), true, objc.ID(0))
	sc.nav.Send(selRelease)
	delegate.Send(selRelease)
	ev := driver.Event{Kind: driver.EventPick, Count: sc.id}
	if code != nil {
		ev.Data = code
	}
	sc.s.engine.Dispatch(ev)
}
