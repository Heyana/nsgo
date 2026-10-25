//go:build android

package nsgo

import "github.com/oneo-me/nsgo/internal/platform/android"

func run(app App, o options) {
	android.Run(app, android.Options{Components: o.components, Fonts: o.fonts})
}
