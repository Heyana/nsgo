//go:build android

package android

import "github.com/nsgo-dev/nsgo/internal/driver"

func (s *surface) ObserveGeometry(report func(driver.GeometrySnapshot)) {
	s.observeGeometry(0, report)
}

func (o *overlay) ObserveGeometry(report func(driver.GeometrySnapshot)) {
	o.s.observeGeometry(o.view.id, report)
}

func (s *surface) observeGeometry(surface uint32, report func(driver.GeometrySnapshot)) {
	s.geometryMu.Lock()
	if s.geometryReports == nil && report != nil {
		s.geometryReports = map[uint32]func(driver.GeometrySnapshot){}
	}
	if report == nil {
		delete(s.geometryReports, surface)
	} else {
		s.geometryReports[surface] = report
	}
	s.geometryMu.Unlock()
	s.send(msgGeometry, func(w *writer) {
		w.u32(surface)
		w.flags(report != nil)
	})
}

func (s *surface) receiveGeometry(payload []byte) {
	surface, snapshot, ok := readGeometrySnapshot(payload)
	if !ok {
		return
	}
	s.geometryMu.Lock()
	report := s.geometryReports[surface]
	s.geometryMu.Unlock()
	if report != nil {
		report(snapshot)
	}
}
