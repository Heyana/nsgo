//go:build android

// Command android serves the hello example to NSGo's Android runtime.
package main

import (
	"github.com/oneo-me/nsgo/examples/hello/app"
	"github.com/oneo-me/nsgo/platform/android"
)

func main() { android.Run(app.Root) }
