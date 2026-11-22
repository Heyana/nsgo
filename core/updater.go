package core

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// UpdaterManaged is the core's part of updater.Managed, which apps use.
type UpdaterManaged uint8

const (
	// UpdaterUnavailable is the core's part of updater.ManagedUnavailable.
	UpdaterUnavailable UpdaterManaged = UpdaterManaged(driver.UpdateManagedUnavailable)
	// UpdaterSelf is the core's part of updater.ManagedSelf.
	UpdaterSelf UpdaterManaged = UpdaterManaged(driver.UpdateManagedSelf)
	// UpdaterPage is the core's part of updater.ManagedPage.
	UpdaterPage UpdaterManaged = UpdaterManaged(driver.UpdateManagedPage)
	// UpdaterDevelopment is the core's part of updater.ManagedDevelopment.
	UpdaterDevelopment UpdaterManaged = UpdaterManaged(driver.UpdateManagedDevelopment)
	// UpdaterChannel is the core's part of updater.ManagedChannel.
	UpdaterChannel UpdaterManaged = UpdaterManaged(driver.UpdateManagedChannel)
	// UpdaterStore is the core's part of updater.ManagedStore.
	UpdaterStore UpdaterManaged = UpdaterManaged(driver.UpdateManagedStore)
)

// UpdaterState is the core's part of updater.State. NativePreferences identifies a platform updater that owns its preferences and schedule. Phase, Loaded and Total follow an in-app update's download and install on Linux and Android (InstallUpdateInBackground), which the core tracks itself.
type UpdaterState struct {
	Managed                             UpdaterManaged
	Store                               string
	NativePreferences                   bool
	AutomaticChecks, AutomaticDownloads bool
	Available, Notes                    string
	LastCheck                           time.Time
	Phase                               UpdatePhase
	Loaded, Total                       int64
}

// Fraction is the download's progress from 0 to 1, or 0 when Total is unknown or no bytes have arrived yet.
func (s UpdaterState) Fraction() float64 {
	if s.Total <= 0 || s.Loaded <= 0 {
		return 0
	}
	if s.Loaded >= s.Total {
		return 1
	}
	return float64(s.Loaded) / float64(s.Total)
}

// UpdaterStatus is the core's tracked read for updater.Status.
func UpdaterStatus(cx *Scope) UpdaterState { return cx.rt.updater.Get() }

func updaterState(s driver.UpdaterState) UpdaterState {
	return UpdaterState{Managed: UpdaterManaged(s.Managed), Store: s.Store, NativePreferences: s.NativePreferences,
		AutomaticChecks: s.AutomaticChecks, AutomaticDownloads: s.AutomaticDownloads, Available: s.Available, Notes: s.Notes, LastCheck: s.LastCheck}
}

// SetUpdaterAutomaticChecks is the core's part of updater.SetAutomaticChecks.
func SetUpdaterAutomaticChecks(cx *Scope, on bool) {
	setUpdaterPreference(cx, &driver.UpdatePreference{AutomaticChecks: &on})
}

// SetUpdaterAutomaticDownloads is the core's part of updater.SetAutomaticDownloads.
func SetUpdaterAutomaticDownloads(cx *Scope, on bool) {
	setUpdaterPreference(cx, &driver.UpdatePreference{AutomaticDownloads: &on})
}

func setUpdaterPreference(cx *Scope, preference *driver.UpdatePreference) {
	if t := cx.tree(); t != nil {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemCheckForUpdates, UpdatePreference: preference})
		t.changed = true
	}
}
