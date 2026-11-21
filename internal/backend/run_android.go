//go:build android

package backend

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/android"
)

func run(root engine.App, app func(*core.Scope), o Options) {
	updates := updateConfig()
	updates.Store = android.InstalledBy()
	root, app = withUpdateChecks(root, app, updates, false)
	android.Run(rootOf(root, app), android.Options{Components: o.Components, Fonts: o.Fonts, Appearance: o.Appearance,
		Perf: o.Monitor, Updates: updates})
}
