//go:build darwin && !ios && nsgoautomation

package macos

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// automationBuild is set in `nsgo dev -automation`'s builds: surfaces keep their alerts for the automation to answer, and it acts on the running app's windows through liveSurface.
const automationBuild = true

func init() { testhost.RegisterLive(liveSurface) }

// liveSurface returns surface id of graph as a test host's surface acts on it: a windowing app's own surface (id 0, not embedded), which shows no window but answers its alerts and chooses the main menu's items, any window or panel an engine opened as id, or an embedded view (NewView, id 0) mounted in a host window; nil for a tray icon's view, an unmounted embedded view or a surface gone.
func liveSurface(graph int, id uint32) testhost.Surface {
	if graph <= 0 {
		return nil
	}
	var found *surface
	onMainSync(func() {
		for s := range surfaces {
			if s.closed || s.engine == nil || s.id != id || s.kind == driver.SurfaceTrayIcon {
				continue
			}
			if s.engine.Graph() != graph {
				continue
			}
			if s.window == 0 && (s.embedded || id != 0) {
				continue
			}
			found = s
			break
		}
	})
	if found == nil {
		return nil
	}
	return &testSurface{s: found}
}
