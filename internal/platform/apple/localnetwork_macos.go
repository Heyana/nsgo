//go:build darwin && !ios

package apple

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/nsgo-dev/nsgo/internal/platform/privacy"
)

// macLocalNetworkGated is LocalNetworkGated's answer, which cannot change while the app runs.
var macLocalNetworkGated = sync.OnceValue(func() bool {
	if OSVersion().Major < 15 {
		return false
	}
	// macOS lets through what another program, such as a terminal, starts and answers for (TN3179): only an app that answers for itself is kept.
	responsible, ok := privacy.ResponsibleExecutable()
	if !ok {
		return true
	}
	self, err := os.Executable()
	if err != nil {
		return true
	}
	return sameFile(responsible, self)
})

func sameFile(a, b string) bool {
	if a == b {
		return true
	}
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ra == rb
}

// LocalNetworkGated says the system keeps the local network from the app: macOS does from 15, for apps that answer for themselves.
func LocalNetworkGated() bool { return macLocalNetworkGated() }

// appActive says the app is active; a Mac's prompt leaves it so.
func appActive() bool { return true }
