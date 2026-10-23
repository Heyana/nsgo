// Package hover follows the pointer over views for the backends whose
// views hear no hovers of their own, Windows' and Linux's. As a tracking
// area or pointerenter does, a view stays hovered while the pointer is
// over it or any view inside it.
package hover

import (
	"slices"

	"github.com/oneo-me/nsgo/core"
)

// Tracker holds the views the pointer is over that follow hovers.
type Tracker struct {
	over []core.NodeID // outermost first
}

// Move sets the views under the pointer that follow hovers, outermost
// first, and returns the events that report the change: leaving views
// innermost first, then entering ones outermost first.
func (t *Tracker) Move(under []core.NodeID) []core.Event {
	var events []core.Event
	for i := len(t.over) - 1; i >= 0; i-- {
		if !slices.Contains(under, t.over[i]) {
			events = append(events, core.Event{Kind: core.EventHoverOut, ID: t.over[i]})
		}
	}
	for _, id := range under {
		if !slices.Contains(t.over, id) {
			events = append(events, core.Event{Kind: core.EventHoverIn, ID: id})
		}
	}
	t.over = append(t.over[:0], under...)
	return events
}

// Forget drops id, whose view went away, without reporting it.
func (t *Tracker) Forget(id core.NodeID) {
	t.over = slices.DeleteFunc(t.over, func(o core.NodeID) bool { return o == id })
}

// Follows reports whether a view reporting events m follows hovers.
func Follows(m core.EventMask) bool {
	return m.Has(core.EventHoverIn) || m.Has(core.EventHoverOut)
}
