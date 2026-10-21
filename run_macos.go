//go:build darwin && !ios

package nsgo

import "github.com/oneo-me/nsgo/platform/macos"

func run(app App) {
	w, h := windowSize()
	macos.Run(app, macos.Options{Title: name, Width: w, Height: h})
}
