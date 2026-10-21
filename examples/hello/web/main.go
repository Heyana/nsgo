//go:build js && wasm

// Command web runs the hello example in a browser page.
package main

import (
	"github.com/oneo-me/nsgo/examples/hello/app"
	"github.com/oneo-me/nsgo/platform/web"
)

func main() { web.Run(app.Root, web.Options{}) }
