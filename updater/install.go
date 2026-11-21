package updater

import "github.com/nsgo-dev/nsgo/core"

// Install hands a verified update package to the platform's installer.
// Linux reports when its package manager exits. Android reports when its
// system installer opens. The callback runs on the app's graph goroutine.
func Install(cx *core.Scope, path string, done func(error)) {
	core.InstallUpdate(cx, path, done)
}
