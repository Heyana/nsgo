//go:build android && nsgotest

package android

import (
	"strings"
	"testing"

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
