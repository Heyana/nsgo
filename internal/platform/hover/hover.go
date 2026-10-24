// Package hover follows the pointer over views for the backends whose
// views hear no hovers of their own, Windows' and Linux's. As a tracking
// area or pointerenter does, a view stays hovered while the pointer is
// over it or any view inside it.
package hover

import (
	"slices"

	"github.com/oneo-me/nsgo/internal/driver"
)

// Tracker holds the views the pointer is over that follow hovers.
type Tracker struct {
	over []driver.NodeID // outermost first
}

// Move sets the views under the pointer that follow hovers, outermost
// first, and returns the events that report the change: leaving views
// innermost first, then entering ones outermost first.
func (t *Tracker) Move(under []driver.NodeID) []driver.Event {
	var events []driver.Event
	for i := len(t.over) - 1; i >= 0; i-- {
		if !slices.Contains(under, t.over[i]) {
			events = append(events, driver.Event{Kind: driver.EventHoverOut, ID: t.over[i]})
		}
	}
	for _, id := range under {
		if !slices.Contains(t.over, id) {
			events = append(events, driver.Event{Kind: driver.EventHoverIn, ID: id})
		}
	}
	t.over = append(t.over[:0], under...)
	return events
}

// Forget drops id, whose view went away, without reporting it.
func (t *Tracker) Forget(id driver.NodeID) {
	t.over = slices.DeleteFunc(t.over, func(o driver.NodeID) bool { return o == id })
}

// Follows reports whether a view reporting events m follows hovers: any
// of the three hover events makes it take part.
func Follows(m driver.EventMask) bool {
	return m.Has(driver.EventHoverIn) || m.Has(driver.EventHoverOut) || m.Has(driver.EventHoverMove)
}
