package core

import (
	"errors"
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// NotificationPermission is whether the app may show notifications.
type NotificationPermission uint8

const (
	// NotificationsNotAsked is the user not asked yet:
	// RequestNotificationPermission shows the system's prompt.
	NotificationsNotAsked NotificationPermission = iota
	// NotificationsGranted lets the app show them.
	NotificationsGranted
	// NotificationsDenied keeps the app from showing them; only the
	// system's settings change it.
	NotificationsDenied
	// NotificationsUnavailable is a platform or a build that shows none,
	// such as a macOS app run outside its bundle, or code its Access does
	// not grant AccessNotifications.
	NotificationsUnavailable
)

// String returns NSH's name of p: "default", "granted", "denied" or
// "unavailable".
func (p NotificationPermission) String() string {
	switch p {
	case NotificationsGranted:
		return "granted"
	case NotificationsDenied:
		return "denied"
	case NotificationsUnavailable:
		return "unavailable"
	}
	return "default"
}

// Notification is a system notification (ShowNotification).
type Notification struct {
	// ID is the app's name for it: showing another with the same ID replaces it, shown or still to show, CloseNotification removes it and a press reports it. Keep in it what the press opens ("chat:42"), as a press can start the app anew.
	ID string
	// Title and Body are its text.
	Title, Body string
	// Silent shows it without the system's sound or vibration.
	Silent bool
	// At, when later than now (Scope.WallTime), schedules it to show then, where Env.CanScheduleNotifications says the platform does (whether the app runs or not on macOS, iOS, watchOS, Android and Windows, and while the app or page runs on Linux and the web); zero or earlier shows it at once.
	At time.Time
	// Repeat shows it again and again, at At's time of day, its clock reading in At's location, which the device keeps to in its own time zone, and for NotificationWeekly on At's weekday: first at the next such time after now, whatever At's date. It needs At.
	Repeat NotificationRepeat
}

// NotificationRepeat is how often a scheduled notification shows (Notification.Repeat).
type NotificationRepeat uint8

const (
	// NotificationOnce shows it once.
	NotificationOnce NotificationRepeat = iota
	// NotificationDaily shows it every day at At's time of day.
	NotificationDaily
	// NotificationWeekly shows it every week on At's weekday at its time of day.
	NotificationWeekly
)

// ErrNotification is ShowNotification's error for a notification without an ID or a title, or with a Repeat it does not know or no At to repeat.
var ErrNotification = errors.New("nsgo: a notification needs an ID and a title, and a repeated one a time")

// NotificationPermission answers done with whether the app may show notifications, without asking the user. On macOS checking uses the notification center as asking does, so from then on the app's Dock badge shows only while the user allows its badges.
func (cx *Scope) NotificationPermission(done func(NotificationPermission)) {
	cx.notificationPermission(false, done)
}

// RequestNotificationPermission asks the user to let the app show notifications, with the system's prompt the first time, and answers done with the permission. Ask as the user does something that needs them: browsers refuse a prompt the user did not cause, and iOS and Android ask only once. An iOS app's badge needs it too, as does a macOS app's once it checks or asks for the permission or shows a notification.
func (cx *Scope) RequestNotificationPermission(done func(NotificationPermission)) {
	cx.notificationPermission(true, done)
}

func notificationStatus(p NotificationPermission) PermissionStatus {
	switch p {
	case NotificationsNotAsked:
		return PermissionNotAsked
	case NotificationsGranted:
		return PermissionGranted
	case NotificationsDenied:
		return PermissionDenied
	}
	return PermissionUnavailable
}

func (cx *Scope) notificationPermission(ask bool, done func(NotificationPermission)) {
	t := cx.tree()
	if t == nil || cx.denied(serviceNotifications, "") {
		if done != nil {
			done(NotificationsUnavailable)
		}
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemNotificationPermission, On: ask}, func(ev driver.Event) {
		if done != nil {
			done(NotificationPermission(ev.Index))
		}
	})
}

