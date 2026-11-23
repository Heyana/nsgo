//go:build windows && !nsgotui && !nsgoandroidhost

package backend

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/windows"
)

func run(root engine.App, app func(*core.Scope), o Options) {
	root, app = hiddenMain(root, app, o)
	windows.Run(rootOf(root, app), windows.Options{Title: name, Width: o.Width, Height: o.Height, Frame: o.Frame, Hidden: app != nil, MainHidden: o.Hidden, ID: id,
		Components: o.Components, Material: o.Material, Constraints: o.Constraints, Fonts: o.Fonts, Appearance: o.Appearance, Perf: o.Monitor,
		Transparent: o.Transparent, NoShadow: o.NoShadow, SkipTaskbar: o.SkipTaskbar,
		Updates: updateConfig(), RegisterToast: launch.registerToast, UnregisterToast: launch.unregisterToast, DockTask: launch.dockTask, LaunchedAtLogin: launch.launchedAtLogin, InstanceKey: o.InstanceKey})
}
