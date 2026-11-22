//go:build android && nsgoautomation

package android

import (
	"sync"

	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func init() { testhost.RegisterLive(liveSurface) }

var (
	liveMu sync.Mutex
	live   *testSurface
)

// setLiveSurface wires the running app's primary surface to a testSurface so nsgo dev -automation's agent can drive it through the runtime's TestAgent.
func setLiveSurface(s *surface) {
	t := newTestSurface(s, nil)
	liveMu.Lock()
	live = t
	liveMu.Unlock()
}

// clearLiveSurface releases the live surface once its pipe to the runtime closes.
func clearLiveSurface(s *surface) {
	liveMu.Lock()
	t := live
	if t != nil && t.s == s {
		live = nil
	} else {
		t = nil
	}
	liveMu.Unlock()
	if t != nil {
		t.close()
	}
}

// liveSurface returns the running app's primary surface (0) once its engine has started, or nil for other surfaces or before start.
func liveSurface(_ int, id uint32) testhost.Surface {
	if id != 0 {
		return nil
	}
	liveMu.Lock()
	t := live
	liveMu.Unlock()
	if t == nil {
		return nil
	}
	select {
	case <-t.s.started:
		return t
	default:
		return nil
	}
}
