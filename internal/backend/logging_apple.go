//go:build darwin

package backend

import (
	"github.com/nsgo-dev/nsgo/internal/dev"
	"github.com/nsgo-dev/nsgo/internal/platform/applelog"
)

// A variable initializer runs before flags.go's init, so malformed launch
// flags reach Console too, before main or a view starts.
var _ = applelog.Install(dev.Build)
