//go:build windows

package windows

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
		g := driver.ViewGeometry{Frame: frameOf(v), Matrix: v.pointerMatrix(), Offset: v.offset}
		if v.parent != nil {
			g.Parent = v.parent.id
		}
		views[v.id] = g
	}
	w.devGeometry.Notify(driver.ProjectGeometry(views))
}
