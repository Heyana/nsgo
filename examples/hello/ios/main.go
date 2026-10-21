//go:build ios

// Command ios runs the hello example full screen on iOS.
package main

import (
	"github.com/oneo-me/nsgo/examples/hello/app"
	"github.com/oneo-me/nsgo/platform/ios"
)

func main() { ios.Run(app.Root) }
