//go:build darwin && !ios

// Command macos runs the hello example in an AppKit window.
package main

import (
	"github.com/oneo-me/nsgo/examples/hello/app"
	"github.com/oneo-me/nsgo/platform/macos"
)

func main() {
	macos.Run(app.Root, macos.Options{Title: "NSGo Hello", Width: 420, Height: 560})
}
