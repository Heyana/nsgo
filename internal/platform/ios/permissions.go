//go:build ios

package ios

import (
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/privacy"
)

func notificationPermissionStatus(np int) int {
	switch np {
	case driver.NotificationsNotAsked:
		return driver.PermissionNotAsked
	case driver.NotificationsGranted:
		return driver.PermissionGranted
	case driver.NotificationsDenied:
		return driver.PermissionDenied
	}
	return driver.PermissionUnavailable
}

// permission carries out an ItemPermission numbered id: privacy tells and asks, and the settings are the app's page in Settings (or its Notifications page from iOS 16). Nothing guides on a phone or a TV, so a guide answers the status at once.
func (s *surface) permission(id uint32, r driver.PermissionRequest) {
	e := s.engine
	answer := func(status int) { e.Dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Index: status}) }
	switch r.Command {
	case driver.PermissionCheck, driver.PermissionGuide:
		if r.Name == driver.PermissionNotifications {
			apple.NotificationPermission(false, func(np int) { answer(notificationPermissionStatus(np)) })
			return
		}
		answer(privacy.Status(r.Name))
	case driver.PermissionAsk:
		if r.Name == driver.PermissionNotifications {
			apple.NotificationPermission(true, func(np int) { answer(notificationPermissionStatus(np)) })
			return
		}
		privacy.Request(r.Name, answer)
	case driver.PermissionOpenSettings:
		opened := func(ok bool) { e.Dispatch(driver.Event{Kind: driver.EventPermission, Count: id, Finished: ok}) }
		settings := ""
		if r.Name == driver.PermissionNotifications {
			if !apple.NotificationsAvailable() {
				opened(false)
				return
			}
			settings = apple.GoString(apple.Global(uikitPath, "UIApplicationOpenNotificationSettingsURLString"))
		} else if r.Name != driver.PermissionActivityRecognition && privacy.Status(r.Name) == driver.PermissionUnavailable {
			opened(false)
			return
		}
		if settings == "" {
			settings = apple.GoString(apple.Global(uikitPath, "UIApplicationOpenSettingsURLString"))
		}
		if settings == "" {
			opened(false)
			return
		}
		openSystemURL(settings, opened)
	}
}
