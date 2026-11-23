//go:build android && nsgotest

package android

import (
	"encoding/base64"
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
	legacy := preparedText{s: s, id: s.ids.Add(1), text: a}
	_, legacy.ready = s.requestText(textPrepare, func(w *writer) {
		writeLegacyPreparedText(w, legacy.id, &a)
	})
	defer legacy.Release()
	if old := legacy.Measure(120); !legacy.ready || old != size {
		t.Fatalf("legacy prepare returned %+v, want %+v (ready %t)", old, size, legacy.ready)
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

func TestTextCachesFollowEngineAndSurfaceLifetimesOnDevice(t *testing.T) {
	live, err := testhost.Current().Open(func(*core.Scope) core.Node { return core.View{} }, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	live.Sync()
	target := live.(*testSurface)
	s := target.s
	step := func(op map[string]any) {
		t.Helper()
		if err := target.do(op); err != nil {
			t.Fatal(err)
		}
	}
	text := driver.AttributedText{Text: "Engine-owned 甲😀", Style: driver.TextStyle{Font: driver.Font{Size: 16}}}
	check := func(owner uint32) *preparedText {
		t.Helper()
		p := s.prepare(owner, &text)
		size := p.Measure(200)
		layout := p.Layout(size).(*textLayout)
		if !p.ready || !layout.native || size.Height <= 0 || len(layout.TextLines()) == 0 || len(layout.caretLines()) == 0 {
			t.Fatalf("surface %d text unavailable: size %+v, ready %t, native %t", owner, size, p.ready, layout.native)
		}
		layout.Release()
		return &p
	}
	primary := check(0)
	defer primary.Release()
	step(map[string]any{"op": "detach-view"})
	defer func() { _ = target.do(map[string]any{"op": "reattach-view"}) }()
	check(0).Release()
	const first, second = uint32(0x7fff0001), uint32(0x7fff0002)
	for id, density := range map[uint32]float64{first: 1.25, second: 2.5} {
		step(map[string]any{"op": "text-cache-surface", "surface": id, "density": density})
		defer func(id uint32) {
			_ = target.do(map[string]any{"op": "text-cache-surface", "surface": id, "remove": true})
		}(id)
	}
	removed, sibling := check(first), check(second)
	defer removed.Release()
	defer sibling.Release()
	step(map[string]any{"op": "text-cache-surface", "surface": first, "remove": true})
	measure := func(p *preparedText) textReply {
		_, reply := s.textRPC.call(textMeasure, textTimeout, func(w *writer) {
			w.u32(p.id)
			w.float(200)
			w.u32(p.owner)
		})
		return reply
	}
	if reply := measure(removed); reply.err == nil || !strings.Contains(reply.err.Error(), "unknown text surface") {
		t.Fatalf("removed surface reply = %v", reply.err)
	}
	step(map[string]any{"op": "text-cache-surface", "surface": first, "density": 2.5})
	if reply := measure(removed); reply.err == nil || !strings.Contains(reply.err.Error(), "unknown prepared text") {
		t.Fatalf("recreated surface retained old text: %v", reply.err)
	}
	check(first).Release()
	if reply := measure(sibling); reply.err != nil {
		t.Fatalf("removing another surface discarded sibling text: %v", reply.err)
	}
	step(map[string]any{"op": "reattach-view"})
	if reply := measure(primary); reply.err != nil {
		t.Fatalf("reattaching lost the primary cache: %v", reply.err)
	}
	var request writer
	request.u64(1)
	request.byte(textPrepare)
	request.u32(1)
	writeTextPrepare(&request, &text, 0)
	step(map[string]any{"op": "text-cache-engine-close", "request": base64.StdEncoding.EncodeToString(request.buf)})
	if reply := measure(primary); reply.err != nil {
		t.Fatalf("closing another engine discarded primary text: %v", reply.err)
	}
}
