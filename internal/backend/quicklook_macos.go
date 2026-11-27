//go:build darwin && !ios && !nsgotui && !nsgoandroidhost

package backend

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/macos"
)

// RunQuickLook runs a macOS QuickLook Preview Extension hosting preview.
func RunQuickLook(preview func(cx *core.Scope, path string) core.Node, o Options, h engine.HostOptions) {
	macos.RunQuickLook(func(path, url string) (engine.App, engine.HostOptions) {
		host := core.NewHost()
		host.SetParams(map[string]any{"path": path, "url": url})
		ho := h
		ho.Channel = host
		ho.Settings = &engine.HostSettings{}
		return func(cx *core.Scope) core.Node {
			return preview(cx, path)
		}, ho
	}, macos.Options{
		Title:      name,
		Width:      o.Width,
		Height:     o.Height,
		ID:         id,
		Components: o.Components,
		Fonts:      o.Fonts,
		Appearance: o.Appearance,
	})
}
