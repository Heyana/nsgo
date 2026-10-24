// Package finger reports a mouse as finger 0 of core.EventTouch, for the
// desktop backends: its primary button going down, moving while down and
// going up are a finger's touches.
package finger

import (
	"time"

	"github.com/oneo-me/nsgo/core"
)

// Mouse follows one mouse's primary button.
type Mouse struct {
	down   bool
	origin time.Time
}

// Listens reports whether a view reporting events m listens to touches.
func Listens(m core.EventMask) bool { return m.Has(core.EventTouch) }

// Down reports the button going down at p, in surface points, when a view
// listens; the touches it starts are reported until the button goes up.
func (m *Mouse) Down(listening bool, p core.Point) (core.Event, bool) {
	m.down = listening
	if m.origin.IsZero() {
		m.origin = time.Now()
	}
	return m.event(core.GestureBegan, p), listening
}

// Move reports the mouse moving to p while the button is down.
func (m *Mouse) Move(p core.Point) (core.Event, bool) {
	return m.event(core.GestureChanged, p), m.down
}

// Up reports the button going up at p.
func (m *Mouse) Up(p core.Point) (core.Event, bool) {
	was := m.down
	m.down = false
	return m.event(core.GestureEnded, p), was
}

// Cancel reports the touches taken away, such as by another window
// capturing the mouse, at p.
func (m *Mouse) Cancel(p core.Point) (core.Event, bool) {
	was := m.down
	m.down = false
	return m.event(core.GestureCancelled, p), was
}

func (m *Mouse) event(phase core.GesturePhase, p core.Point) core.Event {
	return core.Event{Kind: core.EventTouch, Phase: phase, Time: time.Since(m.origin),
		Touches: []core.Touch{{PagePosition: p, Changed: true}}}
}
