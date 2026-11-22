package driver

import (
	"slices"
	"strings"
	"sync"
	"time"
)

// NextNotificationRepeat returns the first wall time strictly after now matching at's clock reading (in at's location), and at's weekday for NotificationWeekly, evaluated in now's location. It returns the zero time for NotificationOnce or an unknown repeat.
func NextNotificationRepeat(at time.Time, r NotificationRepeat, now time.Time) time.Time {
	h, m, s := at.Clock()
	return NextClockRepeat(h, m, s, at.Weekday(), r, now)
}

// NextClockRepeat returns the first wall time strictly after now matching hour:minute:second (and weekday for NotificationWeekly) in now's location. Constructing the date from calendar offsets avoids wall-clock drift across daylight saving transitions.
func NextClockRepeat(hour, minute, second int, weekday time.Weekday, r NotificationRepeat, now time.Time) time.Time {
	if r != NotificationDaily && r != NotificationWeekly {
		return time.Time{}
	}
	loc := now.Location()
	y, m, d := now.Date()
	offset := 0
	step := 1
	if r == NotificationWeekly {
		step = 7
		offset = (int(weekday) - int(now.Weekday()) + 7) % 7
	}
	next := time.Date(y, m, d+offset, hour, minute, second, 0, loc)
	for !next.After(now) {
		offset += step
		next = time.Date(y, m, d+offset, hour, minute, second, 0, loc)
	}
	return next
}

// NotificationTimer is a cancellable timer used by NotificationScheduler.
type NotificationTimer interface {
	Stop() bool
}

const defaultNotificationMaxStep = time.Minute

type scheduledNotification struct {
	n       Notification
	hour    int
	minute  int
	second  int
	weekday time.Weekday
	seq     uint64
	timer   NotificationTimer
}

// NotificationScheduler keeps scheduled notifications in memory while the app runs, invoking Fire as each wall time arrives and re-arming daily and weekly repeats. It is safe for concurrent use.
type NotificationScheduler struct {
	mu       sync.Mutex
	entries  map[string]*scheduledNotification
	canceled map[string]uint64
	seq      uint64
	clearSeq uint64
	closed   bool

	// Fire runs outside the scheduler's lock when a scheduled notification comes due. nextAt is the next scheduled time for repeating notifications and zero for NotificationOnce. Async handlers can check Canceled(n.ID, seq) before delivering.
	Fire func(n Notification, nextAt time.Time, seq uint64)

	// Now returns the current wall time; defaults to time.Now.
	Now func() time.Time

	// AfterFunc schedules fn to run after d; defaults to time.AfterFunc.
	AfterFunc func(d time.Duration, fn func()) NotificationTimer

	// MaxStep caps each timer wait so suspend/resume, wall-clock jumps and JavaScript's 32-bit setTimeout limit are handled without drift. Zero uses one minute; a negative duration disables chunking.
	MaxStep time.Duration
}

// Schedule schedules n for its At (and repeats for Daily/Weekly), replacing any pending notification with the same ID. It returns the entry's sequence number and whether it was scheduled.
func (s *NotificationScheduler) Schedule(n Notification) (uint64, bool) {
	if n.ID == "" || n.At.IsZero() {
		return 0, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return 0, false
	}
	if s.entries == nil {
		s.entries = map[string]*scheduledNotification{}
	}
	if s.canceled == nil {
		s.canceled = map[string]uint64{}
	}
	if old := s.entries[n.ID]; old != nil && old.timer != nil {
		old.timer.Stop()
	}
	s.seq++
	seq := s.seq
	s.canceled[n.ID] = seq
	h, m, sec := n.At.Clock()
	e := &scheduledNotification{
		n:       n,
		hour:    h,
		minute:  m,
		second:  sec,
		weekday: n.At.Weekday(),
		seq:     seq,
	}
	s.entries[n.ID] = e
	s.armLocked(e, s.now())
	return seq, true
}

// Cancel cancels the pending notification called id, or every pending notification when id is empty. It reports whether any pending notification was removed.
func (s *NotificationScheduler) Cancel(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	if id == "" {
		removed := len(s.entries) > 0
		for k, e := range s.entries {
			if e.timer != nil {
				e.timer.Stop()
			}
			delete(s.entries, k)
		}
		s.clearSeq = s.seq
		clear(s.canceled)
		return removed
	}
	if s.canceled == nil {
		s.canceled = map[string]uint64{}
	}
	s.canceled[id] = s.seq
	e, ok := s.entries[id]
	if !ok {
		return false
	}
	if e.timer != nil {
		e.timer.Stop()
	}
	delete(s.entries, id)
	return true
}

// Canceled reports whether the scheduled notification with id and sequence seq has since been canceled, replaced, cleared or closed.
func (s *NotificationScheduler) Canceled(id string, seq uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed || s.clearSeq > seq || s.canceled[id] > seq
}

// Pending returns a snapshot of the currently pending notifications, sorted by scheduled time then ID.
func (s *NotificationScheduler) Pending() []Notification {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Notification, 0, len(s.entries))
	for _, e := range s.entries {
		out = append(out, e.n)
	}
	slices.SortFunc(out, func(a, b Notification) int {
		if c := a.At.Compare(b.At); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}

// Close stops every pending timer and prevents future scheduling or delivery.
func (s *NotificationScheduler) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.seq++
	s.clearSeq = s.seq
	for k, e := range s.entries {
		if e.timer != nil {
			e.timer.Stop()
		}
		delete(s.entries, k)
	}
	clear(s.canceled)
}

func (s *NotificationScheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *NotificationScheduler) maxStep() time.Duration {
	if s.MaxStep < 0 {
		return 0
	}
	if s.MaxStep == 0 {
		return defaultNotificationMaxStep
	}
	return s.MaxStep
}

func (s *NotificationScheduler) afterFunc(d time.Duration, fn func()) NotificationTimer {
	if s.AfterFunc != nil {
		return s.AfterFunc(d, fn)
	}
	return time.AfterFunc(d, fn)
}

func (s *NotificationScheduler) armLocked(e *scheduledNotification, now time.Time) {
	delay := max(e.n.At.Sub(now), 0)
	if step := s.maxStep(); step > 0 && delay > step {
		delay = step
	}
	id, seq := e.n.ID, e.seq
	e.timer = s.afterFunc(delay, func() { s.onTick(id, seq) })
}

func (s *NotificationScheduler) onTick(id string, seq uint64) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	e := s.entries[id]
	if e == nil || e.seq != seq {
		s.mu.Unlock()
		return
	}
	now := s.now()
	if now.Before(e.n.At) {
		s.armLocked(e, now)
		s.mu.Unlock()
		return
	}
	fired := e.n
	var nextAt time.Time
	if e.n.Repeat == NotificationOnce {
		delete(s.entries, id)
	} else {
		nextAt = NextClockRepeat(e.hour, e.minute, e.second, e.weekday, e.n.Repeat, now)
		e.n.At = nextAt
		s.armLocked(e, now)
	}
	fire := s.Fire
	s.mu.Unlock()
	if fire != nil {
		fire(fired, nextAt, seq)
	}
}
