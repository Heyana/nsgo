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
	// EventScroll carries a scroll view's offset (Point), the
	// ItemScrollTo and ItemScrollBy items it applied since it was made
	// (Count) and, when it zooms, its zoom scale (Value).
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
	// With the ID of a screen it is that screen's header's back button or
	// swipe, which goes to the screen's stack while the screen is on top.
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
	// EventFileDragEnter and EventFileDragLeave follow files dragged over
	// the view that would receive them, and EventFileDrop delivers them
	// (URIs).
	EventFileDragEnter
	EventFileDragLeave
	EventFileDrop
	// EventNative is an event a native view's component emitted: its
	// name (Text) and Data.
	EventNative
	// EventNativeSize is the size a native view's component asks for
	// (Size).
	EventNativeSize
	// EventAnimation reports an animation's end or a value read (Count,
	// the request), with Finished and the value's Value.
	EventAnimation
	// EventScrollDragStart and EventScrollDragEnd bracket the user
	// dragging a scroll view, its content or its scroll bar, and
	// EventMomentumStart and EventMomentumEnd the glide or snap after;
	// each carries the offset (Point), the scrolls applied (Count) and
	// zoom scale (Value) as EventScroll does.
	EventScrollDragStart
	EventScrollDragEnd
	EventMomentumStart
	EventMomentumEnd
	// EventRefresh is the user pulling a refreshable scroll view past
	// its top; the platform shows its spinner until a config says else.
	EventRefresh
	// EventImageInfo answers ItemImageInfo Count with the image's Size in
	// pixels, or the error in Text.
	EventImageInfo
	// EventSelectionChange carries a text field's selection after the
	// user moved it: Index to Count, in UTF-16 units.
	EventSelectionChange
	// EventKeyPress is a key typed in a text field (Text: what it enters,
	// "Backspace" or "Enter").
	EventKeyPress
	// EventCompositionChange follows an input method's composition in a
	// text field: the text being composed (Text) as it starts and
	// changes, then its end (Finished), after the EventChangeText of the
	// text it commits.
	EventCompositionChange
	// EventAccessibilityAction is an action assistive technology
	// performed on ID, by name (Text).
	EventAccessibilityAction
	// EventRequestClose asks to close the modal ID for the reason Text
	// ("swipe" for an iOS sheet); Finished says the platform closed it.
	EventRequestClose
	// EventModalSize is the Size and Insets of a modal the platform
	// presents at a size of its own, such as an iOS sheet.
	EventModalSize
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
	// URIs are the file: URLs of dropped files.
	URIs []string
	// Data is what a native component emitted.
	Data any
	// Finished is whether an animation ran to its end, or the platform
	// closed a modal.
	Finished bool
	// Insets are what system UI covers of a modal (EventModalSize).
	Insets Edges
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
