//go:build linux && !android && nsgoautomation

package linux

import (
	"sync"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// liveWindows holds the standalone app's windows for the agent. Keep the same testSurface between requests so its input state follows the window.
var liveWindows sync.Map // *window -> *testSurface

func init() {
	testhost.RegisterLive(func(graph int, id uint32) testhost.Surface {
		var found *testSurface
		// The engine learns its graph on its own goroutine after the window registers, so the graph is compared as the agent asks.
		liveWindows.Range(func(key, surface any) bool {
			if w := key.(*window); w.surfaceID == id && graph > 0 && w.engine.Graph() == graph {
				found = surface.(*testSurface)
				return false
			}
			return true
		})
		if found == nil {
			return nil
		}
		return found
	})
}

func registerLiveWindow(w *window) {
	if w.opts.test == nil && w.kind != driver.SurfaceTrayIcon {
		liveWindows.Store(w, &testSurface{w: w})
	}
}

func unregisterLiveWindow(w *window) { liveWindows.Delete(w) }
