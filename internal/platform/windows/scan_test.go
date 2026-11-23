//go:build windows

package windows

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

func TestScanHRESULTMessage(t *testing.T) {
	if got := scanHRESULTMessage(hrAccessDenied); got != "camera access denied" {
		t.Errorf("E_ACCESSDENIED = %q", got)
	}
	if got := scanHRESULTMessage(hrNoCaptureDevices); got != "this device has no camera" {
		t.Errorf("MF_E_NO_CAPTURE_DEVICES_AVAILABLE = %q", got)
	}
	if got := scanHRESULTMessage(0x80004005); got != "the camera could not be started" {
		t.Errorf("E_FAIL = %q", got)
	}
}

func TestScanFitAspect(t *testing.T) {
	box := rectF{0, 0, 400, 300}
	// 16:9 video inside 4:3 box letterboxes vertically.
	got := fitAspect(box, 1600, 900)
	if got.left != 0 || got.right != 400 || got.top != 37.5 || got.bottom != 262.5 {
		t.Errorf("16:9 in 4:3 = %+v, want {0 37.5 400 262.5}", got)
	}
	// 3:4 portrait video inside 4:3 box pillarboxes horizontally.
	got = fitAspect(box, 300, 400)
	if got.left != 87.5 || got.right != 312.5 || got.top != 0 || got.bottom != 300 {
		t.Errorf("3:4 in 4:3 = %+v, want {87.5 0 312.5 300}", got)
	}
	if got := fitAspect(box, 0, 0); got != box {
		t.Errorf("zero source = %+v, want %+v", got, box)
	}
}

type scanStubPlatform struct{}

func (scanStubPlatform) Prepare(*driver.AttributedText) driver.PreparedText { return nil }
func (scanStubPlatform) Mount(*driver.MountBatch)                           {}
func (scanStubPlatform) RequestFrame()                                      {}
func (scanStubPlatform) SubmitGPU(driver.GPUFrame)                          {}

func TestScanCodeErrorsAndLifecycle(t *testing.T) {
	origAvail, origPerm := scanCameraAvailable, scanPermissionState
	t.Cleanup(func() {
		scanCameraAvailable, scanPermissionState = origAvail, origPerm
	})

	events := make(chan driver.Event, 4)
	eng := engine.Start(scanStubPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{
		Event: func(_ uint32, e driver.Event) {
			if e.Kind == driver.EventPick {
				events <- e
			}
		},
	})
	defer eng.Close()

	wait := func() driver.Event {
		t.Helper()
		select {
		case ev := <-events:
			return ev
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for EventPick")
			return driver.Event{}
		}
	}

	w := &window{engine: eng}

	// Unsupported kind alone (PDF417) fails immediately without opening a camera.
	scanCameraAvailable = func() bool { return true }
	scanPermissionState = func(string) (int, bool) { return driver.PermissionGranted, false }
	w.scanCode(driver.ScanRequest{Kinds: []driver.CodeKind{driver.CodePDF417}}, 1)
	if ev := wait(); ev.Count != 1 || ev.Text != "the camera reads none of the codes asked for" {
		t.Fatalf("PDF417 event = %+v", ev)
	}

	// Missing camera fails cleanly.
	scanCameraAvailable = func() bool { return false }
	w.scanCode(driver.ScanRequest{}, 2)
	if ev := wait(); ev.Count != 2 || ev.Text != "this device has no camera" {
		t.Fatalf("no camera event = %+v", ev)
	}

	// Denied consent fails cleanly.
	scanCameraAvailable = func() bool { return true }
	scanPermissionState = func(string) (int, bool) { return driver.PermissionDenied, false }
	w.scanCode(driver.ScanRequest{}, 3)
	if ev := wait(); ev.Count != 3 || ev.Text != "camera access denied" {
		t.Fatalf("denied event = %+v", ev)
	}

	// Escape cancels an active scanner once.
	sc := &shownScanner{w: w, id: 4, req: driver.ScanRequest{}}
	w.scanner = sc
	if !w.scannerKey(vkEscape) {
		t.Fatal("scannerKey(vkEscape) returned false")
	}
	if w.scanner != nil {
		t.Fatal("w.scanner still set after Escape")
	}
	// Second finish is a no-op.
	w.finishScanner(sc, &driver.ScannedCode{Kind: driver.CodeQR, Text: "late"}, "")
	if ev := wait(); ev.Count != 4 || ev.Data != nil || ev.Text != "" {
		t.Fatalf("escape event = %+v", ev)
	}

	// Decoded code is delivered via finishScanner.
	sc2 := &shownScanner{w: w, id: 5, req: driver.ScanRequest{}}
	w.scanner = sc2
	w.finishScanner(sc2, &driver.ScannedCode{Kind: driver.CodeEAN13, Text: "4006381333931"}, "")
	ev := wait()
	code, ok := ev.Data.(*driver.ScannedCode)
	if ev.Count != 5 || !ok || code.Kind != driver.CodeEAN13 || code.Text != "4006381333931" {
		t.Fatalf("decoded event = %+v", ev)
	}
}

