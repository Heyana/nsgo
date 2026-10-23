//go:build js && wasm

package nsgo

import "github.com/oneo-me/nsgo/platform/web"

func run(app App, o options) { web.Run(app, web.Options{ID: id, Components: o.components}) }
