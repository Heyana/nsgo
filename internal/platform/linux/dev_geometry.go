//go:build linux && !android

package linux

import "github.com/nsgo-dev/nsgo/internal/driver"

func (w *window) ObserveGeometry(report func(driver.GeometrySnapshot)) {
	w.post(func() {
		w.devGeometry.Observe(report)
		w.reportGeometry()
	})
}

func (w *window) reportGeometry() {
	if !w.devGeometry.Active() {
		return
	}
	views := make(map[driver.NodeID]driver.ViewGeometry, len(w.views))
	for _, v := range w.views {
		if v == nil || v.deleted {
			continue
		}
		var ox, oy float32
		var parent driver.NodeID
		if v.parent != nil {
			ox, oy, parent = float32(v.parent.offset.X), float32(v.parent.offset.Y), v.parent.id
		}
		// placement is exactly the matrix passed to GTK's allocation, with the shown layout frame, sticky shift, transition and keyframe.
		views[v.id] = driver.ViewGeometry{Parent: parent, Frame: driver.Bounds{Size: v.shownFrame().Size}, Matrix: v.placement(ox, oy)}
	}
	w.devGeometry.Notify(driver.ProjectGeometry(views))
}
