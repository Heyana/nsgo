package web

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakeWebTimer struct {
	at      time.Time
	fn      func()
	stopped bool
}

func (f *fakeWebTimer) Stop() bool {
	if f.stopped {
		return false
	}
	f.stopped = true
	return true
}

type fakeWebClock struct {
	now    time.Time
	timers []*fakeWebTimer
}

func (c *fakeWebClock) Now() time.Time { return c.now }

func (c *fakeWebClock) AfterFunc(d time.Duration, fn func()) driver.NotificationTimer {
	t := &fakeWebTimer{at: c.now.Add(d), fn: fn}
	c.timers = append(c.timers, t)
	return t
}

func (c *fakeWebClock) Advance(d time.Duration) {
	target := c.now.Add(d)
	for {
		var next *fakeWebTimer
		nextIdx := -1
		for i, t := range c.timers {
			if t.stopped {
				continue
			}
			if !t.at.After(target) && (next == nil || t.at.Before(next.at)) {
				next = t
				nextIdx = i
			}
		}
		if next == nil {
			c.now = target
			return
		}
		c.now = next.at
		next.stopped = true
		c.timers = append(c.timers[:nextIdx], c.timers[nextIdx+1:]...)
		next.fn()
	}
}

func TestWebNotificationSchedulerPermissionScheduleAndRepeat(t *testing.T) {
	clk := &fakeWebClock{now: time.Date(2026, 7, 1, 8, 0, 0, 0, time.UTC)} // Wednesday
	granted := false
	var shown []driver.Notification
	var closed []string

	s := newWebNotificationScheduler(
		func() bool { return granted },
		func(n driver.Notification) bool {
			shown = append(shown, n)
			return true
		},
		func(id string) { closed = append(closed, id) },
	)
	s.sched.Now = clk.Now
	s.sched.AfterFunc = clk.AfterFunc
	s.sched.MaxStep = -1

	// Without permission, scheduling is refused.
	if s.Show(driver.Notification{ID: "a", Title: "Denied", At: clk.now.Add(time.Minute)}) {
		t.Fatal("scheduled without permission")
	}

	granted = true
	if !s.Show(driver.Notification{ID: "once", Title: "Once", At: clk.now.Add(time.Minute)}) {
		t.Fatal("schedule once failed")
	}
	if !s.Show(driver.Notification{ID: "daily", Title: "Daily", At: time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC), Repeat: driver.NotificationDaily}) {
		t.Fatal("schedule daily failed")
	}
	if !s.Show(driver.Notification{ID: "weekly", Title: "Weekly", At: time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC), Repeat: driver.NotificationWeekly}) {
		t.Fatal("schedule weekly failed")
	}

	// Replace "once" with an immediate notification: old timer must not fire.
	if !s.Show(driver.Notification{ID: "once", Title: "Now"}) {
		t.Fatal("immediate replace failed")
	}
	clk.Advance(time.Minute)
	if len(shown) != 1 || shown[0].Title != "Now" {
		t.Fatalf("after immediate replace shown = %+v", shown)
	}

	// Advance to 10:00: daily and weekly fire.
	clk.Advance(2*time.Hour - time.Minute)
	if len(shown) != 3 || shown[1].ID != "daily" || shown[2].ID != "weekly" {
		t.Fatalf("at 10:00 shown = %+v", shown)
	}

	// Revoke permission before the next day: daily skips showing while revoked.
	granted = false
	clk.Advance(24 * time.Hour)
	if len(shown) != 3 {
		t.Fatalf("fired while revoked: %+v", shown)
	}

	// Restore permission: daily resumes on day 3.
	granted = true
	clk.Advance(24 * time.Hour)
	if len(shown) != 4 || shown[3].ID != "daily" {
		t.Fatalf("after re-grant shown = %+v", shown)
	}

	// Close all cancels remaining schedules and calls closeShown("").
	s.Close("")
	clk.Advance(7 * 24 * time.Hour)
	if len(shown) != 4 {
		t.Fatalf("fired after Close(\"\"): %+v", shown)
	}
	if len(closed) != 1 || closed[0] != "" {
		t.Fatalf("closed = %+v", closed)
	}

	// Stop prevents future scheduling on pagehide.
	s.Stop()
	if s.Show(driver.Notification{ID: "late", Title: "Late", At: clk.now.Add(time.Minute)}) {
		t.Fatal("scheduled after Stop")
	}
}
