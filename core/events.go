package core

import "time"

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
	// EventLoadStart, EventProgress (Loaded, Total), EventLoad (Size, the
	// source's pixels), EventError (Text) and EventLoadEnd follow an image
	// view's loading. A web view reports EventLoadStart and EventLoad with
	// its Navigation, and EventError with the URL in Navigation. A video
	// reports EventLoadStart, EventLoad (Duration and Size, in points),
	// EventProgress (Time and Duration) and EventError.
	EventLoadStart
	EventProgress
	EventLoad
	EventError
	EventLoadEnd
	// EventPressOutside is a press starting outside the overlay ID, at
	// Point in surface points.
	EventPressOutside
	// EventBack is Escape, the back button or gesture, or the browser's
	// back; the engine routes it to the topmost overlay, modal or stack.
	EventBack
	// EventAlert answers ItemAlert Count with the button at Index, or -1.
	EventAlert
	// EventClipboard answers ItemReadClipboard Count with Text.
	EventClipboard
	// EventContextMenu is a secondary click (right click, control click)
	// on ID at Point in the view's coordinates.
	EventContextMenu
	// EventKey is a key press matching a binding of the keymap (Text).
	EventKey
	// EventPan follows an active pan: its phase (Phase), translation
	// (Point) and velocity (Velocity).
	EventPan
	// EventPinch and EventRotation follow a pinch's scale and a rotation's
	// angle in radians (Value) with their phase (Phase) and velocity
	// (Velocity.X).
	EventPinch
	EventRotation
	// EventMenu is an application menu item chosen (Count, its ID).
	EventMenu
	// EventMessage carries what a web view's page posted (Text).
	EventMessage
	// EventNavigationStateChange is a web view's page or history changing
	// (Navigation).
	EventNavigationStateChange
	// EventEnd is a video reaching its end, also when it repeats.
	EventEnd
	// EventBuffer is a video starting or stopping waiting for data
	// (Buffering).
	EventBuffer
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
	Size  Size
	// Loaded and Total are an image's bytes loaded and expected.
	Loaded, Total int64
	Index         int
	// Phase, Velocity and Value are a gesture's.
	Phase    GesturePhase
	Velocity Point
	Value    float32
	// Navigation is a web view's state.
	Navigation *WebNavigation
	// Time, Duration and Buffering are a video's.
	Time, Duration time.Duration
	Buffering      bool
}

// Mergeable reports whether a later event of the same kind and view
// replaces this one before the engine handles it: scrolls, progress and
// a gesture's moves, never its first or last report.
func (e Event) Mergeable() bool {
	switch e.Kind {
	case EventScroll, EventProgress:
		return true
	case EventPan, EventPinch, EventRotation:
		return e.Phase == GestureChanged
	}
	return false
}
