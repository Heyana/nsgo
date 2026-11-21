package scrollbar

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// OffsetAnimation is a scroll toward an offset. A user snap ends momentum
// when it finishes. The toolkit chooses the timing curve.
type OffsetAnimation struct {
	From, To driver.Point
	Start    time.Time
	Momentum bool
}

// RebaseOffset carries a scroll or thumb drag along with an unclamped
// ItemScrollBy, so a change above the viewport preserves the content in view.
func RebaseOffset(config *driver.ScrollConfig, offset, delta driver.Point, moving *OffsetAnimation, drag *Drag) driver.Point {
	if moving != nil {
		moving.From = shiftOffset(moving.From, delta)
		moving.To = shiftOffset(moving.To, delta)
	}
	if drag != nil {
		drag.Shift(Along(config, delta))
	}
	return shiftOffset(offset, delta)
}

func shiftOffset(offset, delta driver.Point) driver.Point {
	return driver.Point{X: offset.X + delta.X, Y: offset.Y + delta.Y}
}
