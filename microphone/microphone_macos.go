//go:build darwin && !ios

package microphone

import (
	"fmt"

	"github.com/nsgo-dev/nsgo/internal/platform/privacy"
)

// enterSession does nothing on macOS, whose recordings need no session
// and end only when stopped.
func enterSession(func(bool)) (leave func(), err error) { return func() {}, nil }

// checkUsageDescription stands in front of the permission prompt: macOS asks on behalf of the responsible process, which an app started from another app is not, and aborts this one when that process declares no usage description. Where the system cannot name that process, it lets the prompt go ahead.
func checkUsageDescription() error {
	if executable, ok := privacy.ResponsibleExecutable(); ok && !privacy.DeclaresUsage("NSMicrophoneUsageDescription") {
		return fmt.Errorf("%w: %s", ErrNoUsageDescription, executable)
	}
	return nil
}
