//go:build ios

package ios

import (
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func (s *surface) setSystemGestureEdges(edges driver.SystemGestureEdge) {
	if s.gestureEdges == edges {
		return
	}
	s.gestureEdges = edges
	update := sel("setNeedsUpdateOfScreenEdgesDeferringSystemGestures")
	if c := s.viewController(); c != 0 && objc.Send[bool](c, sel("respondsToSelector:"), update) {
		c.Send(update)
	}
	for c, v := range sheets {
		if v.surface == s && objc.Send[bool](c, sel("respondsToSelector:"), update) {
			c.Send(update)
		}
	}
}
