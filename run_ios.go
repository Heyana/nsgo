//go:build ios && !nsgoembed

package nsgo

import "github.com/oneo-me/nsgo/internal/platform/ios"

func run(app App, o options) {
	ios.Run(app, ios.Options{ID: id, Components: o.components, Fonts: o.fonts, Appearance: o.appearance, Perf: o.monitor})
}
