package driver

import "time"

// UpdateManaged identifies who updates this installed copy, without a feed request.
type UpdateManaged uint8

const (
	UpdateManagedUnavailable UpdateManaged = iota
	UpdateManagedSelf
	UpdateManagedPage
	UpdateManagedDevelopment
	UpdateManagedChannel
	UpdateManagedStore
)

// UpdaterState is the updater's latest snapshot. NativePreferences means the platform updater owns persistence and the schedule; otherwise package updater does.
type UpdaterState struct {
	Managed                             UpdateManaged
	Store                               string
	NativePreferences                   bool
	AutomaticChecks, AutomaticDownloads bool
	Available, Notes                    string
	LastCheck                           time.Time
}

// UpdatePreference changes one preference through ItemCheckForUpdates instead of checking the feed. Nil leaves that preference alone.
type UpdatePreference struct{ AutomaticChecks, AutomaticDownloads *bool }
