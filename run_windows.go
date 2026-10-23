//go:build windows

package nsgo

import "github.com/oneo-me/nsgo/platform/windows"

func run(app App, o options) {
	w, h := windowSize()
	windows.Run(app, windows.Options{Title: name, Width: w, Height: h, ID: id, Components: o.components, Material: o.material})
}
