package nsgotest_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestPermissionNotificationsModelSync(t *testing.T) {
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{}
	})

	// RequestPermission updates both Permissions and NotificationPermission so ShowNotification works.
	var status core.PermissionStatus
	scope.RequestPermission(core.PermissionNotifications, func(s core.PermissionStatus) { status = s })
	app.Settle()
	if status != core.PermissionGranted || app.NotificationPermission != core.NotificationsGranted || app.Permissions[core.PermissionNotifications] != core.PermissionGranted {
		t.Fatalf("after RequestPermission: status=%v notif=%v map=%v", status, app.NotificationPermission, app.Permissions[core.PermissionNotifications])
	}
	var shown bool
	if err := scope.ShowNotification(core.Notification{ID: "n1", Title: "Hello"}, func(ok bool) { shown = ok }); err != nil {
		t.Fatal(err)
	}
	app.Settle()
	if !shown || len(app.Notifications) != 1 {
		t.Fatalf("ShowNotification after PermissionNotifications grant: shown=%v count=%d", shown, len(app.Notifications))
	}
}

func TestPermissionNotificationsRefusalAndGuideGrant(t *testing.T) {
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{}
	})

	app.NotificationAnswer = core.NotificationsDenied
	var status core.PermissionStatus
	scope.RequestPermission(core.PermissionNotifications, func(s core.PermissionStatus) { status = s })
	app.Settle()
	if status != core.PermissionDenied || app.Permissions[core.PermissionNotifications] != core.PermissionDenied {
		t.Fatalf("denied via NotificationAnswer: status=%v map=%v", status, app.Permissions[core.PermissionNotifications])
	}

	var guided core.PermissionStatus
	scope.GuidePermission(core.PermissionNotifications, core.PermissionGuideOptions{}, func(s core.PermissionStatus) { guided = s })
	app.Settle()
	if app.Guide != core.PermissionNotifications {
		t.Fatalf("guide = %q, want notifications", app.Guide)
	}
	app.GrantPermission(core.PermissionNotifications)
	if guided != core.PermissionGranted || app.NotificationPermission != core.NotificationsGranted {
		t.Fatalf("after GrantPermission: guided=%v notif=%v", guided, app.NotificationPermission)
	}
}
