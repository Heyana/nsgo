package nsgotest

import (
	"maps"
	"slices"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Event is a platform report a test sends with App.Report. The report copies
// its slices, navigation state and touch geometry; Data keeps the test's payload.
type Event struct {
	Kind  EventKind
	ID    core.NodeID
	Text  string
	Count uint32
	Point core.Point
	// Page is a pointer's position in the root view, the surface or the
	// modal, and Modifiers the keys held.
	Page      core.Point
	Modifiers core.Modifiers
	// Button is the mouse button a mouse event holds down.
	Button core.MouseButton
	Size   core.Size
	// Loaded and Total are an image's bytes loaded and expected.
	Loaded, Total int64
	Index         int
	// Phase, Velocity and Value are a gesture's.
	Phase    core.GesturePhase
	Velocity core.Point
	Value    float32
	// Position and PagePosition are the current point of a pan in its view
	// and page coordinates. Point remains the pan's translation.
	Position, PagePosition core.Point
	// Navigation is a web view's state.
	Navigation *core.WebNavigation
	// Origin and URL identify the frame that posted a web view's message;
	// MainFrame says whether it is the page's top frame.
	Origin    string
	URL       string
	MainFrame bool
	// Time, Duration, Buffering and RateChangeable are a video's.
	Time, Duration time.Duration
	Buffering      bool
	RateChangeable bool
	// URIs are the file: URLs of dropped files.
	URIs []string
	// Data is what a native component emitted.
	Data any
	// Ranges are what each text of a selection area has selected, in
	// document order (EventSelectionChange), nil on a platform that
	// does not report them.
	Ranges []SelectedRange
	// Repeat is whether EventKeyDown's key is a held key's auto-repeat.
	Repeat bool
	// Forced is whether EventQuitRequest was issued by the system to end its session.
	Forced bool
	// Finished is whether an animation ran to its end, or the platform
	// closed a modal.
	Finished bool
	// Insets are what system UI covers of a modal (EventModalSize).
	Insets core.Edges
	// Touches are EventTouch's fingers.
	Touches []core.Touch
	// TouchGeometry is the platform's presentation snapshot for these
	// touches. Without one, the committed layout and static transforms apply.
	TouchGeometry *TouchGeometry
	// Values are the values a pan moves as it begins and ends.
	Values core.Point
	// Delta and Precise are a wheel's (EventWheel).
	Delta   core.Point
	Precise bool
}

// TouchGeometry captures native hit targets and local positions for Event.Touches.
// Positions use the same order as the event's touches.
type TouchGeometry struct {
	Targets   map[int]core.NodeID
	Positions map[core.NodeID][]core.Point
}

func (g *TouchGeometry) driverGeometry() *driver.TouchGeometry {
	if g == nil {
		return nil
	}
	copy := &driver.TouchGeometry{Targets: maps.Clone(g.Targets)}
	if g.Positions != nil {
		copy.Positions = make(map[core.NodeID][]core.Point, len(g.Positions))
		for id, points := range g.Positions {
			copy.Positions[id] = slices.Clone(points)
		}
	}
	return copy
}

func (e Event) driverEvent() driver.Event {
	var navigation *core.WebNavigation
	if e.Navigation != nil {
		copy := *e.Navigation
		navigation = &copy
	}
	return driver.Event{
		Kind:           e.Kind,
		ID:             e.ID,
		Text:           e.Text,
		Count:          e.Count,
		Point:          e.Point,
		Page:           e.Page,
		Modifiers:      e.Modifiers,
		Button:         e.Button,
		Size:           e.Size,
		Loaded:         e.Loaded,
		Total:          e.Total,
		Index:          e.Index,
		Phase:          e.Phase,
		Velocity:       e.Velocity,
		Value:          e.Value,
		Position:       e.Position,
		PagePosition:   e.PagePosition,
		Navigation:     navigation,
		Origin:         e.Origin,
		URL:            e.URL,
		MainFrame:      e.MainFrame,
		Time:           e.Time,
		Duration:       e.Duration,
		Buffering:      e.Buffering,
		RateChangeable: e.RateChangeable,
		URIs:           slices.Clone(e.URIs),
		Data:           e.Data,
		Ranges:         slices.Clone(e.Ranges),
		Repeat:         e.Repeat,
		Forced:         e.Forced,
		Finished:       e.Finished,
		Insets:         e.Insets,
		Touches:        slices.Clone(e.Touches),
		TouchGeometry:  e.TouchGeometry.driverGeometry(),
		Values:         e.Values,
		Delta:          e.Delta,
		Precise:        e.Precise,
	}
}
