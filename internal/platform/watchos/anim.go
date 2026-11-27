//go:build ios

package watchos

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// setFrame applies a frame to v, moving to it with the batch's layout
// animation when v existed before the batch, and updates field and image
// layouts to match.
func (s *surface) setFrame(v *view, frame driver.Bounds, lb *layoutBatch, now time.Time) {
	s.setNodeFrame(v.watchNode, frame, lb, now)
	if v.kind == driver.KindInput {
		s.relayoutField(v)
	}
	if v.image != nil {
		v.image.Layout(apple.CGSize{Width: float64(frame.Size.Width), Height: float64(frame.Size.Height)})
	}
}

// stepMotions advances style transitions, keyframes, layout frame motions,
// entrances and leaving-ghost exits to now.
func (s *surface) stepMotions() {
	s.stepNodes(time.Now(), s.destroyPayload)
}