// ShowNotification shows n, also while the app is in front, or schedules it for its At or Repeat, and answers done, when not nil, with whether the system took it: false without the permission, and for a scheduled one where Env.CanScheduleNotifications is false. It returns ErrNotification, calling no done, for n without an ID or a title, or with a Repeat but no At.
func (cx *Scope) ShowNotification(n Notification, done func(shown bool)) error {
	if n.ID == "" || n.Title == "" || n.Repeat > NotificationWeekly || n.Repeat != NotificationOnce && n.At.IsZero() {
		return ErrNotification
	}
	answer := func(shown bool) {
		if done != nil {
			done(shown)
		}
	}
	t := cx.tree()
	if t == nil || cx.denied(serviceNotifications, "") {
		answer(false)
		return nil
	}
	dn := driver.Notification{ID: n.ID, Title: n.Title, Body: n.Body, Silent: n.Silent, Repeat: driver.NotificationRepeat(n.Repeat)}
	if now := cx.WallTime(); n.Repeat != NotificationOnce {
		dn.At = nextRepeat(n.At, n.Repeat, now)
	} else if n.At.After(now) {
		dn.At = n.At
	}
	if !dn.At.IsZero() && !Untracked(cx, cx.Env().CanScheduleNotifications) {
		answer(false)
		return nil
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemShowNotification, Notification: &dn},
		func(ev driver.Event) { answer(ev.Finished) })
	return nil
}

// nextRepeat is the first time after now that a notification repeated as r shows: at's clock reading in its location, on at's weekday for a weekly one.
func nextRepeat(at time.Time, r NotificationRepeat, now time.Time) time.Time {
	return driver.NextNotificationRepeat(at, driver.NotificationRepeat(r), now.In(at.Location()))
}

// CloseNotification removes the notification called id, if it still shows, or keeps it from showing if it is scheduled.
func (cx *Scope) CloseNotification(id string) {
	if t := cx.tree(); t != nil && id != "" && !cx.denied(serviceNotifications, "") {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemCloseNotification, Value: id})
		t.changed = true
	}
}

// CloseNotifications removes every notification ShowNotification showed or scheduled, also those earlier runs left where the system keeps them: on macOS, iOS, watchOS, Windows and Android. Linux and the web reach only those this run showed or scheduled. It leaves the app's other notifications alone, such as Android's Live Updates and its foreground services', and the badge.
func (cx *Scope) CloseNotifications() {
	if t := cx.tree(); t != nil && !cx.denied(serviceNotifications, "") {
		// An empty Value, which CloseNotification never sends, means all.
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemCloseNotification})
		t.changed = true
	}
}

// SetBadge shows count on the app's icon, and 0 or less clears it: the Dock's on macOS, which needs the user to allow the app's badges once it checks or asks for the notification permission or shows a notification, the Home Screen's on iOS, which needs the notification permission, the taskbar button's on Windows, the launcher's on Linux where it follows Unity's LauncherEntry API, such as Ubuntu's dock, Plasma and Dash to Dock, and an installed web app's. Android has no count to set: launchers mark an app whose notifications show, and count goes with those shown after it. Elsewhere nothing happens.
func (cx *Scope) SetBadge(count int) {
	t := cx.tree()
	if t == nil || cx.denied(serviceNotifications, "") {
		return
	}
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemBadge, Count: uint32(min(max(count, 0), 1<<31-1))})
	t.changed = true
}

type notificationPressHandler struct {
	owner *Scope
	fn    func(id string)
}

// OnNotificationPress runs fn, while the current owner lives, with the ID
// of a notification the user pressed, which brings the app to the front.
// Presses that came before any handler, as the one that started the app,
// go to the first one registered.
func (cx *Scope) OnNotificationPress(fn func(id string)) {
	if cx.denied(serviceNotifications, "") {
		return
	}
	rt := cx.rt
	h := &notificationPressHandler{owner: cx.current(), fn: fn}
	rt.notificationPresses = append(rt.notificationPresses, h)
	h.owner.addCleanup(func() {
		rt.notificationPresses = slices.DeleteFunc(rt.notificationPresses, func(o *notificationPressHandler) bool { return o == h })
	})
	if ids := rt.pressesPending; len(ids) > 0 {
		rt.pressesPending = nil
		// Not while the caller builds its view: the handler may well
		// change what it shows.
		h.owner.Post(func() {
			for _, id := range ids {
				h.fn(id)
			}
		})
	}
}

// notificationPressed hands the press of notification id to the
// handlers, or keeps it for the first one.
func (rt *runtime) notificationPressed(id string) {
	if len(rt.notificationPresses) == 0 {
		rt.pressesPending = append(rt.pressesPending, id)
		return
	}
	for _, h := range slices.Clone(rt.notificationPresses) {
		rt.tree.within(h.owner, func() { h.fn(id) })
	}
}
