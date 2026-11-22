//go:build darwin && !ios && !nsgotui

package backend

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/macos"
)

// run runs the app on its own surface, which opens its windows.
func run(root engine.App, app func(*core.Scope), o Options) {
	macos.Run(core.AppRoot(appOf(root, app)), macos.Options{
		Updates: updateConfig(), Title: name, Width: o.Width, Height: o.Height, Frame: o.Frame, ID: id,
		Hidden:     o.Hidden,
		Components: o.Components, Perf: o.Monitor, Material: o.Material, Constraints: o.Constraints, Fonts: o.Fonts,
		Appearance: o.Appearance, HideDock: o.HideDock, LaunchedAtLogin: launch.launchedAtLogin,
		Transparent: o.Transparent, NoShadow: o.NoShadow, SkipTaskbar: o.SkipTaskbar,
	})
}
