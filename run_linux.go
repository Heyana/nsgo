//go:build linux && !android

package nsgo

import "github.com/oneo-me/nsgo/internal/platform/linux"

func run(app App, o options) {
	w, h := windowSize()
	linux.Run(app, linux.Options{Title: name, Width: w, Height: h, ID: id, Components: o.components,
		Fonts: o.fonts, Appearance: o.appearance, Perf: o.monitor})
}
