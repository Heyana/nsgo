package core

import (
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Permission is something the system keeps for the user to give apps, named by its key. The app checks it (Scope.PermissionStatus), asks for it with the system's prompt (Scope.RequestPermission), opens the page of the system's settings that gives it (Scope.OpenPermissionSettings) and, on desktops, guides the user there (desktop.GuidePermission).
type Permission string

const (
	// PermissionAccessibility is controlling the computer through the accessibility API, on macOS, which names it Device Control and Data Access from macOS 27.
	PermissionAccessibility Permission = driver.PermissionAccessibility
	// PermissionScreenRecording is capturing the screen and its audio, on macOS, which tells a change only after the app starts again.
	PermissionScreenRecording Permission = driver.PermissionScreenRecording
	// PermissionInputMonitoring is reading the keyboard and the mouse while other apps are in front, on macOS.
	PermissionInputMonitoring Permission = driver.PermissionInputMonitoring
	// PermissionFullDiskAccess is the files macOS protects, which it tells by reading one.
	PermissionFullDiskAccess Permission = driver.PermissionFullDiskAccess
	// PermissionAppManagement is changing other apps, on macOS, which never tells it (PermissionUnknown).
	PermissionAppManagement Permission = driver.PermissionAppManagement
	// PermissionDeveloperTools is running software that does not meet the system's security policy, on macOS, which never tells it (PermissionUnknown).
	PermissionDeveloperTools Permission = driver.PermissionDeveloperTools
	// PermissionMicrophone is recording; asking needs NSMicrophoneUsageDescription in apple.info.
	PermissionMicrophone Permission = driver.PermissionMicrophone
	// PermissionCamera is the camera; asking needs NSCameraUsageDescription.
	PermissionCamera Permission = driver.PermissionCamera
	// PermissionCalendars is the user's calendars; asking needs NSCalendarsFullAccessUsageDescription from macOS 14 and iOS 17, NSCalendarsUsageDescription before.
	PermissionCalendars Permission = driver.PermissionCalendars
	// PermissionReminders is the user's reminders; asking needs NSRemindersFullAccessUsageDescription from macOS 14 and iOS 17, NSRemindersUsageDescription before.
	PermissionReminders Permission = driver.PermissionReminders
	// PermissionBluetooth is Bluetooth devices; asking needs NSBluetoothAlwaysUsageDescription.
	PermissionBluetooth Permission = driver.PermissionBluetooth
	// PermissionMediaLibrary is Media & Apple Music: iOS tells and asks it (NSAppleMusicUsageDescription), macOS never tells it.
	PermissionMediaLibrary Permission = driver.PermissionMediaLibrary
	// PermissionLocation is the device's location while the app shows (Locate, WatchLocation): asking needs NSLocationWhenInUseUsageDescription, or NSLocationUsageDescription on macOS, and on Android ACCESS_COARSE_LOCATION, with ACCESS_FINE_LOCATION for a precise one, in android.permissions. Whether the user allowed only an approximate location shows on each fix (LocationFix.Approximate).
	PermissionLocation Permission = driver.PermissionLocation
	// PermissionBackgroundLocation is a location stream going on in the background (LocationOptions.Background): on Apple platforms PermissionLocation with UIBackgroundModes' location, and on Android also ACCESS_BACKGROUND_LOCATION where android.permissions lists it, which Android 11 and later give only on the app's page of the settings, which asking opens. Ask for it after PermissionLocation.
	PermissionBackgroundLocation Permission = driver.PermissionBackgroundLocation
	// PermissionMotion covers accelerometer, gyroscope, magnetometer and fused readings (WatchMotion). Apple requires NSMotionUsageDescription on iOS and watchOS from 27.2. Android grants it. Mobile browsers can ask for it from a user gesture.
	PermissionMotion Permission = driver.PermissionMotion
)

// Permissions returns every Permission, in the order of macOS's Privacy & Security, then the sensors'.
func Permissions() []Permission {
	return []Permission{PermissionAccessibility, PermissionScreenRecording, PermissionInputMonitoring, PermissionFullDiskAccess,
		PermissionAppManagement, PermissionDeveloperTools, PermissionMicrophone, PermissionCamera, PermissionCalendars,
		PermissionReminders, PermissionBluetooth, PermissionMediaLibrary, PermissionLocation, PermissionBackgroundLocation, PermissionMotion}
}

// PermissionStatus is whether the app has a Permission.
type PermissionStatus uint8

const (
	// PermissionNotAsked is the user not asked yet: RequestPermission shows the system's prompt. macOS never reports it for Accessibility, Screen Recording and Input Monitoring, which are PermissionDenied until granted.
	PermissionNotAsked PermissionStatus = driver.PermissionNotAsked
	// PermissionGranted is the app having it.
	PermissionGranted PermissionStatus = driver.PermissionGranted
	// PermissionDenied is the user or an administrator having refused it; only the system's settings change it.
	PermissionDenied PermissionStatus = driver.PermissionDenied
	// PermissionUnknown is a permission the system keeps but cannot tell without asking, or that the platform's backend cannot tell yet.
	PermissionUnknown PermissionStatus = driver.PermissionUnknown
	// PermissionNotGated is a permission the system does not keep from this app, which may use what it guards: on Linux outside a sandbox, and on Windows for all but the microphone and the camera.
	PermissionNotGated PermissionStatus = driver.PermissionNotGated
	// PermissionUnavailable is a permission the platform has no such thing for, such as Accessibility on iOS or the web, a key that names none, or one the current owner's Access does not grant AccessPermissions for.
	PermissionUnavailable PermissionStatus = driver.PermissionUnavailable
)

// String returns "prompt", "granted", "denied", "unknown", "not-gated" or "unavailable".
func (s PermissionStatus) String() string {
	switch s {
	case PermissionNotAsked:
		return "prompt"
	case PermissionGranted:
		return "granted"
	case PermissionDenied:
		return "denied"
	case PermissionNotGated:
		return "not-gated"
	case PermissionUnavailable:
		return "unavailable"
	}
	return "unknown"
}

// Allowed reports whether the app may use what the permission guards: PermissionGranted or PermissionNotGated.
func (s PermissionStatus) Allowed() bool { return s == PermissionGranted || s == PermissionNotGated }

// PermissionStatus answers done with whether the app has p, without asking the user.
func (cx *Scope) PermissionStatus(p Permission, done func(PermissionStatus)) {
	cx.permission(p, driver.PermissionCheck, func(ev driver.Event) { answerPermission(done, PermissionStatus(ev.Index)) }, func() {
		answerPermission(done, PermissionUnavailable)
	})
}

// RequestPermission asks the user for p with the system's prompt, where it has one and has not asked yet, and answers done with the status then. Ask as the user does something that needs it. An Apple build whose Info.plist lacks p's usage description answers PermissionUnavailable and logs the key it needs, as the system would end the app as it asks. macOS lists the app under Accessibility, Screen Recording and Input Monitoring as it asks, and answers before the user turns it on there, which desktop.GuidePermission waits for.
func (cx *Scope) RequestPermission(p Permission, done func(PermissionStatus)) {
	cx.permission(p, driver.PermissionAsk, func(ev driver.Event) { answerPermission(done, PermissionStatus(ev.Index)) }, func() {
		answerPermission(done, PermissionUnavailable)
	})
}

// OpenPermissionSettings opens the page of the system's settings that gives p, and answers done, when not nil, with whether one opened: Privacy & Security at p's pane on macOS, the app's page in Settings on iOS, and the microphone's or the camera's page of Privacy & security on Windows. Elsewhere it answers false.
func (cx *Scope) OpenPermissionSettings(p Permission, done func(opened bool)) {
	answer := func(opened bool) {
		if done != nil {
			done(opened)
		}
	}
	cx.permission(p, driver.PermissionOpenSettings, func(ev driver.Event) { answer(ev.Finished) }, func() { answer(false) })
}

func answerPermission(done func(PermissionStatus), s PermissionStatus) {
	if done != nil {
		done(s)
	}
}

// permission sends command about p and hands its answer to answered, or calls refused at once, sending nothing, for a key that names no permission or one the current owner's Access refuses.
func (cx *Scope) permission(p Permission, command driver.PermissionCommand, answered func(driver.Event), refused func()) {
	t := cx.tree()
	// A sensor's permission is the sensor service's to tell and ask, held to the sensor's capability; only its settings open as the others' do.
	if kind, ok := sensorPermissions[p]; ok && command != driver.PermissionOpenSettings {
		if t == nil || !cx.sensorPermission(kind, command == driver.PermissionAsk, func(s PermissionStatus) { answered(driver.Event{Index: int(s)}) }) {
			refused()
		}
		return
	}
	if t == nil || !p.known() || cx.denied(servicePermissions, string(p)) {
		refused()
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemPermission, Permission: &driver.PermissionRequest{Command: command, Name: string(p)}}, answered)
}

