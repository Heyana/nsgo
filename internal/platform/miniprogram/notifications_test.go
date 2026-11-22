package miniprogram

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMiniProgramNotificationsUnavailable(t *testing.T) {
	// Mini programs have no local system notification API (subscription messages
	// are server-pushed only), so ScheduledNotifications stays false and requests
	// answer via driver.NoNotification.
	var env driver.Environment
	deviceOS(&env, "ios")
	if env.ScheduledNotifications {
		t.Fatal("mini program unexpectedly reported ScheduledNotifications true")
	}
	perm := driver.NoNotification(&driver.MountItem{Kind: driver.ItemNotificationPermission, Count: 1, On: true})
	if perm.Kind != driver.EventNotification || perm.Index != driver.NotificationsUnavailable || perm.Finished {
		t.Fatalf("permission = %+v", perm)
	}
	n := driver.Notification{ID: "mp", Title: "Reminder", At: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), Repeat: driver.NotificationDaily}
	show := driver.NoNotification(&driver.MountItem{Kind: driver.ItemShowNotification, Count: 2, Notification: &n})
	if show.Kind != driver.EventNotification || show.Finished {
		t.Fatalf("show = %+v", show)
	}
}
