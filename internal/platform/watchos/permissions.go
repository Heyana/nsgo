package watchos

import "github.com/nsgo-dev/nsgo/internal/driver"

// Host permission kinds carried by opPermission (Host.swift's Permissions).
const (
	permKindMicrophone byte = 1
	permKindCalendars  byte = 2
	permKindReminders  byte = 3
	permKindBluetooth  byte = 4
)

// watchPermissions routes ItemPermission requests on watchOS between the
// SwiftUI host (microphone, calendars, reminders and Bluetooth), Apple's
// UNUserNotificationCenter (notifications), and static availability answers.
// Its methods run on the host's main thread.
type watchPermissions struct {
	pending map[uint32]bool
}

// hostPermissionKind returns the wire kind for permissions queried and
// requested by the SwiftUI host.
func hostPermissionKind(name string) (byte, bool) {
	switch name {
	case driver.PermissionMicrophone:
		return permKindMicrophone, true
	case driver.PermissionCalendars:
		return permKindCalendars, true
	case driver.PermissionReminders:
		return permKindReminders, true
	case driver.PermissionBluetooth:
		return permKindBluetooth, true
	}
	return 0, false
}

// handle applies one ItemPermission request numbered id.
func (p *watchPermissions) handle(id uint32, r driver.PermissionRequest, w *writer, notifyPerm func(ask bool, answer func(int)), dispatch func(driver.Event)) {
	switch r.Command {
	case driver.PermissionStopGuide:
		return
	case driver.PermissionOpenSettings:
		// watchOS has no app-level Settings URL scheme.
		dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Finished: false})
		return
	case driver.PermissionCheck, driver.PermissionAsk, driver.PermissionGuide:
		ask := r.Command == driver.PermissionAsk
		if r.Name == driver.PermissionNotifications {
			if notifyPerm == nil {
				dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: driver.PermissionUnavailable})
				return
			}
			notifyPerm(ask, func(np int) {
				dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: driver.NotificationPermissionStatus(np)})
			})
			return
		}
		if kind, ok := hostPermissionKind(r.Name); ok {
			if p.pending == nil {
				p.pending = map[uint32]bool{}
			}
			p.pending[id] = true
			w.op(opPermission)
			w.u32(id)
			w.byte(kind)
			w.flag(ask)
			return
		}
		dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: watchPermission(r.Name)})
	}
}

// finished completes a pending host permission request with the driver.Permission*
// status code returned by Host.swift. Unknown or duplicate completions are ignored.
func (p *watchPermissions) finished(id uint32, status uint8, dispatch func(driver.Event)) {
	if !p.pending[id] {
		return
	}
	delete(p.pending, id)
	st := int(status)
	if st < driver.PermissionNotAsked || st > driver.PermissionLimited {
		st = driver.PermissionUnknown
	}
	dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: st})
}

// watchPermission returns the static status of permissions not queried by the
// SwiftUI host or notification center: a watch has no camera, photo library,
// media library or Mac-only TCC panes.
func watchPermission(name string) int {
	switch name {
	case driver.PermissionCamera, driver.PermissionPhotos, driver.PermissionMediaLibrary:
		return driver.PermissionUnavailable
	}
	return driver.PhonePermission(name)
}
