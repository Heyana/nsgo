//go:build nsgotui && ((darwin && !ios) || (linux && !android) || windows)

package backend

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/tui"
)

func run(root engine.App, app func(*core.Scope), o Options) {
	if err := tui.Run(rootOf(root, app), tui.Options{ID: id, Title: name, Appearance: o.Appearance}); err != nil {
		panic(err)
	}
}

func captureNativeCrashes(func(string, string)) func() { return func() {} }
