//go:build nsgo_steam && ((darwin && !nsgoembed && !nsgowatch && !nsgotui) || (linux && !nsgotui) || (windows && !nsgotui) || android || nsgoandroidhost)

package backend

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

func withUpdateChecks(root engine.App, app func(*core.Scope), _ updates.Config, _ bool) (engine.App, func(*core.Scope)) {
	return root, app
}
