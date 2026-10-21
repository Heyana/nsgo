package core

// EventKind is what a platform reports about a view.
type EventKind uint8

const (
	// EventPress is a completed press: tap, click or accessibility action.
	EventPress EventKind = iota + 1
	// EventPressDown and EventPressUp bracket a press as it happens.
	EventPressDown
	EventPressUp
	// EventLongPress is a press held past the platform's delay.
	EventLongPress
	// EventHoverIn and EventHoverOut follow a pointer over a view.
	EventHoverIn
	EventHoverOut
	// EventFocus and EventBlur follow keyboard focus.
	EventFocus
	EventBlur
	// EventChangeText carries a text field's text after a user edit
	// (Text) and the edit's number (Count).
	EventChangeText
	// EventSubmitEditing is Return in a text field (Text).
	EventSubmitEditing
	// EventScroll carries a scroll view's offset (Point).
	EventScroll
)

// EventMask is a set of event kinds.
type EventMask uint64

// Has reports whether m holds k.
func (m EventMask) Has(k EventKind) bool { return m&(1<<k) != 0 }

func maskOf(kinds ...EventKind) EventMask {
	var m EventMask
	for _, k := range kinds {
		m |= 1 << k
	}
	return m
}

// Event is one report from the platform to the engine.
type Event struct {
	Kind  EventKind
	ID    NodeID
	Text  string
	Count uint32
	Point Point
}

// Mergeable reports whether a later event of the same kind and view
// replaces this one before the engine handles it.
func (e Event) Mergeable() bool { return e.Kind == EventScroll }
