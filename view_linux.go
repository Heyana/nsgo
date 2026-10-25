//go:build linux && !android

package nsgo

import (
	"github.com/oneo-me/nsgo/internal/engine"
	"github.com/oneo-me/nsgo/internal/platform/linux"
)

func newView(app App, o options, h engine.HostOptions) platformView {
	v, err := linux.NewView(app, linux.Options{Title: name, ID: id, Components: o.components, Fonts: o.fonts,
		Appearance: o.appearance, Host: h})
	if err != nil {
		panic("nsgo: " + err.Error())
	}
	return v
}
