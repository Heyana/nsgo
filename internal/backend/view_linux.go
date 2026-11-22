//go:build linux && !android && !nsgotui

package backend

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/linux"
)

func newView(app engine.App, o Options, h engine.HostOptions) View {
	v, err := linux.NewView(app, linux.Options{Title: name, Name: core.App().Name, ID: id, Components: o.Components, Fonts: o.Fonts,
		Appearance: o.Appearance, Host: h})
	if err != nil {
		panic("nsgo: " + err.Error())
	}
	return v
}
