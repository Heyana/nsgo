//go:build linux && !android && nsgo_steam

package linux

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

func configureUpdates(c updates.Config) updates.Config { return c }

func (w *window) checkForUpdates(id uint32) {
	w.engine.Dispatch(driver.Event{Kind: driver.EventUpdate, Count: id,
		Data: &driver.Update{Err: "updates: Steam updates the app"}})
}

func (w *window) installPackage(id uint32, _ string) {
	w.engine.Dispatch(driver.NoPackageInstaller(id))
}
