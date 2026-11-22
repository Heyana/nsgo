package watchos

import "github.com/nsgo-dev/nsgo/internal/driver"

type watchNotificationBackend struct {
	permission func(ask bool, answer func(int))
	show       func(n driver.Notification, answer func(bool))
	close      func(id string)
}

// dispatchWatchNotification routes notification commands on watchOS to Apple's UNUserNotificationCenter (which supports immediate and scheduled once, daily and weekly local notifications). Badge items remain no-ops because watchOS app icons have no badge.
func dispatchWatchNotification(it *driver.MountItem, b watchNotificationBackend, dispatch func(driver.Event)) {
	id := it.Count
	switch it.Kind {
	case driver.ItemNotificationPermission:
		if b.permission == nil {
			dispatch(driver.NoNotification(it))
			return
		}
		b.permission(it.On, func(p int) {
			dispatch(driver.Event{Kind: driver.EventNotification, Count: id, Index: p})
		})
	case driver.ItemShowNotification:
		if b.show == nil || it.Notification == nil {
			dispatch(driver.NoNotification(it))
			return
		}
		b.show(*it.Notification, func(shown bool) {
			dispatch(driver.Event{Kind: driver.EventNotification, Count: id, Finished: shown})
		})
	case driver.ItemCloseNotification:
		if b.close != nil {
			b.close(it.Value)
		}
	}
}
