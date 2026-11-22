//go:build js && wasm && !nsgominiprogram && !nsgoharmony

package backend

import (
	"os"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/web"
)

// The web runs as the OS --nsgo-os names.
func init() { simulates = true }

// run fills the page with app, or the element the page names in the app's environment (NSGO_MOUNT, a CSS selector), so a page shows several apps, each a Go program of its own; NSGO_HOST names the bridge to the page's host that nsgo.js, the loader, made.
func run(root engine.App, app func(*core.Scope), o Options) {
	web.Run(rootOf(root, app), web.Options{Name: name, UpdateConfig: os.Getenv("NSGO_WEB_CONFIG"), ID: id, Components: o.Components, Fonts: o.Fonts, Appearance: o.Appearance,
		Mount: os.Getenv("NSGO_MOUNT"), Host: os.Getenv("NSGO_HOST"), OS: o.Simulate})
}
