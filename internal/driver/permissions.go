package driver

import "strings"

// The permissions an ItemPermission names (PermissionRequest.Name), as core.Permission's values.
const (
	PermissionAccessibility   = "accessibility"
	PermissionScreenRecording = "screen-recording"
	PermissionInputMonitoring = "input-monitoring"
	PermissionFullDiskAccess  = "full-disk-access"
	PermissionAppManagement   = "app-management"
	PermissionDeveloperTools  = "developer-tools"
	PermissionAutomation      = "automation"
	PermissionMicrophone      = "microphone"
	PermissionCamera          = "camera"
	PermissionContacts        = "contacts"
	PermissionCalendars       = "calendars"
	PermissionReminders       = "reminders"
	PermissionPhotos          = "photos"
	PermissionBluetooth       = "bluetooth"
	PermissionMediaLibrary    = "media-library"
	// The sensors' permissions, which core asks the sensor service for (ItemSensor) rather than sending ItemPermission, except to open their settings or guide.
	PermissionLocation            = "location"
	PermissionBackgroundLocation  = "background-location"
	PermissionMotion              = "motion"
	PermissionActivityRecognition = "activity-recognition"
	// PermissionNotifications is the notification service's permission, which core asks through ItemNotificationPermission rather than ItemPermission, except to open its settings or guide.
	PermissionNotifications = "notifications"
)

// AutomationTarget splits a macOS Automation permission key targeting a specific app ("automation:<bundleID>").
func AutomationTarget(name string) (bundleID string, ok bool) {
	rest, has := strings.CutPrefix(name, PermissionAutomation+":")
	if !has || rest == "" {
		return "", false
	}
	for i := range len(rest) {
		if rest[i] <= ' ' || rest[i] >= 0x7f {
			return "", false
		}
	}
	return rest, true
}

// The statuses EventPermission answers with (Index), as core.PermissionStatus.
const (
	PermissionNotAsked = iota
	PermissionGranted
	PermissionDenied
	PermissionUnknown
	PermissionNotGated
	PermissionUnavailable
	PermissionLimited
)

// PermissionCommand is what an ItemPermission asks.
type PermissionCommand uint8

const (
	// PermissionCheck answers the status without asking the user.
	PermissionCheck PermissionCommand = iota
	// PermissionAsk shows the system's prompt where it has one and has not asked yet, then answers the status.
	PermissionAsk
	// PermissionOpenSettings opens the page of the system's settings that gives the permission; EventPermission's Finished says whether one opened.
	PermissionOpenSettings
	// PermissionGuide opens the settings and guides the user there; it is answered once, as the guide ends, with the status then.
	PermissionGuide
	// PermissionStopGuide ends the guide whose request was Count, which still answers its own request; it is not answered itself.
	PermissionStopGuide
)

// PermissionRequest is an ItemPermission's.
type PermissionRequest struct {
	Command PermissionCommand
	// Name is the permission's key, one of the Permission* keys; "" for PermissionStopGuide.
	Name string
	// App is the path of the app PermissionGuide holds out to drag, and AppName what it calls it; "" for the defaults.
	App, AppName string
}

// AnswerPermission answers ItemPermission id where nothing asks, opens settings or guides: a check, a request and a guide hear status of the permission at once, opening the settings that none opened, and a stop nothing.
func AnswerPermission(id uint32, r PermissionRequest, status func(name string) int, dispatch func(Event)) {
	switch r.Command {
	case PermissionStopGuide:
	case PermissionOpenSettings:
		dispatch(Event{Kind: EventPermission, Count: id})
	default:
		dispatch(Event{Kind: EventPermission, Count: id, Index: status(r.Name)})
	}
}

// PhonePermission is whether a phone's or a watch's system keeps the permission name, PermissionUnknown for one it keeps, for the backends that cannot tell it yet: the ones only Macs keep are PermissionUnavailable.
func PhonePermission(name string) int {
	switch name {
	case PermissionMicrophone, PermissionCamera, PermissionContacts, PermissionCalendars, PermissionReminders, PermissionPhotos,
		PermissionBluetooth, PermissionMediaLibrary, PermissionLocation, PermissionBackgroundLocation, PermissionMotion, PermissionActivityRecognition, PermissionNotifications:
		return PermissionUnknown
	}
	return PermissionUnavailable
}
