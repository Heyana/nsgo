package watchos

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWatchNotificationDispatch(t *testing.T) {
	var events []driver.Event
	var asked []bool
	var shown []driver.Notification
	var closed []string

	backend := watchNotificationBackend{
		permission: func(ask bool, answer func(int)) {
			asked = append(asked, ask)
			answer(driver.NotificationsGranted)
		},
		show: func(n driver.Notification, answer func(bool)) {
			shown = append(shown, n)
			answer(true)
		},
		close: func(id string) {
			closed = append(closed, id)
		},
	}
	dispatch := func(ev driver.Event) { events = append(events, ev) }

	dispatchWatchNotification(&driver.MountItem{Kind: driver.ItemNotificationPermission, Count: 1, On: true}, backend, dispatch)
	at := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)
	n := driver.Notification{ID: "stand", Title: "Stand up", At: at, Repeat: driver.NotificationDaily}
	dispatchWatchNotification(&driver.MountItem{Kind: driver.ItemShowNotification, Count: 2, Notification: &n}, backend, dispatch)
	dispatchWatchNotification(&driver.MountItem{Kind: driver.ItemCloseNotification, Value: "stand"}, backend, dispatch)
	dispatchWatchNotification(&driver.MountItem{Kind: driver.ItemCloseNotification}, backend, dispatch)

	if len(asked) != 1 || !asked[0] {
		t.Fatalf("asked = %+v", asked)
	}
	if len(shown) != 1 || shown[0].ID != "stand" || !shown[0].At.Equal(at) || shown[0].Repeat != driver.NotificationDaily {
		t.Fatalf("shown = %+v", shown)
	}
	if len(closed) != 2 || closed[0] != "stand" || closed[1] != "" {
		t.Fatalf("closed = %+v", closed)
	}
	if len(events) != 2 || events[0].Index != driver.NotificationsGranted || !events[1].Finished {
		t.Fatalf("events = %+v", events)
	}

	// Nil backend falls back to driver.NoNotification.
	events = nil
	dispatchWatchNotification(&driver.MountItem{Kind: driver.ItemNotificationPermission, Count: 3}, watchNotificationBackend{}, dispatch)
	dispatchWatchNotification(&driver.MountItem{Kind: driver.ItemShowNotification, Count: 4, Notification: &n}, watchNotificationBackend{}, dispatch)
	if len(events) != 2 || events[0].Index != driver.NotificationsUnavailable || events[1].Finished {
		t.Fatalf("nil backend events = %+v", events)
	}
}
