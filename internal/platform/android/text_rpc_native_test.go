//go:build android && nsgotest

package android

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func TestTextRPCReportsErrorsAndRecoversOnDevice(t *testing.T) {
	live, err := testhost.Current().Open(func(*core.Scope) core.Node { return core.View{} }, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	live.Sync()
	s := live.(*testSurface).s
	for _, tc := range []struct {
		kind byte
		want string
	}{
		{textPrepare, "EOFException"},
		{textMeasure, "unknown prepared text"},
		{textCarets, "unknown text layout"},
		{255, "unknown text operation"},
	} {
		_, got := s.textRPC.call(tc.kind, textTimeout, func(w *writer) { w.u32(0) })
		if got.err == nil || !strings.Contains(got.err.Error(), tc.want) {
			t.Fatalf("%s error = %v, want %q", textOperation(tc.kind), got.err, tc.want)
		}
	}
	a := driver.AttributedText{Text: "RPC text", Style: driver.TextStyle{Font: driver.Font{Size: 16}}}
	missing := preparedText{s: s, ready: true, text: a}
	if size := missing.Measure(120); size.Width <= 0 || size.Height <= 0 {
		t.Fatalf("a failed measurement returned an empty estimate: %+v", size)
	}
	p := s.Prepare(&a)
	defer p.Release()
	size := p.Measure(120)
	l := p.Layout(size).(*textLayout)
	defer l.Release()
	if size.Width <= 0 || size.Height <= 0 || !l.native || len(l.TextLines()) != 1 || len(l.caretLines()) != 1 {
		t.Fatalf("text RPCs did not recover after the rejected requests: size %+v, native %t", size, l.native)
	}
}

// Withhold the rest of a picture while the native text engine prepares,
// measures and lays out a string on the same pipe.
func TestTextRPCPassesAnUnfinishedImageOnDevice(t *testing.T) {
	live, err := testhost.Current().Open(func(*core.Scope) core.Node { return core.View{} }, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	live.Sync()
	s := live.(*testSurface).s
	for end := time.Now().Add(2 * time.Second); !s.bulkReady.Load(); {
		if time.Now().After(end) {
			t.Fatal("runtime did not acknowledge bulk transfers")
		}
		time.Sleep(time.Millisecond)
	}

	// An image for a node no longer mounted still goes through the runtime
	// framing and image dispatch, and is discarded on the UI thread.
	const bytes = 2 * bulkChunk
	var metadata writer
	metadata.u32(0)
	metadata.u32(^uint32(0))
	metadata.u32(1)
	metadata.byte(imageSource)
	metadata.floats(1, 0, 0)
	metadata.u32(bytes)
	var fragments [][]byte
	var encode bulkWriter
	if err := encode.write(msgImage, metadata.buf, make([]byte, bytes), func(part []byte) error {
		fragments = append(fragments, append([]byte(nil), part...))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	send := func(part []byte) {
		t.Helper()
		if err := s.sendExtended(extBulk, func(w *writer) { w.buf = append(w.buf, part...) }); err != nil {
			t.Fatal(err)
		}
	}
	send(fragments[0])

	p := s.Prepare(&driver.AttributedText{Text: "text ahead of a picture", Style: driver.TextStyle{Font: driver.Font{Size: 16}}})
	defer p.Release()
	size := p.Measure(160)
	layout := p.Layout(size).(*textLayout)
	defer layout.Release()
	if !layout.native || size.Width <= 0 || len(layout.TextLines()) == 0 {
		t.Fatalf("text did not finish natively with an incomplete image: %+v, native %t", size, layout.native)
	}
	for _, part := range fragments[1:] {
		send(part)
	}
	live.Sync()
}
