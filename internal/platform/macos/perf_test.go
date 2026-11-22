//go:build darwin && !ios

package macos

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/engine"
)

// Closing a surface stops its performance HUD, which would otherwise keep sampling and hold the surface.
func TestClosingASurfaceStopsItsHud(t *testing.T) {
	onMain(func() {
		loadAppKit()
		s := newSurface(Options{Width: 200, Height: 100}, false)
		defer func() {
			delete(views, s.native)
			s.native.Send(selRelease)
		}()
		s.perf = engine.NewPerfMonitor()
		s.hud = newHud(s, s.perf)
		h := s.hud
		h.panel.Send(sel("retain"))
		defer h.panel.Send(selRelease)
		s.close()
		select {
		case <-h.done:
		default:
			t.Error("a closed surface's HUD still samples")
		}
		if _, ok := perfTraces[h.trace]; ok {
			t.Error("a closed surface's HUD stays among the traces")
		}
		if h.panel.Send(selSuperview) != 0 {
			t.Error("a closed surface keeps its HUD's panel")
		}
		if s.hud != nil || s.perf != nil {
			t.Error("a closed surface keeps its HUD")
		}
	})
}
