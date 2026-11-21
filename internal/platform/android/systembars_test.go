//go:build android && nsgotest

package android

import (
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/testhost"
	"github.com/nsgo-dev/nsgo/mobile"
)

func TestImmersiveAndroidSystemBars(t *testing.T) {
	var scope *core.Scope
	var safe atomic.Pointer[core.Edges]
	surface, err := testhost.Current().Open(func(cx *core.Scope) core.Node {
		scope = cx
		cx.Effect(func() {
			insets := cx.Env().SafeArea()
			safe.Store(&insets)
		})
		return core.View{}
	}, testhost.Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := surface.(*testSurface)
	s.Sync()

	for _, immersive := range []bool{false, true, false} {
		scope.Post(func() { mobile.Immersive(scope, immersive) })
		s.Sync()
		deadline := time.Now().Add(5 * time.Second)
		for {
			var bars struct {
				Ready, Status, Navigation, Transient bool
				Safe                                 core.Edges
			}
			data, err := s.ask(map[string]any{"op": "system-bars"})
			if err != nil {
				t.Fatal(err)
			}
			if data == "" {
				t.Skip("window insets visibility requires Android API 30")
			}
			if err := json.Unmarshal([]byte(data), &bars); err != nil {
				t.Fatal(err)
			}
			got := safe.Load()
			if bars.Ready && bars.Status == !immersive && bars.Navigation == !immersive && (!immersive || bars.Transient) && got != nil && *got == bars.Safe {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("immersive %v: window bars %+v, reported safe area %v", immersive, bars, got)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}
