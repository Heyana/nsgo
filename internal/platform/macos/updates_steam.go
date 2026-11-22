//go:build darwin && !ios && nsgo_steam

package macos

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

const sparkleEnabled = false

func startSparkle(updates.Config) {}
func sparkleStatus() driver.UpdaterState {
	return driver.UpdaterState{Managed: driver.UpdateManagedChannel}
}
func (s *surface) setUpdatePreference(*driver.UpdatePreference) {}

func (s *surface) checkForUpdates(_ bool, id uint32) {
	s.engine.Dispatch(driver.Event{Kind: driver.EventUpdate, Count: id,
		Data: &driver.Update{Err: "updates: Steam updates the app"}})
}
