//go:build darwin && !ios

package macos

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
	root := s.native.Send(selLayer)
	views := make(map[driver.NodeID]driver.ViewGeometry, len(s.views))
	for _, v := range s.views {
		if v != nil && v.native != 0 {
			if m, size, ok := apple.LayerGeometry(v.native.Send(selLayer), root); ok {
				views[v.id] = driver.ViewGeometry{Frame: driver.Bounds{Size: size}, Matrix: m}
			}
		}
	}
	s.devGeometry.Notify(driver.ProjectGeometry(views))
	if s.geometryAnimating() {
		s.startTicking()
	}
}

func (s *surface) geometryAnimating() bool {
	if !s.devGeometry.Active() {
		return false
	}
	for _, v := range s.views {
		if v != nil && v.native != 0 && apple.LayerGeometryAnimating(v.native.Send(selLayer)) {
			return true
		}
	}
	return false
}

func (s *surface) geometryEnding() bool {
	for _, v := range s.views {
		if v != nil && v.native != 0 && apple.LayerGeometryEnding(v.native.Send(selLayer)) {
			return true
		}
	}
	return false
}
