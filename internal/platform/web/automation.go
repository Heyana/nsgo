//go:build js && wasm && nsgoautomation

package web

import "github.com/nsgo-dev/nsgo/internal/testhost"

func init() { testhost.RegisterLive(liveSurface) }

// liveSurface returns the running page's surface for surface 0 of its graph as
// a test host's surface acts on it, or nil before the page opened or for
// another graph or surface.
func liveSurface(graph int, id uint32) testhost.Surface {
	if id != 0 || live == nil || live.engine == nil || live.engine.Graph() != graph {
		return nil
	}
	return &testSurface{s: live}
}
