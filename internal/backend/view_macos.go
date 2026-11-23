//go:build darwin && !ios && !nsgotui && !nsgoandroidhost

package backend

import (
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/macos"
)

func newView(app engine.App, o Options, h engine.HostOptions) View {
	return macos.NewView(app, macos.Options{Title: name, Width: o.Width, Height: o.Height, ID: id, Components: o.Components,
		Fonts: o.Fonts, Appearance: o.Appearance, Host: h})
}
