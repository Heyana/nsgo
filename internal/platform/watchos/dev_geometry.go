//go:build ios

package watchos

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func (s *surface) ObserveGeometry(report func(driver.GeometrySnapshot)) {
	apple.OnMain(func() {
		s.devGeometry.Observe(report)
		s.reportGeometry()
	})
}

func (s *surface) reportGeometry() {
	if !s.devGeometry.Active() {
		return
	}
	now := time.Now()
	views := make(map[driver.NodeID]driver.ViewGeometry, len(s.views))
	for _, n := range s.views {
		if n == nil || n.deleted {
			continue
		}
		g := driver.ViewGeometry{Frame: s.placedFrame(n), Matrix: n.pointerMatrixAt(now), Offset: n.offset}
		if n.parent != nil {
			g.Parent = n.parent.id
		}
		views[n.id] = g
	}
	s.devGeometry.Notify(driver.ProjectGeometry(views))
}
