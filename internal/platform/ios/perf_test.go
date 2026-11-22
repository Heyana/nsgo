//go:build ios && nsgotest

package ios_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/ios"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// Closing a surface stops its performance HUD, which would otherwise keep sampling and hold the surface.
func TestClosingASurfaceStopsItsHud(t *testing.T) {
	nsgotest.Start(t, func(*core.Scope) core.Node { return core.View{} })
	stopped, forgotten, removed := ios.HudStopsWithSurfaceForTest()
	if !stopped {
		t.Error("a closed surface's HUD still samples")
	}
	if !forgotten {
		t.Error("a closed surface's HUD stays among the traces")
	}
	if !removed {
		t.Error("a closed surface keeps its HUD's panel")
	}
}
