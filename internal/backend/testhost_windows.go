//go:build windows && nsgotest && !nsgotui && !nsgoandroidhost

package backend

import (
	"github.com/nsgo-dev/nsgo/internal/platform/windows"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func init() { testhost.Register(windows.TestHost()) }
