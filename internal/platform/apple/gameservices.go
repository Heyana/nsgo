//go:build darwin

package apple

import (
	"sync"

	"github.com/ebitengine/purego"
)

const gameKitPath = "/System/Library/Frameworks/GameKit.framework/GameKit"

// GameServicesAvailable is Environment.CanUseGameServices: whether GameKit
// loads on this macOS, iOS or tvOS device and exposes GKLocalPlayer.
var GameServicesAvailable = sync.OnceValue(func() bool {
	if _, err := purego.Dlopen(gameKitPath, purego.RTLD_LAZY|purego.RTLD_GLOBAL); err != nil {
		return false
	}
	return Class("GKLocalPlayer") != 0
})
