package ios

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestIOSAndTVOSNotificationDispatch(t *testing.T) {
	if canScheduleIOSNotifications(driver.IdiomTV, true) {
		t.Fatal("tvOS unexpectedly reported scheduled notifications available")
	}
	if !canScheduleIOSNotifications(driver.IdiomPhone, true) {
		t.Fatal("iPhone with center should report scheduled notifications available")
	}
	if canScheduleIOSNotifications(driver.IdiomPhone, false) {
		t.Fatal("iPhone without center should report scheduled notifications unavailable")
	}

	var events []driver.Event
	var showed bool
	var closed bool
	var badge uint32
	backend := iosNotificationBackend{
		permission: func(_ bool, answer func(int)) { answer(driver.NotificationsGranted) },
		show: func(_ driver.Notification, answer func(bool)) {
			showed = true
			answer(true)
		},
		close: func(string) { closed = true },
		badge: func(c uint32) { badge = c },
	}
	dispatch := func(ev driver.Event) { events = append(events, ev) }

	// On tvOS: permission is unavailable, show answers false without calling UNMutableNotificationContent setters, close is a no-op, badge still works.
	n := driver.Notification{ID: "tv", Title: "TV", At: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)}
	dispatchIOSNotification(true, &driver.MountItem{Kind: driver.ItemNotificationPermission, Count: 1, On: true}, backend, dispatch)
	dispatchIOSNotification(true, &driver.MountItem{Kind: driver.ItemShowNotification, Count: 2, Notification: &n}, backend, dispatch)
	dispatchIOSNotification(true, &driver.MountItem{Kind: driver.ItemCloseNotification, Value: "tv"}, backend, dispatch)
	dispatchIOSNotification(true, &driver.MountItem{Kind: driver.ItemBadge, Count: 4}, backend, dispatch)

	if showed || closed || badge != 4 {
		t.Fatalf("tvOS side effects: showed=%v closed=%v badge=%d", showed, closed, badge)
	}
	if len(events) != 2 || events[0].Index != driver.NotificationsUnavailable || events[1].Finished {
		t.Fatalf("tvOS events = %+v", events)
	}

	// On iPhone/iPad: all commands reach UNUserNotificationCenter.
	events = nil
	dispatchIOSNotification(false, &driver.MountItem{Kind: driver.ItemNotificationPermission, Count: 3, On: false}, backend, dispatch)
	dispatchIOSNotification(false, &driver.MountItem{Kind: driver.ItemShowNotification, Count: 4, Notification: &n}, backend, dispatch)
	dispatchIOSNotification(false, &driver.MountItem{Kind: driver.ItemCloseNotification, Value: "tv"}, backend, dispatch)

	if !showed || !closed {
		t.Fatalf("iOS side effects: showed=%v closed=%v", showed, closed)
	}
	if len(events) != 2 || events[0].Index != driver.NotificationsGranted || !events[1].Finished {
		t.Fatalf("iOS events = %+v", events)
	}
}
