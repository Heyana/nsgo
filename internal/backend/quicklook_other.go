//go:build !(darwin && !ios && !nsgotui && !nsgoandroidhost)

package backend

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// RunQuickLook is a no-op outside macOS.
func RunQuickLook(func(cx *core.Scope, path string) core.Node, Options, engine.HostOptions) {}
