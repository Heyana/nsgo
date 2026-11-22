//go:build ios && nsgotest

package ios

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// HudStopsWithSurfaceForTest shows a performance HUD over a surface of its own, closes the surface and reports whether the HUD stopped sampling, left the traces and let its panel go.
func HudStopsWithSurfaceForTest() (stopped, forgotten, removed bool) {
	onMainSync(func() {
		s := newSurface(nil, Options{})
		s.native = objc.ID(apple.Class("UIView")).Send(selAlloc).Send(sel("initWithFrame:"), apple.Rect(0, 0, 200, 100))
		defer s.native.Send(selRelease)
		s.perf = engine.NewPerfMonitor()
		s.hud = newHud(s, s.perf)
		h := s.hud
		h.panel.Send(sel("retain"))
		defer h.panel.Send(selRelease)
		s.close()
		select {
		case <-h.done:
			stopped = s.hud == nil && s.perf == nil
		default:
		}
		_, kept := perfTraces[h.trace]
		forgotten = !kept
		removed = h.panel.Send(sel("superview")) == 0
	})
	return stopped, forgotten, removed
}
