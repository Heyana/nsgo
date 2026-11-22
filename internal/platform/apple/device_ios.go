//go:build ios

package apple

import (
	"os"
	"sync"
	"syscall"
)

// ModelIdentifier returns iOS's, tvOS's or watchOS's hardware identifier. A
// simulator's hw.machine names its Mac's architecture instead, so use
// the simulated device's identifier there. An unavailable model is empty.
func ModelIdentifier() string { return modelIdentifier() }

var modelIdentifier = sync.OnceValue(func() string {
	if model := os.Getenv("SIMULATOR_MODEL_IDENTIFIER"); model != "" {
		return model
	}
	model, _ := syscall.Sysctl("hw.machine")
	if model == "arm64" || model == "x86_64" {
		return ""
	}
	return model
})
