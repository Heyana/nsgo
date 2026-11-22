//go:build windows && nsgo_steam

package windows

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

func startWinSparkle(updates.Config, *window, func() string) {}
func stopWinSparkle()                                        {}
func answerUpdateQuit(*window, *driver.MountItem) bool       { return false }
func prepareWinSparkle(updates.Config, uintptr)              {}
func winSparkleStatus() driver.UpdaterState {
	return driver.UpdaterState{Managed: driver.UpdateManagedChannel}
}
func (w *window) setUpdatePreference(*driver.UpdatePreference) {}

func (w *window) checkForUpdates(_ bool, id uint32) {
	w.engine.Dispatch(driver.Event{Kind: driver.EventUpdate, Count: id,
		Data: &driver.Update{Err: "updates: Steam updates the app"}})
}
