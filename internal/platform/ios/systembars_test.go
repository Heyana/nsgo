//go:build ios && nsgotest

package ios_test

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/ios"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestImmersiveIOSSystemBars(t *testing.T) {
	if nsgotest.Target() == "tvos_simulator" {
		t.Skip("requires iOS system bars")
	}
	var set func(bool)
	var safe atomic.Pointer[core.Edges]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		set = func(on bool) { cx.Post(func() { mobile.Immersive(cx, on) }) }
		cx.Effect(func() {
			insets := cx.Env().SafeArea()
			safe.Store(&insets)
		})
		return core.View{}
	})
	for _, immersive := range []bool{false, true, false} {
		set(immersive)
		app.WaitUntil(5*time.Second, func() bool {
			status, home, insets := ios.SystemBarsForTest()
			got := safe.Load()
			return status == immersive && home == immersive && got != nil && *got == insets
		})
	}
}
