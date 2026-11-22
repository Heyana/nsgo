//go:build js && wasm && nsgominiprogram

package backend

import (
	"os"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/miniprogram"
)

// run fills the mini program's page with app.
func run(root engine.App, app func(*core.Scope), o Options) {
	miniprogram.Run(rootOf(root, app), miniprogram.Options{Name: name, Version: core.App().Version, Development: core.App().Development, UpdateChannel: os.Getenv("NSGO_UPDATE_CHANNEL"), ID: id, Appearance: o.Appearance})
}
