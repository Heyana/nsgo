//go:build ios

package ios

import (
	"github.com/ebitengine/purego/objc"

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
	if apple.ReportLayerGeometry(&s.devGeometry, s.native.Send(selLayer), s.nativeViews) {
		s.startTicking()
	}
}

func (s *surface) geometryAnimating() bool {
	return apple.LayersAnimating(&s.devGeometry, s.nativeViews)
}

// nativeViews yields the surface's mounted views for the geometry helpers apple shares.
func (s *surface) nativeViews(yield func(driver.NodeID, objc.ID) bool) {
	for _, v := range s.views {
		if v != nil && v.native != 0 && !yield(v.id, v.native) {
			return
		}
	}
}
