//go:build linux && !android && nsgotest && !nsgotui && !nsgoandroidhost

package backend

import (
	"github.com/nsgo-dev/nsgo/internal/platform/linux"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func init() { testhost.Register(linux.TestHost()) }
