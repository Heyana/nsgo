package updater

import (
	"log"
	"strconv"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// Managed identifies who updates an installed copy, without checking a feed.
type Managed = core.UpdaterManaged

const (
	ManagedUnavailable = core.UpdaterUnavailable
	ManagedSelf        = core.UpdaterSelf
	ManagedPage        = core.UpdaterPage
	ManagedDevelopment = core.UpdaterDevelopment
	ManagedChannel     = core.UpdaterChannel
	ManagedStore       = core.UpdaterStore
)

// Capability describes how this copy updates. Store names its installer when ManagedStore.
type Capability struct {
	Managed Managed
	Store   string
}

// State is the updater's latest knowledge. Reading Status in a binding tracks changes. AutomaticDownloads is supported only by Sparkle on macOS.
type State struct {
	Capability                          Capability
	AutomaticChecks, AutomaticDownloads bool
	Available, Notes                    string
	LastCheck                           time.Time
}

// CapabilityOf reports how this copy updates, synchronously and without network I/O.
func CapabilityOf(cx *core.Scope) Capability {
	s := core.UpdaterStatus(cx)
	return Capability{Managed: s.Managed, Store: s.Store}
}

const schedulePartition = "nsgo.updater"
const checkedKey = "checked"
const automaticKey = "automatic"
const lastCheckKey = "last-check"

// Status returns tracked state and preferences, without checking for updates.
func Status(cx *core.Scope) State {
	s := core.UpdaterStatus(cx)
	result := State{Capability: Capability{s.Managed, s.Store}, AutomaticChecks: s.AutomaticChecks,
		AutomaticDownloads: s.AutomaticDownloads, Available: s.Available, Notes: s.Notes, LastCheck: s.LastCheck}
	if !s.NativePreferences {
		storage := scheduleStorage(cx)
		if value, ok := storage.GetItem(automaticKey); ok {
			result.AutomaticChecks = value == "true"
		}
		if value, ok := storage.GetItem(lastCheckKey); ok {
			if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
				result.LastCheck = time.Unix(seconds, 0)
			}
		} else if value, ok := storage.GetItem(checkedKey); ok {
			// Keep displaying checks made by releases predating last-check.
			if seconds, err := strconv.ParseInt(value, 10, 64); err == nil {
				result.LastCheck = time.Unix(seconds, 0)
			}
		}
	}
	if s.Managed != ManagedSelf && s.Managed != ManagedPage {
		result.AutomaticChecks = false
		result.AutomaticDownloads = false
	}
	return result
}

// SetAutomaticChecks persists the user's choice. Native updaters own their preferences; the daily scheduler on Linux, Android and iOS uses its own partition.
func SetAutomaticChecks(cx *core.Scope, on bool) {
	s := core.UpdaterStatus(cx)
	if s.Managed != ManagedSelf && s.Managed != ManagedPage {
		return
	}
	if s.NativePreferences {
		core.SetUpdaterAutomaticChecks(cx, on)
		return
	}
	if err := scheduleStorage(cx).SetItem(automaticKey, strconv.FormatBool(on)); err != nil {
		log.Printf("updater: %v", err)
	}
}

// SetAutomaticDownloads changes Sparkle's persisted choice on macOS. It is a no-op on platforms with no automatic-download setting.
func SetAutomaticDownloads(cx *core.Scope, on bool) {
	if cx.Env().OS() == core.OSMacOS && core.UpdaterStatus(cx).NativePreferences {
		core.SetUpdaterAutomaticDownloads(cx, on)
	}
}

// scheduleStorage gets the private partition without moving a caller's bindings under a new owner: GetItem below this call still tracks that caller.
func scheduleStorage(cx *core.Scope) core.Storage {
	part := cx.Child()
	var storage core.Storage
	part.Run(func() { part.ProvideStorage(schedulePartition, 0); storage = part.Storage() })
	part.Dispose()
	return storage
}
