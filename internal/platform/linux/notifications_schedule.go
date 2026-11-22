package linux

import (
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// linuxNotificationScheduler keeps scheduled notifications in memory while the Linux app runs, delivering each to org.freedesktop.Notifications when its wall time arrives because the desktop notification server offers no schedule.
type linuxNotificationScheduler struct {
	mu         sync.Mutex
	appIDs     map[string]string
	sched      driver.NotificationScheduler
	available  func() bool
	show       func(appID string, n driver.Notification) bool
	closeShown func(id string)
	enqueue    func(func())
}

func newLinuxNotificationScheduler(
	available func() bool,
	show func(appID string, n driver.Notification) bool,
	closeShown func(id string),
	enqueue func(func()),
) *linuxNotificationScheduler {
	s := &linuxNotificationScheduler{
		appIDs:     map[string]string{},
		available:  available,
		show:       show,
		closeShown: closeShown,
		enqueue:    enqueue,
	}
	s.sched.Fire = s.onFire
	return s
}

// Show displays n immediately when At is zero, or schedules it while the app runs when At is set. Showing or scheduling an ID cancels any pending scheduled notification of the same ID.
func (s *linuxNotificationScheduler) Show(appID string, n driver.Notification) bool {
	s.mu.Lock()
	delete(s.appIDs, n.ID)
	s.sched.Cancel(n.ID)
	s.mu.Unlock()

	if n.At.IsZero() {
		return s.show != nil && s.show(appID, n)
	}
	if s.available != nil && !s.available() {
		return false
	}
	s.mu.Lock()
	s.appIDs[n.ID] = appID
	_, ok := s.sched.Schedule(n)
	if !ok {
		delete(s.appIDs, n.ID)
	}
	s.mu.Unlock()
	return ok
}

// Close cancels scheduled notifications (id, or all when id is empty) and closes any currently shown notifications.
func (s *linuxNotificationScheduler) Close(id string) {
	s.mu.Lock()
	s.sched.Cancel(id)
	if id == "" {
		clear(s.appIDs)
	} else {
		delete(s.appIDs, id)
	}
	s.mu.Unlock()
	if s.closeShown != nil {
		s.closeShown(id)
	}
}

// Stop stops all pending scheduled notification timers on app shutdown.
func (s *linuxNotificationScheduler) Stop() {
	s.mu.Lock()
	s.sched.Close()
	clear(s.appIDs)
	s.mu.Unlock()
}

func (s *linuxNotificationScheduler) onFire(n driver.Notification, nextAt time.Time, seq uint64) {
	s.mu.Lock()
	appID := s.appIDs[n.ID]
	if nextAt.IsZero() && !s.sched.Canceled(n.ID, seq) {
		delete(s.appIDs, n.ID)
	}
	s.mu.Unlock()

	deliver := n
	deliver.At = time.Time{}
	run := func() {
		if s.sched.Canceled(deliver.ID, seq) || s.show == nil {
			return
		}
		s.show(appID, deliver)
	}
	if s.enqueue != nil {
		s.enqueue(run)
	} else {
		run()
	}
}
