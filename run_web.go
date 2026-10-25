//go:build js && wasm

package nsgo

import "github.com/oneo-me/nsgo/internal/platform/web"

func run(app App, o options) {
	web.Run(app, web.Options{ID: id, Components: o.components, Fonts: o.fonts, Appearance: o.appearance})
}
