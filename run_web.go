//go:build js && wasm

package nsgo

import "github.com/oneo-me/nsgo/platform/web"

func run(app App) { web.Run(app, web.Options{}) }
