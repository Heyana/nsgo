//go:build windows

// Command windows runs the hello example in a Win32 window.
package main

import (
	"github.com/oneo-me/nsgo/examples/hello/app"
	"github.com/oneo-me/nsgo/platform/windows"
)

func main() {
	windows.Run(app.Root, windows.Options{Title: "NSGo Hello", Width: 420, Height: 560})
}
