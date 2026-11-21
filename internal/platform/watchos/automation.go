//go:build ios && nsgoautomation

package watchos

import "github.com/nsgo-dev/nsgo/internal/testhost"

func init() { testhost.RegisterLive(liveSurface) }

// liveSurface returns the running watch app's surface as a test host's surface acts on it: the watch's one surface for 0, nil for any other or before the host started its engine.
func liveSurface(_ int, id uint32) testhost.Surface {
	if id != 0 {
		return nil
	}
	var ready bool
	onMainSync(func() { ready = s != nil && s.engine != nil })
	if !ready {
		return nil
	}
	return &testSurface{}
}
