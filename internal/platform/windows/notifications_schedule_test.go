package windows

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakeToastTimer struct {
	at      time.Time
	fn      func()
	stopped bool
}

func (f *fakeToastTimer) Stop() bool {
	if f.stopped {
		return false
	}
	f.stopped = true
	return true
}

type fakeToastClock struct {
	now    time.Time
	timers []*fakeToastTimer
}

func (c *fakeToastClock) Now() time.Time { return c.now }

func (c *fakeToastClock) AfterFunc(d time.Duration, fn func()) driver.NotificationTimer {
	t := &fakeToastTimer{at: c.now.Add(d), fn: fn}
	c.timers = append(c.timers, t)
	return t
}

func (c *fakeToastClock) Advance(d time.Duration) {
	target := c.now.Add(d)
	for {
		var next *fakeToastTimer
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

func TestToastRepeatManagerDailyAndWeeklyChain(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("timezone America/New_York unavailable: %v", err)
	}
	// 2026-03-07 08:00 EST (the day before spring-forward DST).
	clk := &fakeToastClock{now: time.Date(2026, 3, 7, 8, 0, 0, 0, ny)}
	path := filepath.Join(t.TempDir(), toastScheduleName)
	var scheduled []driver.Notification
	var unscheduled []string

	m := newToastRepeatManager(
		path,
		func(n driver.Notification) (time.Time, bool) {
			scheduled = append(scheduled, n)
			return n.At, true
		},
		func(id string) { unscheduled = append(unscheduled, id) },
		nil,
	)
	m.sched.Now = clk.Now
	m.sched.AfterFunc = clk.AfterFunc
	m.sched.MaxStep = -1

	// One-shot does not persist or chain.
	onceAt := clk.now.Add(10 * time.Minute)
	if !m.Schedule(driver.Notification{ID: "once", Title: "Once", At: onceAt, Repeat: driver.NotificationOnce}) {
		t.Fatal("failed to schedule once")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("one-shot unexpectedly wrote %s", path)
	}

	// Daily across spring-forward DST.
	dailyAt := time.Date(2026, 3, 7, 9, 0, 0, 0, ny)
	if !m.Schedule(driver.Notification{ID: "daily", Title: "Standup", Body: "9am", At: dailyAt, Repeat: driver.NotificationDaily}) {
		t.Fatal("failed to schedule daily")
	}
	// Weekly on Saturday 10:00.
	weeklyAt := time.Date(2026, 3, 7, 10, 0, 0, 0, ny)
	if !m.Schedule(driver.Notification{ID: "weekly", Title: "Review", At: weeklyAt, Repeat: driver.NotificationWeekly}) {
		t.Fatal("failed to schedule weekly")
	}
	if len(scheduled) != 3 {
		t.Fatalf("initial scheduled = %d, want 3", len(scheduled))
	}

	// Advance past 09:00:02 EST -> daily chains to 2026-03-08 09:00 EDT (23h later).
	clk.Advance(time.Hour + 2*time.Second)
	if len(scheduled) != 4 {
		t.Fatalf("after daily fire: scheduled = %+v", scheduled)
	}
	wantDay2 := time.Date(2026, 3, 8, 9, 0, 0, 0, ny)
	if !scheduled[3].At.Equal(wantDay2) || scheduled[3].At.Hour() != 9 {
		t.Fatalf("daily chained to %v, want %v", scheduled[3].At, wantDay2)
	}

	// Advance past 10:00:02 EST -> weekly chains to Saturday 2026-03-14 10:00 EDT.
	clk.Advance(time.Hour)
	if len(scheduled) != 5 {
		t.Fatalf("after weekly fire: scheduled = %+v", scheduled)
	}
	wantWeek2 := time.Date(2026, 3, 14, 10, 0, 0, 0, ny)
	if !scheduled[4].At.Equal(wantWeek2) || scheduled[4].At.Weekday() != time.Saturday || scheduled[4].At.Hour() != 10 {
		t.Fatalf("weekly chained to %v, want %v", scheduled[4].At, wantWeek2)
	}

	// Replacing "daily" with a one-shot removes it from repeating persistence.
	if !m.Schedule(driver.Notification{ID: "daily", Title: "OnceNow", At: clk.now.Add(time.Hour), Repeat: driver.NotificationOnce}) {
		t.Fatal("replace daily with once failed")
	}
	if p := m.pending(); len(p) != 1 || p[0].ID != "weekly" {
		t.Fatalf("pending after replace = %+v, want only weekly", p)
	}

	// Cancel all clears memory and removes the persisted file.
	m.Cancel("")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Cancel(\"\") left %s behind", path)
	}
}

