//go:build darwin

package apple

import (
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// metadataTypes are AVFoundation's AVMetadataObjectType values of each kind of code driver.ScanRequest asks for; an UPC-A code reads as an EAN-13 with a leading 0.
var metadataTypes = map[driver.CodeKind]string{
	driver.CodeQR:         "org.iso.QRCode",
	driver.CodeAztec:      "org.iso.Aztec",
	driver.CodeDataMatrix: "org.iso.DataMatrix",
	driver.CodePDF417:     "org.iso.PDF417",
	driver.CodeEAN13:      "org.gs1.EAN-13",
	driver.CodeEAN8:       "org.gs1.EAN-8",
	driver.CodeUPCE:       "org.gs1.UPC-E",
	driver.CodeCode128:    "org.iso.Code128",
	driver.CodeCode39:     "org.iso.Code39",
	driver.CodeCode93:     "com.intermec.Code93",
	driver.CodeITF:        "org.ansi.Interleaved2of5",
}

// MetadataTypes are the AVMetadataObjectTypes r reads.
func MetadataTypes(r driver.ScanRequest) []string {
	var types []string
	for k := driver.CodeQR; k <= driver.CodeITF; k++ {
		if r.Wants(k) {
			types = append(types, metadataTypes[k])
		}
	}
	return types
}

// MetadataKind is the kind of code of an AVMetadataObjectType, and whether it is one.
func MetadataKind(t string) (driver.CodeKind, bool) {
	for k, v := range metadataTypes {
		if v == t {
			return k, true
		}
	}
	return 0, false
}

// MainQueue is the main dispatch queue, as AVFoundation's delegates take it.
func MainQueue() objc.ID { return objc.ID(mainQueue) }

var loadScanCapture = sync.OnceValue(func() bool {
	const path = "/System/Library/Frameworks/AVFoundation.framework/AVFoundation"
	_, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	return err == nil && Class("AVCaptureMetadataOutput") != 0
})

// ScanCamera returns the default video capture device, when available.
func ScanCamera() objc.ID {
	if !loadScanCapture() {
		return 0
	}
	return objc.ID(Class("AVCaptureDevice")).Send(sel("defaultDeviceWithMediaType:"), NSString("vide"))
}

// NewScanCapture configures a session to read the requested metadata
// types. The caller owns the returned session and metadata output.
func NewScanCapture(device objc.ID, r driver.ScanRequest) (session, output objc.ID, why string) {
	input := objc.ID(Class("AVCaptureDeviceInput")).Send(sel("deviceInputWithDevice:error:"), device, objc.ID(0))
	session = objc.ID(Class("AVCaptureSession")).Send(sel("new"))
	if input == 0 || !objc.Send[bool](session, sel("canAddInput:"), input) {
		session.Send(sel("release"))
		return 0, 0, "the camera cannot be used"
	}
	session.Send(sel("addInput:"), input)
	output = objc.ID(Class("AVCaptureMetadataOutput")).Send(sel("new"))
	if !objc.Send[bool](session, sel("canAddOutput:"), output) {
		output.Send(sel("release"))
		session.Send(sel("release"))
		return 0, 0, "the camera cannot read codes"
	}
	session.Send(sel("addOutput:"), output)

	available := map[string]bool{}
	ForEach(output.Send(sel("availableMetadataObjectTypes")), func(t objc.ID) { available[GoString(t)] = true })
	types := objc.ID(Class("NSMutableArray")).Send(sel("array"))
	for _, t := range MetadataTypes(r) {
		if available[t] {
			types.Send(sel("addObject:"), NSString(t))
		}
	}
	if objc.Send[uint](types, sel("count")) == 0 {
		output.Send(sel("release"))
		session.Send(sel("release"))
		return 0, 0, "the camera reads none of the codes asked for"
	}
	output.Send(sel("setMetadataObjectTypes:"), types)
	return session, output, ""
}

// CodeFromMetadata returns the first requested code with a text value.
func CodeFromMetadata(objects objc.ID, r driver.ScanRequest) *driver.ScannedCode {
	var code *driver.ScannedCode
	ForEach(objects, func(o objc.ID) {
		if code != nil || !objc.Send[bool](o, sel("respondsToSelector:"), sel("stringValue")) {
			return
		}
		kind, ok := MetadataKind(GoString(o.Send(sel("type"))))
		text := o.Send(sel("stringValue"))
		if ok && r.Wants(kind) && text != 0 {
			code = &driver.ScannedCode{Kind: kind, Text: GoString(text)}
		}
	})
	return code
}

// NewPhotoCapture configures a session to take photos with device, for macOS's TakePhoto, which shows the scanner's camera. The caller owns the returned session and photo output.
func NewPhotoCapture(device objc.ID) (session, output objc.ID, why string) {
	if Class("AVCapturePhotoOutput") == 0 {
		return 0, 0, "the camera cannot take photos"
	}
	input := objc.ID(Class("AVCaptureDeviceInput")).Send(sel("deviceInputWithDevice:error:"), device, objc.ID(0))
	session = objc.ID(Class("AVCaptureSession")).Send(sel("new"))
	if input == 0 || !objc.Send[bool](session, sel("canAddInput:"), input) {
		session.Send(sel("release"))
		return 0, 0, "the camera cannot be used"
	}
	if preset := NSString("AVCaptureSessionPresetPhoto"); objc.Send[bool](session, sel("canSetSessionPreset:"), preset) {
		session.Send(sel("setSessionPreset:"), preset)
	}
	session.Send(sel("addInput:"), input)
	output = objc.ID(Class("AVCapturePhotoOutput")).Send(sel("new"))
	if !objc.Send[bool](session, sel("canAddOutput:"), output) {
		output.Send(sel("release"))
		session.Send(sel("release"))
		return 0, 0, "the camera cannot take photos"
	}
	session.Send(sel("addOutput:"), output)
	return session, output, ""
}

// StartScanCapture takes ownership of session and starts it off the main
// thread. Stop releases the session after startup returns, so a quick
// cancellation cannot leave the camera running. Failed runs on main.
func StartScanCapture(session objc.ID, failed func()) (stop func()) {
	return StartCapture(session, nil, failed)
}

// StartCapture is StartScanCapture that also runs running on main once the session runs.
func StartCapture(session objc.ID, running, failed func()) (stop func()) {
	started := make(chan struct{})
	go withPool(func() {
		session.Send(sel("startRunning"))
		ok := objc.Send[bool](session, sel("isRunning"))
		close(started)
		switch {
		case !ok && failed != nil:
			OnMain(failed)
		case ok && running != nil:
			OnMain(running)
		}
	})
	return func() {
		go func() {
			<-started
			withPool(func() {
				session.Send(sel("stopRunning"))
				session.Send(sel("release"))
			})
		}()
	}
}
