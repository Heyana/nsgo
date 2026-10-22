//go:build linux

package nsgo

import "github.com/oneo-me/nsgo/platform/linux"

func run(app App) {
	w, h := windowSize()
	linux.Run(app, linux.Options{Title: name, Width: w, Height: h, ID: id})
}
