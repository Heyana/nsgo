package driver

import "time"

const panelFrameDuration = 200 * time.Millisecond

// PanelFrameMotion eases a desktop panel between two frames in its backend's
// native coordinates. The zero value is idle; the backend owns the frame clock.
type PanelFrameMotion struct {
	started  time.Time
	from, to Bounds
}

// Start moves from the frame currently shown, leaving an identical active
// destination alone. It reports whether frames are needed.
func (m *PanelFrameMotion) Start(from, to Bounds, now time.Time) bool {
	if from == to {
		m.Stop()
		return false
	}
	if !m.Active() || m.to != to {
		*m = PanelFrameMotion{started: now, from: from, to: to}
	}
	return true
}

// Active reports whether a destination still needs frames.
func (m *PanelFrameMotion) Active() bool { return !m.started.IsZero() }

// Stop leaves the panel where it was last placed.
func (m *PanelFrameMotion) Stop() { m.started = time.Time{} }

// Step returns the frame at now, or false if idle. The last sample is exactly
// the requested destination even when the display skipped frames.
func (m *PanelFrameMotion) Step(now time.Time, reducedMotion bool) (Bounds, bool) {
	if !m.Active() {
		return Bounds{}, false
	}
	elapsed := now.Sub(m.started)
	switch {
	case reducedMotion || elapsed >= panelFrameDuration:
		m.Stop()
		return m.to, true
	case elapsed <= 0:
		return m.from, true
	}
	left := 1 - float64(elapsed)/float64(panelFrameDuration)
	return LerpBounds(m.from, m.to, 1-left*left*left), true
}
