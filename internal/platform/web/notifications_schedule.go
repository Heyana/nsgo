package web

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// webNotificationScheduler keeps scheduled notifications in memory while the page runs, delivering each through the browser Notification API when its wall time arrives because browsers have no standard notification schedule API.
type webNotificationScheduler struct {
	sched      driver.NotificationScheduler
	permitted  func() bool
	show       func(n driver.Notification) bool
	closeShown func(id string)
}

func newWebNotificationScheduler(
	permitted func() bool,
	show func(n driver.Notification) bool,
	closeShown func(id string),
) *webNotificationScheduler {
	s := &webNotificationScheduler{
		permitted:  permitted,
		show:       show,
		closeShown: closeShown,
	}
	s.sched.Fire = s.onFire
	return s
}

// Show displays n immediately when At is zero, or schedules it while the page runs when At is set. It returns false without permission and cancels any previously scheduled notification with the same ID.
func (s *webNotificationScheduler) Show(n driver.Notification) bool {
	if s.permitted != nil && !s.permitted() {
		return false
	}
	s.sched.Cancel(n.ID)
	if n.At.IsZero() {
		return s.show != nil && s.show(n)
	}
	_, ok := s.sched.Schedule(n)
	return ok
}

// Close cancels pending scheduled notifications (id, or all when id is empty) and closes any currently shown notifications.
func (s *webNotificationScheduler) Close(id string) {
	s.sched.Cancel(id)
	if s.closeShown != nil {
		s.closeShown(id)
	}
}

// Stop stops every pending timer when the page or surface closes.
func (s *webNotificationScheduler) Stop() {
	s.sched.Close()
}

func (s *webNotificationScheduler) onFire(n driver.Notification, _ time.Time, seq uint64) {
	if s.sched.Canceled(n.ID, seq) {
		return
	}
	if s.permitted != nil && !s.permitted() {
		return
	}
	deliver := n
	deliver.At = time.Time{}
	if s.show != nil {
		s.show(deliver)
	}
}
