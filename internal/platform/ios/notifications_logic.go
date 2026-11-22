package ios

import "github.com/nsgo-dev/nsgo/internal/driver"

type iosNotificationBackend struct {
	permission func(ask bool, answer func(int))
	show       func(n driver.Notification, answer func(bool))
	close      func(id string)
	badge      func(count uint32)
}

// canScheduleIOSNotifications reports whether local notification scheduling is
// available for idiom: tvOS's UNUserNotificationCenter supports app icon badges
// only and shows no local alerts or banners.
func canScheduleIOSNotifications(idiom driver.Idiom, centerAvailable bool) bool {
	return idiom != driver.IdiomTV && centerAvailable
}

// dispatchIOSNotification routes notification commands on iOS, iPadOS and tvOS.
// On tvOS, UNUserNotificationCenter supports badges only (alert title, body and
// sound APIs are prohibited), so permission and show requests answer with
// driver.NoNotification while badge updates still run.
func dispatchIOSNotification(tv bool, it *driver.MountItem, b iosNotificationBackend, dispatch func(driver.Event)) {
	id := it.Count
	switch it.Kind {
	case driver.ItemNotificationPermission:
		if tv || b.permission == nil {
			dispatch(driver.NoNotification(it))
			return
		}
		b.permission(it.On, func(p int) {
			dispatch(driver.Event{Kind: driver.EventNotification, Count: id, Index: p})
		})
	case driver.ItemShowNotification:
		if tv || b.show == nil || it.Notification == nil {
			dispatch(driver.NoNotification(it))
			return
		}
		b.show(*it.Notification, func(shown bool) {
			dispatch(driver.Event{Kind: driver.EventNotification, Count: id, Finished: shown})
		})
	case driver.ItemCloseNotification:
		if !tv && b.close != nil {
			b.close(it.Value)
		}
	case driver.ItemBadge:
		if b.badge != nil {
			b.badge(it.Count)
		}
	}
}
