package driver

import (
	"testing"
	"time"
)

func TestNoNotification(t *testing.T) {
	permission := NoNotification(&MountItem{Kind: ItemNotificationPermission, Count: 7})
	if permission.Kind != EventNotification || permission.Count != 7 || permission.Index != NotificationsUnavailable || permission.Finished {
		t.Fatalf("permission = %+v", permission)
	}
	show := NoNotification(&MountItem{Kind: ItemShowNotification, Count: 8})
	if show.Kind != EventNotification || show.Count != 8 || show.Index != 0 || show.Finished {
		t.Fatalf("show = %+v", show)
	}
}

func TestNextNotificationRepeatAndDST(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("timezone America/New_York unavailable: %v", err)
	}

	// Daily across spring-forward (2026-03-08 skips 02:00-02:59): 09:00 stays 09:00 EDT (23 hours later).
	before := time.Date(2026, 3, 7, 9, 0, 0, 0, ny)
	got := NextNotificationRepeat(before, NotificationDaily, before)
	want := time.Date(2026, 3, 8, 9, 0, 0, 0, ny)
	if !got.Equal(want) || got.Hour() != 9 || got.Sub(before) != 23*time.Hour {
		t.Fatalf("spring-forward daily = %v (delta %v), want %v", got, got.Sub(before), want)
	}

	// Daily across fall-back (2026-11-01 repeats 01:00-01:59): 09:00 stays 09:00 EST (25 hours later).
	beforeFall := time.Date(2026, 10, 31, 9, 0, 0, 0, ny)
	gotFall := NextNotificationRepeat(beforeFall, NotificationDaily, beforeFall)
	wantFall := time.Date(2026, 11, 1, 9, 0, 0, 0, ny)
	if !gotFall.Equal(wantFall) || gotFall.Hour() != 9 || gotFall.Sub(beforeFall) != 25*time.Hour {
		t.Fatalf("fall-back daily = %v (delta %v), want %v", gotFall, gotFall.Sub(beforeFall), wantFall)
	}

	// Weekly lands on the matching weekday and moves ahead 7 days when now >= target.
	wed := time.Date(2026, 1, 7, 14, 30, 0, 0, time.UTC)
	mon := time.Date(2026, 1, 5, 10, 0, 0, 0, time.UTC)
	if got := NextNotificationRepeat(wed, NotificationWeekly, mon); !got.Equal(wed) {
		t.Fatalf("weekly before target = %v, want %v", got, wed)
	}
	nextWed := time.Date(2026, 1, 14, 14, 30, 0, 0, time.UTC)
	if got := NextNotificationRepeat(wed, NotificationWeekly, wed); !got.Equal(nextWed) {
		t.Fatalf("weekly at target = %v, want %v", got, nextWed)
	}
	if got := NextNotificationRepeat(wed, NotificationOnce, mon); !got.IsZero() {
		t.Fatalf("once repeat = %v, want zero", got)
	}
}

type fakeNotifTimer struct {
	at      time.Time
	fn      func()
	stopped bool
}

func (f *fakeNotifTimer) Stop() bool {
	if f.stopped {
		return false
	}
	f.stopped = true
	return true
}

type fakeNotifClock struct {
	now    time.Time
	timers []*fakeNotifTimer
}

func (c *fakeNotifClock) Now() time.Time { return c.now }

func (c *fakeNotifClock) AfterFunc(d time.Duration, fn func()) NotificationTimer {
	t := &fakeNotifTimer{at: c.now.Add(d), fn: fn}
	c.timers = append(c.timers, t)
	return t
}

