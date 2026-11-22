package linux

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakeLinuxTimer struct {
	at      time.Time
	fn      func()
	stopped bool
}

func (f *fakeLinuxTimer) Stop() bool {
	if f.stopped {
		return false
	}
	f.stopped = true
	return true
}

type fakeLinuxClock struct {
	now    time.Time
	timers []*fakeLinuxTimer
}

func (c *fakeLinuxClock) Now() time.Time { return c.now }

func (c *fakeLinuxClock) AfterFunc(d time.Duration, fn func()) driver.NotificationTimer {
	t := &fakeLinuxTimer{at: c.now.Add(d), fn: fn}
	c.timers = append(c.timers, t)
	return t
}

func (c *fakeLinuxClock) Advance(d time.Duration) {
	target := c.now.Add(d)
	for {
		var next *fakeLinuxTimer
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

func TestLinuxNotificationSchedulerOnceDailyWeeklyAndClose(t *testing.T) {
	clk := &fakeLinuxClock{now: time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)} // Monday
	serverUp := true
	type shownCall struct {
		appID string
		n     driver.Notification
	}
	var shown []shownCall
	var closed []string

	s := newLinuxNotificationScheduler(
		func() bool { return serverUp },
		func(appID string, n driver.Notification) bool {
			shown = append(shown, shownCall{appID: appID, n: n})
			return true
		},
		func(id string) { closed = append(closed, id) },
		nil,
	)
	s.sched.Now = clk.Now
	s.sched.AfterFunc = clk.AfterFunc
	s.sched.MaxStep = -1

	// Unavailable notification server refuses scheduled notifications.
	serverUp = false
	if s.Show("dev.app", driver.Notification{ID: "no", Title: "No", At: clk.now.Add(time.Minute)}) {
		t.Fatal("scheduled while server unavailable")
	}
	serverUp = true

	// One-shot, then replaced by a later time with the same ID.
	if !s.Show("dev.app", driver.Notification{ID: "once", Title: "Old", At: clk.now.Add(time.Minute)}) {
		t.Fatal("schedule once failed")
	}
	if !s.Show("dev.app", driver.Notification{ID: "once", Title: "New", At: clk.now.Add(2 * time.Minute)}) {
		t.Fatal("replace once failed")
	}

	// Daily at 09:00 and weekly on Monday 10:00.
	if !s.Show("dev.app", driver.Notification{ID: "daily", Title: "Daily", At: time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC), Repeat: driver.NotificationDaily}) {
		t.Fatal("schedule daily failed")
	}
	if !s.Show("dev.app", driver.Notification{ID: "weekly", Title: "Weekly", At: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC), Repeat: driver.NotificationWeekly}) {
		t.Fatal("schedule weekly failed")
	}

	// Advance 90s: replaced "Old" must not fire.
	clk.Advance(90 * time.Second)
	if len(shown) != 0 {
		t.Fatalf("fired early: %+v", shown)
	}
	// Advance 30s (to +2m): "New" fires with At zeroed.
	clk.Advance(30 * time.Second)
	if len(shown) != 1 || shown[0].appID != "dev.app" || shown[0].n.Title != "New" || !shown[0].n.At.IsZero() {
		t.Fatalf("after 2m shown = %+v", shown)
	}

	// Advance to 10:00: daily and weekly fire.
	clk.Advance(2*time.Hour - 2*time.Minute)
	if len(shown) != 3 || shown[1].n.ID != "daily" || shown[2].n.ID != "weekly" {
		t.Fatalf("at 10:00 shown = %+v", shown)
	}

	// Next day 10:00: only daily fires.
	clk.Advance(24 * time.Hour)
	if len(shown) != 4 || shown[3].n.ID != "daily" {
		t.Fatalf("next day shown = %+v", shown)
	}

	// Cancel "daily" by ID; weekly still fires the following Monday.
	s.Close("daily")
	clk.Advance(6 * 24 * time.Hour)
	if len(shown) != 5 || shown[4].n.ID != "weekly" {
		t.Fatalf("following week shown = %+v", shown)
	}

	// Stop on shutdown prevents any future delivery.
	s.Stop()
	clk.Advance(14 * 24 * time.Hour)
	if len(shown) != 5 {
		t.Fatalf("fired after Stop: %+v", shown)
	}
	if len(closed) != 1 || closed[0] != "daily" {
		t.Fatalf("closed = %+v", closed)
	}
}

func TestLinuxNotificationSchedulerSkipsQueuedWhenCanceled(t *testing.T) {
	clk := &fakeLinuxClock{now: time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)}
	var queued []func()
	var shown []driver.Notification

	s := newLinuxNotificationScheduler(
		func() bool { return true },
		func(_ string, n driver.Notification) bool {
			shown = append(shown, n)
			return true
		},
		nil,
		func(fn func()) { queued = append(queued, fn) },
	)
	s.sched.Now = clk.Now
	s.sched.AfterFunc = clk.AfterFunc
	s.sched.MaxStep = -1

	s.Show("dev.app", driver.Notification{ID: "race", Title: "Race", At: clk.now.Add(time.Minute)})
	clk.Advance(time.Minute)
	if len(queued) != 1 {
		t.Fatalf("queued = %d, want 1", len(queued))
	}
	// User closes notification before the D-Bus worker dequeues it.
	s.Close("race")
	queued[0]()
	if len(shown) != 0 {
		t.Fatalf("canceled queued notification still showed: %+v", shown)
	}
}
