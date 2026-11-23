//go:build windows && !nsgotui && !nsgoandroidhost

package backend

import (
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/windows"
)

func newView(app engine.App, o Options, h engine.HostOptions) View {
	v, err := windows.NewView(app, windows.Options{Title: name, ID: id, Components: o.Components, Fonts: o.Fonts,
		Appearance: o.Appearance, Host: h, Parent: o.Parent})
	if err != nil {
		panic("nsgo: " + err.Error())
	}
	return v
}
