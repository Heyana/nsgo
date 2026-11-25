//go:build js && wasm && !nsgominiprogram && !nsgoharmony

package backend

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestWebOptionsForwardsMonitor(t *testing.T) {
	if got := webOptions(Options{}); got.Perf {
		t.Fatal("default Options enabled web Perf")
	}
	got := webOptions(Options{Monitor: true, Simulate: core.OSIOS, Appearance: core.AppearanceDark})
	if !got.Perf || got.OS != core.OSIOS || got.Appearance != core.AppearanceDark {
		t.Fatalf("webOptions = %+v, want Perf=true OS=ios Appearance=dark", got)
	}
}
