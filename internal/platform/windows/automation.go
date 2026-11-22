//go:build windows && nsgoautomation

package windows

import (
	"sync"

	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func init() { testhost.RegisterLive(liveSurface) }

var (
	liveMu      sync.Mutex
	liveWindows = map[uint32]*window{}
)

// trackLiveWindow records a running app's window under its surface ID (0 for the root window, cfg.ID for an opened window or panel) so nsgo dev -automation can act on it.
func trackLiveWindow(id uint32, w *window) {
	liveMu.Lock()
	liveWindows[id] = w
	liveMu.Unlock()
}

// untrackLiveWindow forgets w once it closes.
func untrackLiveWindow(id uint32, w *window) {
	liveMu.Lock()
	if liveWindows[id] == w {
		delete(liveWindows, id)
	}
	liveMu.Unlock()
}

// liveSurface returns surface id of graph as a test host's surface acts on it: the app's root window for 0 (a shown window for a root-view app, or the hidden root surface that answers app-level alerts and Jump List tasks for an app function), else the window or panel the app opened as id; nil for a tray icon, a surface gone or one another graph's engine runs. Graph 0, from a surface whose engine has not started, and a window without an engine yet match any graph.
func liveSurface(graph int, id uint32) testhost.Surface {
	if graph < 0 {
		return nil
	}
	liveMu.Lock()
	w := liveWindows[id]
	liveMu.Unlock()
	if w == nil {
		return nil
	}
	if g := w.engine.Graph(); graph != 0 && g != 0 && g != graph {
		return nil
	}
	if w.closed != nil {
		select {
		case <-w.closed:
			return nil
		default:
		}
	}
	return &testSurface{w: w}
}
