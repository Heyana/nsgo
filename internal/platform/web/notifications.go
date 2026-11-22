//go:build js && wasm

package web

import (
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// System notifications through the page's Notification API, and the badge
// of an installed web app's icon through navigator.setAppBadge. A page
// whose browser has no Notification, or shows them only from a service
// worker, as Chrome on Android, answers unavailable or fails to show.
// Scheduled notifications are kept in memory while the page runs, because
// browsers have no standard background schedule API.

// shownNotifications are the notifications the page shows, by ID, which
// one of the same ID and CloseNotification close.
var shownNotifications = map[string]*shownNotification{}

// shownNotification is a shown notification and the function its click
// and its close, the user's dismissal included, call.
type shownNotification struct {
	note  js.Value
	ended js.Func
}

// endNotification closes the notification of id, if the page shows one,
// and forgets it.
func endNotification(id string) {
	n, ok := shownNotifications[id]
	if !ok {
		return
	}
	delete(shownNotifications, id)
	n.note.Set("onclick", js.Null())
	n.note.Set("onclose", js.Null())
	n.note.Call("close")
	n.ended.Release()
}

func (s *surface) notificationScheduler() *webNotificationScheduler {
	if s.notifications == nil {
		s.notifications = newWebNotificationScheduler(
			func() bool {
				api := js.Global().Get("Notification")
				return api.Truthy() && api.Get("permission").String() == "granted"
			},
			s.showWebNotification,
			s.closeWebNotifications,
		)
	}
	return s.notifications
}

func (s *surface) showWebNotification(n driver.Notification) bool {
	api := js.Global().Get("Notification")
	if !api.Truthy() || api.Get("permission").String() != "granted" {
		return false
	}
	endNotification(n.ID)
	shown := func() (note js.Value, ok bool) {
		// The constructor throws where only a service worker shows
		// them.
		defer func() { ok = recover() == nil }()
		return api.New(n.Title, map[string]any{"body": n.Body, "tag": n.ID, "silent": n.Silent}), true
	}
	note, ok := shown()
	if ok {
		shown := &shownNotification{note: note}
		shown.ended = js.FuncOf(func(_ js.Value, args []js.Value) any {
			endNotification(n.ID)
			if args[0].Get("type").String() == "click" {
				js.Global().Call("focus")
				s.engine.Dispatch(driver.Event{Kind: driver.EventNotificationPress, Text: n.ID})
			}
			return nil
		})
		shownNotifications[n.ID] = shown
		note.Set("onclick", shown.ended)
		note.Set("onclose", shown.ended)
	}
	return ok
}

func (s *surface) closeWebNotifications(id string) {
	if id == "" {
		for id := range shownNotifications {
			endNotification(id)
		}
		return
	}
	endNotification(id)
}

func (s *surface) closeNotifications() {
	if s.notifications != nil {
		s.notifications.Stop()
	}
}

func (s *surface) notify(it *driver.MountItem) {
	api := js.Global().Get("Notification")
	id := it.Count
	switch it.Kind {
	case driver.ItemNotificationPermission:
		answer := func(p js.Value) {
			s.engine.Dispatch(driver.Event{Kind: driver.EventNotification, Count: id, Index: notificationPermission(p)})
		}
		if !api.Truthy() {
			s.engine.Dispatch(driver.Event{Kind: driver.EventNotification, Count: id, Index: driver.NotificationsUnavailable})
			return
		}
		if !it.On || api.Get("permission").String() != "default" {
			answer(api.Get("permission"))
			return
		}
		then(api.Call("requestPermission"), answer, func(js.Value) { answer(api.Get("permission")) })
	case driver.ItemShowNotification:
		ok := s.notificationScheduler().Show(*it.Notification)
		s.engine.Dispatch(driver.Event{Kind: driver.EventNotification, Count: id, Finished: ok})
	case driver.ItemCloseNotification:
		s.notificationScheduler().Close(it.Value)
	case driver.ItemBadge:
		nav := js.Global().Get("navigator")
		switch {
		case !nav.Get("setAppBadge").Truthy():
		case it.Count == 0:
			then(nav.Call("clearAppBadge"), func(js.Value) {}, func(js.Value) {})
		default:
			then(nav.Call("setAppBadge", it.Count), func(js.Value) {}, func(js.Value) {})
		}
	case driver.ItemTaskProgress:
		// Browsers have no portable app-icon progress surface.
	}
}

// notificationPermission is Notification.permission's value as the
// driver's.
func notificationPermission(p js.Value) int {
	switch p.String() {
	case "granted":
		return driver.NotificationsGranted
	case "denied":
		return driver.NotificationsDenied
	}
	return driver.NotificationsNotAsked
}
