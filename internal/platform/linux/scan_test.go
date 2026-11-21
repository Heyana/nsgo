//go:build linux && !android

package linux

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

func TestScanPipelineDescription(t *testing.T) {
	desc := scanPipelineDescription(42)
	for _, want := range []string{"pipewiresrc fd=42", "videoconvert", "video/x-raw,format=BGRA", "appsink name=sink"} {
		if !strings.Contains(desc, want) {
			t.Errorf("pipeline %q missing %q", desc, want)
		}
	}
}

func TestScanPortalResponseMessage(t *testing.T) {
	if got := scanPortalResponseMessage(0); got != "" {
		t.Errorf("response 0 = %q, want empty", got)
	}
	if got := scanPortalResponseMessage(1); got != "camera access denied" {
		t.Errorf("response 1 = %q, want denied", got)
	}
	if got := scanPortalResponseMessage(2); got != "camera access failed" {
		t.Errorf("response 2 = %q, want failed", got)
	}
}

type scanStubPlatform struct{}

func (scanStubPlatform) Prepare(*driver.AttributedText) driver.PreparedText { return nil }
func (scanStubPlatform) Mount(*driver.MountBatch)                           {}
func (scanStubPlatform) RequestFrame()                                      {}
func (scanStubPlatform) SubmitGPU(driver.GPUFrame)                          {}

func TestScanCodeErrorsAndLifecycle(t *testing.T) {
	origGST := scanGSTAvailable
	t.Cleanup(func() { scanGSTAvailable = origGST })

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

	// Unsupported kind alone (PDF417) fails immediately.
	scanGSTAvailable = func() bool { return true }
	w.scanCode(driver.ScanRequest{Kinds: []driver.CodeKind{driver.CodePDF417}}, 1)
	if ev := wait(); ev.Count != 1 || ev.Text != "the camera reads none of the codes asked for" {
		t.Fatalf("PDF417 event = %+v", ev)
	}

	// Missing GStreamer pipewiresrc fails cleanly.
	scanGSTAvailable = func() bool { return false }
	w.scanCode(driver.ScanRequest{}, 2)
	if ev := wait(); ev.Count != 2 || ev.Text != "GStreamer pipewiresrc is unavailable" {
		t.Fatalf("missing gst event = %+v", ev)
	}

	// closeScanner cancels an active scan once.
	sc := &codeScanner{w: w, id: 3, req: driver.ScanRequest{}}
	w.scanner = sc
	w.closeScanner()
	if w.scanner != nil {
		t.Fatal("w.scanner still set after closeScanner")
	}
	// Second finish is a no-op.
	w.finishScanner(sc, &driver.ScannedCode{Kind: driver.CodeQR, Text: "late"}, "")
	if ev := wait(); ev.Count != 3 || ev.Data != nil || ev.Text != "" {
		t.Fatalf("cancel event = %+v", ev)
	}

	// Decoded code is delivered via finishScanner.
	sc2 := &codeScanner{w: w, id: 4, req: driver.ScanRequest{}}
	w.scanner = sc2
	w.finishScanner(sc2, &driver.ScannedCode{Kind: driver.CodeCode128, Text: "NSGO-42"}, "")
	ev := wait()
	code, ok := ev.Data.(*driver.ScannedCode)
	if ev.Count != 4 || !ok || code.Kind != driver.CodeCode128 || code.Text != "NSGO-42" {
		t.Fatalf("decoded event = %+v", ev)
	}
}
