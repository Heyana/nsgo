package core

// Gestures the platform recognizes on a view and reports as they happen:
// drags (pans), pinches and rotations. Moves are merged per engine turn,
// so a handler sees the latest state, never a backlog.

// GesturePhase is where a gesture is in its course.
type GesturePhase uint8

const (
	// GestureBegan is the first report, once the platform recognized it.
	GestureBegan GesturePhase = iota + 1
	// GestureChanged follows each move.
	GestureChanged
	// GestureEnded is the last report of a gesture that completed.
	GestureEnded
	// GestureCancelled is the last report of one the system took away,
	// such as by a scroll view or an incoming call.
	GestureCancelled
)

// PanEvent is a drag: its translation since it began and its velocity,
// in points and points per second.
type PanEvent struct {
	Phase       GesturePhase
	Translation Point
	Velocity    Point
	// Values are the PanValues as the pan begins and ends: a drag may grab
	// them mid animation, which a snap after it starts from.
	Values Point
}

// GestureEvent is a pinch or a rotation: its amount since it began and
// how fast that changes per second. A pinch's amount is its scale, 1 when
// it begins; a rotation's is an angle in radians, clockwise.
type GestureEvent struct {
	Phase    GesturePhase
	Value    float32
	Velocity float32
}

// PanActivation decides on the main thread when a drag on a view becomes
// its pan, so it can share touches with the scroll views around it, as
// react-native-gesture-handler's offsets do. A fail offset passed first
// leaves the touch to others; an active offset passed activates the pan.
// With no active offset the pan activates after 10 points either way.
// Zero fields are unset.
type PanActivation struct {
	ActiveOffsetX, ActiveOffsetY Points
	FailOffsetX, FailOffsetY     Points
}

// DefaultPanSlop is the distance a drag moves before it activates a pan
// without active offsets.
const DefaultPanSlop Points = 10

// Decide returns whether a drag translated by d activates the pan (1),
// fails it (-1) or is undecided (0). Platforms call it on each move until
// it decides.
func (a PanActivation) Decide(d Point) int {
	past := func(v, limit Points) bool { return limit != 0 && abs(v) >= abs(limit) }
	if past(d.X, a.FailOffsetX) || past(d.Y, a.FailOffsetY) {
		return -1
	}
	if a.ActiveOffsetX == 0 && a.ActiveOffsetY == 0 {
		if abs(d.X) >= DefaultPanSlop || abs(d.Y) >= DefaultPanSlop {
			return 1
		}
		return 0
	}
	if past(d.X, a.ActiveOffsetX) || past(d.Y, a.ActiveOffsetY) {
		return 1
	}
	return 0
}

func abs(v Points) Points {
	if v < 0 {
		return -v
	}
	return v
}