func (p Permission) known() bool { return slices.Contains(Permissions(), p) }

// PermissionGuideOptions are desktop.GuidePermission's.
type PermissionGuideOptions struct {
	// App is the path of the app the user drags into System Settings' list. Empty means this app: its bundle, or its executable outside one, as under nsgo dev, where macOS charges what the app does to the terminal that started it instead. Code under an Access must leave it empty.
	App string
	// Name is what the guide calls the app: App's name without its extension when empty.
	Name string
}

// permissionGuide is the guide showing, which the app's own surface asks for: one at a time.
type permissionGuide struct {
	tree *Tree
	// id is the guide's request, which StopGuide names.
	id uint32
	// over says the platform answered, and stopped that StopGuide was sent.
	over, stopped bool
}

// end has the platform end g, which answers g's own request then.
func (g *permissionGuide) end() {
	if g.over || g.stopped {
		return
	}
	g.stopped = true
	t := g.tree
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemPermission, Count: g.id,
		Permission: &driver.PermissionRequest{Command: driver.PermissionStopGuide}})
	t.changed = true
}

// GuidePermission is the core's part of desktop.GuidePermission, which apps use. One guide shows at a time: a new one ends the one before, and the current owner's going ends it as stop does. done runs once, with the status as it ends, under the root once the owner is gone.
func (cx *Scope) GuidePermission(p Permission, opts PermissionGuideOptions, done func(PermissionStatus)) (stop func()) {
	rt := cx.rt
	t := rt.tree
	// Code under an Access may not hold out another app for the user to give p to.
	if t == nil || !p.known() || cx.denied(servicePermissions, string(p)) ||
		opts.App != "" && accessOf(cx.current()) != nil {
		answerPermission(done, PermissionUnavailable)
		return func() {}
	}
	if old := rt.guide; old != nil {
		old.end()
	}
	g := &permissionGuide{tree: t}
	t.request(cx, driver.MountItem{Kind: driver.ItemPermission, Permission: &driver.PermissionRequest{
		Command: driver.PermissionGuide, Name: string(p), App: opts.App, AppName: opts.Name}}, func(ev driver.Event) {
		g.over = true
		if rt.guide == g {
			rt.guide = nil
		}
		answerPermission(done, PermissionStatus(ev.Index))
	})
	// request numbered the guide's item with the tree's last number.
	g.id = t.alertID
	rt.guide = g
	cx.current().addCleanup(g.end)
	return g.end
}