func TestToastRepeatManagerStopAndRearmAcrossRestart(t *testing.T) {
	clk := &fakeToastClock{now: time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)}
	path := filepath.Join(t.TempDir(), toastScheduleName)
	var scheduled []driver.Notification
	var unscheduled []string

	makeManager := func() *toastRepeatManager {
		m := newToastRepeatManager(
			path,
			func(n driver.Notification) (time.Time, bool) {
				scheduled = append(scheduled, n)
				return n.At, true
			},
			func(id string) { unscheduled = append(unscheduled, id) },
			nil,
		)
		m.sched.Now = clk.Now
		m.sched.AfterFunc = clk.AfterFunc
		m.sched.MaxStep = -1
		return m
	}

	// Rearm with no file must not touch the notifier.
	m0 := makeManager()
	m0.Rearm()
	if len(scheduled) != 0 || len(unscheduled) != 0 {
		t.Fatalf("empty Rearm touched notifier: scheduled=%+v unscheduled=%+v", scheduled, unscheduled)
	}

	// Schedule a daily 09:00 and a daily 20:00, then stop the app (exiting).
	m1 := makeManager()
	m1.Schedule(driver.Notification{ID: "morning", Title: "Morning", At: time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC), Repeat: driver.NotificationDaily})
	m1.Schedule(driver.Notification{ID: "evening", Title: "Evening", At: time.Date(2026, 5, 1, 20, 0, 0, 0, time.UTC), Repeat: driver.NotificationDaily})
	m1.Stop()

	// App stays closed until 2026-05-03 10:00 UTC (2 days later): stopped timers must not fire!
	scheduled = nil
	unscheduled = nil
	clk.Advance(50 * time.Hour)
	if len(scheduled) != 0 {
		t.Fatalf("stopped manager fired while closed: %+v", scheduled)
	}

	// Next launch rearms: morning (09:00) advances to 2026-05-04 09:00, evening (20:00) advances to 2026-05-03 20:00.
	m2 := makeManager()
	m2.Rearm()
	if len(scheduled) != 2 {
		t.Fatalf("rearm scheduled = %+v, want 2", scheduled)
	}
	byID := map[string]time.Time{}
	for _, n := range scheduled {
		byID[n.ID] = n.At
	}
	if want := time.Date(2026, 5, 3, 20, 0, 0, 0, time.UTC); !byID["evening"].Equal(want) {
		t.Fatalf("rearmed evening = %v, want %v", byID["evening"], want)
	}
	if want := time.Date(2026, 5, 4, 9, 0, 0, 0, time.UTC); !byID["morning"].Equal(want) {
		t.Fatalf("rearmed morning = %v, want %v", byID["morning"], want)
	}

	// While m2 runs, advancing past 20:00:02 chains evening to 2026-05-04 20:00.
	clk.Advance(10*time.Hour + 2*time.Second)
	if len(scheduled) != 3 || scheduled[2].ID != "evening" || !scheduled[2].At.Equal(time.Date(2026, 5, 4, 20, 0, 0, 0, time.UTC)) {
		t.Fatalf("rearmed chain = %+v", scheduled)
	}
}

func TestToastRepeatManagerRefusesFailedSchedule(t *testing.T) {
	clk := &fakeToastClock{now: time.Date(2026, 5, 1, 8, 0, 0, 0, time.UTC)}
	path := filepath.Join(t.TempDir(), toastScheduleName)
	m := newToastRepeatManager(
		path,
		func(driver.Notification) (time.Time, bool) { return time.Time{}, false },
		nil,
		nil,
	)
	m.sched.Now = clk.Now
	m.sched.AfterFunc = clk.AfterFunc
	m.sched.MaxStep = -1

	if m.Schedule(driver.Notification{ID: "fail", Title: "Fail", At: clk.now.Add(time.Hour), Repeat: driver.NotificationDaily}) {
		t.Fatal("Schedule succeeded when notifier failed")
	}
	if len(m.pending()) != 0 {
		t.Fatalf("pending after failed schedule = %+v", m.pending())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("failed schedule created %s", path)
	}
}
