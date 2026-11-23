package android

import (
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestCaptureCapabilityGate keeps ItemCapture away from a runtime built before SurfaceCapture.java, which would misread the rest of the batch, answering it unavailable instead, and passes it to a runtime that sets capCapture.
func TestCaptureCapabilityGate(t *testing.T) {
	batch := &driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemCapture, Count: 7},
		{Kind: driver.ItemKeepAwake},
		{Kind: driver.ItemCapture, ID: 3, Count: 8},
	}}
	var answers []driver.Event
	out := filterCaptures(false, batch, func(ev driver.Event) { answers = append(answers, ev) })
	if len(batch.Items) != 3 || len(out.Items) != 1 || out.Items[0].Kind != driver.ItemKeepAwake {
		t.Fatalf("an older runtime received capture items, or the original batch changed: %+v", out.Items)
	}
	if len(answers) != 2 {
		t.Fatalf("answers %+v, want one per capture", answers)
	}
	for i, want := range []uint32{7, 8} {
		if ev := answers[i]; ev.Kind != driver.EventFiles || ev.Count != want || ev.Text != driver.NoCapture {
			t.Errorf("capture %d answered %+v, want EventFiles %d with driver.NoCapture", i, ev, want)
		}
	}
	if filterCaptures(true, batch, func(driver.Event) { t.Fatal("a capable runtime's capture was answered in Go") }) != batch {
		t.Fatal("a capable runtime's batch was copied")
	}

	if capCapture == 0 || capCapture&(capCapture-1) != 0 || capCapture&(capExtendedSensors|capAuthSession|capVideoPiP) != 0 {
		t.Fatalf("capCapture %d is not a free single bit of the fourth byte", capCapture)
	}
	protocol, err := os.ReadFile("java/dev/oneo/nsgo/Protocol.java")
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`static final int CAP_CAPTURE = (\d+);`).FindSubmatch(protocol); m == nil || string(m[1]) != strconv.Itoa(capCapture) {
		t.Fatalf("Protocol.java's CAP_CAPTURE is %q, protocol.go's capCapture %d", m, capCapture)
	}
}
