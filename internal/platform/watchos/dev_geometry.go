//go:build ios

package watchos

import (
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
	views := make(map[driver.NodeID]driver.ViewGeometry, len(s.views))
	for _, v := range s.views {
		if v == nil || v.deleted {
			continue
		}
		g := driver.ViewGeometry{Frame: v.shownFrame(), Matrix: v.pointerMatrix(), Offset: v.offset}
		if v.parent != nil {
			g.Parent = v.parent.id
		}
		views[v.id] = g
	}
	s.devGeometry.Notify(driver.ProjectGeometry(views))
}