// TestTakePhotoTakesTheFrameShown runs TakePhoto with the camera's capture stubbed out: without a camera it fails with driver.NoCamera, the shutter takes nothing before a frame shows and then answers with the frame as a JPEG scaled to MaxDimension, Return is the shutter, and Escape answers with no photo.
func TestTakePhotoTakesTheFrameShown(t *testing.T) {
	origAvail, origPerm, origStart := scanCameraAvailable, scanPermissionState, scanStartCapture
	t.Cleanup(func() { scanCameraAvailable, scanPermissionState, scanStartCapture = origAvail, origPerm, origStart })
	scanStartCapture = func(*shownScanner) {}
	scanPermissionState = func(string) (int, bool) { return driver.PermissionGranted, false }
	events := make(chan driver.Event, 4)
	eng := engine.Start(scanStubPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{
		Event: func(_ uint32, e driver.Event) {
			if e.Kind == driver.EventPick {
				events <- e
			}
		},
	})
	defer eng.Close()
	wait := func() driver.Event {
		t.Helper()
		select {
		case ev := <-events:
			return ev
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for EventPick")
			return driver.Event{}
		}
	}
	w := &window{engine: eng}

	scanCameraAvailable = func() bool { return false }
	w.takePhoto(driver.PickRequest{Limit: 1}, 1)
	if ev := wait(); ev.Count != 1 || ev.Text != driver.NoCamera || w.scanner != nil {
		t.Fatalf("no camera answers %+v", ev)
	}

	scanCameraAvailable = func() bool { return true }
	w.takePhoto(driver.PickRequest{Limit: 1, MaxDimension: 4}, 2)
	sc := w.scanner
	if sc == nil || sc.photo == nil {
		t.Fatal("TakePhoto opened no camera")
	}
	w.shootPhoto(sc)
	if w.scanner != sc {
		t.Fatal("the shutter closed the camera before a frame showed")
	}
	sc.framePix, sc.frameW, sc.frameH = make([]byte, 8*4*4), 8, 4
	if !w.scannerKey(vkReturn) || w.scanner != nil {
		t.Fatal("Return did not take the photo")
	}
	ev := wait()
	files, _ := ev.Data.([]driver.PickedFile)
	if ev.Count != 2 || ev.Text != "" || len(files) != 1 || files[0].MIME != "image/jpeg" || files[0].Size != (driver.Size{Width: 4, Height: 2}) {
		t.Fatalf("the shutter answers %+v", ev)
	}

	w.takePhoto(driver.PickRequest{Limit: 1}, 3)
	if !w.scannerKey(vkEscape) || w.scanner != nil {
		t.Fatal("Escape did not close the camera")
	}
	if ev := wait(); ev.Count != 3 || ev.Data != nil || ev.Text != "" {
		t.Fatalf("Escape answers %+v", ev)
	}
}
