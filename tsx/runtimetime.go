package tsx

import (
	"sync"
	"time"
)

// executionClock measures elapsed work spans. Its lock also lets the host
// sample an active span while the engine is busy.
type executionClock struct {
	mu      sync.Mutex
	started time.Time
	elapsed time.Duration
}

func (c *executionClock) enter() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started.IsZero() {
		return false
	}
	c.started = time.Now()
	return true
}

func (c *executionClock) leave() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started.IsZero() {
		return false
	}
	c.elapsed += time.Since(c.started)
	c.started = time.Time{}
	return true
}

func (c *executionClock) read() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started.IsZero() {
		return c.elapsed
	}
	return c.elapsed + time.Since(c.started)
}

func (r *runtime) executionTime() (time.Duration, bool) {
	if r == nil || r.clock == nil {
		return 0, false
	}
	return r.clock.read(), true
}

func (r *runtime) measureJS(fn func() error) error {
	if end := r.cx.BeginWork(); end != nil {
		defer end()
	}
	if r.clock != nil && r.clock.enter() {
		defer r.clock.leave()
	}
	return fn()
}
