//go:build !nsgo_steam

package backend

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
	"github.com/nsgo-dev/nsgo/updater"
)

// withUpdateChecks returns what Run runs, checking for updates in the
// background at most once a day when nsgo.yaml's updates.automatic is on
// and c has anything to check (Config.Checks), for the platforms whose
// updater has no schedule of its own: Linux, Android and iOS. It checks as
// the app starts and whenever the user comes back to it, as a phone's app
// stays in memory for days, and never while the app is out of sight.
func withUpdateChecks(root engine.App, app func(*core.Scope), c updates.Config, appStore bool) (engine.App, func(*core.Scope)) {
	if !c.Checks(appStore) {
		return root, app
	}
	inner := appOf(root, app)
	return nil, func(cx *core.Scope) {
		active := false
		cx.Effect(func() {
			now := cx.Env().Lifecycle() == core.LifecycleActive && updater.Status(cx).AutomaticChecks
			if now && !active {
				// Under the app's scope, which outlives this run of the effect.
				cx.Run(func() { updater.CheckDaily(cx) })
			}
			active = now
		})
		inner(cx)
	}
}