func (c *fakeNotifClock) Advance(d time.Duration) {
	target := c.now.Add(d)
	for {
		var next *fakeNotifTimer
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

func TestNotificationSchedulerOnceReplaceCancelAndChunking(t *testing.T) {
	clk := &fakeNotifClock{now: time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)}
	type delivery struct {
		n      Notification
		nextAt time.Time
		seq    uint64
	}
	var fired []delivery
	sched := &NotificationScheduler{
		Now:       clk.Now,
		AfterFunc: clk.AfterFunc,
		MaxStep:   30 * time.Second,
		Fire: func(n Notification, nextAt time.Time, seq uint64) {
			fired = append(fired, delivery{n: n, nextAt: nextAt, seq: seq})
		},
	}

	seqA, ok := sched.Schedule(Notification{ID: "a", Title: "A", At: clk.now.Add(2 * time.Minute)})
	if !ok || seqA == 0 {
		t.Fatal("failed to schedule a")
	}
	seqB, ok := sched.Schedule(Notification{ID: "b", Title: "B", At: clk.now.Add(time.Minute)})
	if !ok {
		t.Fatal("failed to schedule b")
	}
	// Replacing "a" moves it earlier and invalidates seqA.
	seqA2, ok := sched.Schedule(Notification{ID: "a", Title: "A2", At: clk.now.Add(90 * time.Second)})
	if !ok || !sched.Canceled("a", seqA) || sched.Canceled("a", seqA2) {
		t.Fatalf("replace cancel tracking wrong: old=%v new=%v", sched.Canceled("a", seqA), sched.Canceled("a", seqA2))
	}
	// Cancel "b" before it fires.
	if !sched.Cancel("b") || !sched.Canceled("b", seqB) {
		t.Fatal("cancel b failed")
	}

	clk.Advance(89 * time.Second)
	if len(fired) != 0 {
		t.Fatalf("fired early: %+v", fired)
	}
	clk.Advance(time.Second)
	if len(fired) != 1 || fired[0].n.ID != "a" || fired[0].n.Title != "A2" || !fired[0].nextAt.IsZero() {
		t.Fatalf("fired = %+v", fired)
	}
	if len(sched.Pending()) != 0 {
		t.Fatalf("pending after once = %+v", sched.Pending())
	}
	// A one-shot that fired without being canceled still reports Canceled == false.
	if sched.Canceled("a", seqA2) {
		t.Fatal("fired one-shot unexpectedly reported canceled")
	}
	// Closing marks even previously fired sequences canceled and stops new schedules.
	sched.Close()
	if !sched.Canceled("a", seqA2) {
		t.Fatal("closed scheduler should report canceled")
	}
	if _, ok := sched.Schedule(Notification{ID: "c", Title: "C", At: clk.now.Add(time.Minute)}); ok {
		t.Fatal("closed scheduler accepted schedule")
	}
}

func TestNotificationSchedulerDailyAndWeeklyRepeats(t *testing.T) {
	clk := &fakeNotifClock{now: time.Date(2026, 1, 1, 8, 0, 0, 0, time.UTC)} // Thursday
	var fired []Notification
	sched := &NotificationScheduler{
		Now:       clk.Now,
		AfterFunc: clk.AfterFunc,
		MaxStep:   -1,
		Fire: func(n Notification, _ time.Time, _ uint64) {
			fired = append(fired, n)
		},
	}

	sched.Schedule(Notification{
		ID:     "daily",
		Title:  "Daily",
		At:     time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
		Repeat: NotificationDaily,
	})
	sched.Schedule(Notification{
		ID:     "weekly",
		Title:  "Weekly",
		At:     time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
		Repeat: NotificationWeekly,
	})

	clk.Advance(2 * time.Hour) // 10:00 Thu
	if len(fired) != 2 || fired[0].ID != "daily" || fired[1].ID != "weekly" {
		t.Fatalf("first day fired = %+v", fired)
	}

	// Next day: only daily fires.
	clk.Advance(24 * time.Hour) // 10:00 Fri
	if len(fired) != 3 || fired[2].ID != "daily" {
		t.Fatalf("second day fired = %+v", fired)
	}

	// Cancel all: nothing fires over the next week.
	if !sched.Cancel("") {
		t.Fatal("Cancel(\"\") reported nothing removed")
	}
	clk.Advance(7 * 24 * time.Hour)
	if len(fired) != 3 {
		t.Fatalf("fired after cancel all = %+v", fired)
	}
}
