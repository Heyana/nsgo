//go:build ios && nsgoautomation

package ios

import "github.com/nsgo-dev/nsgo/internal/testhost"

// automationBuild is set in `nsgo dev -automation`'s builds: surfaces keep their alerts for the automation to answer, and it acts on the running app's scenes through liveSurface.
const automationBuild = true

func init() { testhost.RegisterLive(liveSurface) }

// liveSurface returns the running app's scene surface numbered id (0 for the first scene, else the tree ID of a further scene) as a test host's surface acts on it, or nil while no connected scene shows it.
func liveSurface(id uint32) testhost.Surface {
	var found *surface
	onMainSync(func() {
		for _, s := range scenes {
			if s != nil && s.tree == id && !s.closed && !s.ending && s.window != 0 && s.engine != nil {
				found = s
				break
			}
		}
	})
	if found == nil {
		return nil
	}
	return &testSurface{s: found}
}
