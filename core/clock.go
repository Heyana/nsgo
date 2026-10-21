package core

import (
	"sync"
	"time"
)

// Clock is the time a graph's timers run on. Go cannot hook time
// globally, so tests inject a ManualClock where an app would use the
// system's.
type Clock interface {
	Now() time.Time
	// AfterFunc calls f on its own goroutine after d, unless stop is
	// called first; stop reports whether it prevented the call.
	AfterFunc(d time.Duration, f func()) (stop func() bool)
}

// SystemClock is the system's time.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

func (SystemClock) AfterFunc(d time.Duration, f func()) func() bool {
	return time.AfterFunc(d, f).Stop
}

// ManualClock moves only when Advance is called. It is safe for
// concurrent use.
type ManualClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*manualTimer
}

type manualTimer struct {
	at      time.Time
	f       func()
	stopped bool
}

// NewManualClock returns a clock reading start.
func NewManualClock(start time.Time) *ManualClock { return &ManualClock{now: start} }

func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *ManualClock) AfterFunc(d time.Duration, f func()) func() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &manualTimer{at: c.now.Add(d), f: f}
	c.timers = append(c.timers, t)
	return func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		was := t.stopped
		t.stopped = true
		return !was
	}
}

// Advance moves the clock by d, calling the timers that come due on the
// way in order, each at its own time.
func (c *ManualClock) Advance(d time.Duration) {
	c.mu.Lock()
	end := c.now.Add(d)
	c.mu.Unlock()
	for {
		c.mu.Lock()
		var next *manualTimer
		for _, t := range c.timers {
			if !t.stopped && !t.at.After(end) && (next == nil || t.at.Before(next.at)) {
				next = t
			}
		}
		if next == nil {
			c.now = end
			c.timers = slicesDeleteStopped(c.timers)
			c.mu.Unlock()
			return
		}
		next.stopped = true
		c.now = next.at
		c.mu.Unlock()
		next.f()
	}
}

func slicesDeleteStopped(ts []*manualTimer) []*manualTimer {
	out := ts[:0]
	for _, t := range ts {
		if !t.stopped {
			out = append(out, t)
		}
	}
	return out
}
