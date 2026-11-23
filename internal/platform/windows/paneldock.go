//go:build windows

package windows

import "github.com/nsgo-dev/nsgo/internal/driver"

func (w *window) placeDockedPanel() bool {
	w.cancelPanelFrame()
	if r, ok := w.dockedPanelFrame(0, 0); ok {
		w.movePanel(r.Left, r.Top, r.Right-r.Left, r.Bottom-r.Top)
		return true
	}
	w.dockPlacementFailed()
	return false
}

// dockedPanelFrame puts the requested size on the dock's monitor at its scale.
// Without a new size it uses the panel's current size in points, as on show.
func (w *window) dockedPanelFrame(width, height driver.Points) (rect, bool) {
	all := monitorLayout()
	for _, on := range all {
		if w.dock.Display != "" && on.display.ID != w.dock.Display {
			continue
		}
		if w.dock.Display == "" && !on.display.Main {
			continue
		}
		var cx, cy int32
		if width > 0 && height > 0 {
			cx, cy = on.px(width), on.px(height)
		} else {
			var ok bool
			cx, cy, ok = w.panelPixels(on.monitor)
			if !ok {
				break
			}
		}
		size := driver.Size{Width: driver.Points(float64(cx) / on.scale), Height: driver.Points(float64(cy) / on.scale)}
		frame, ok := driver.DockWindow(size, []driver.DisplayInfo{on.display}, w.dock)
		if !ok {
			break
		}
		x, y := on.pixel(frame.X, frame.Y)
		return rect{x, y, x + cx, y + cy}, true
	}
	return rect{}, false
}

func (w *window) dockPlacementFailed() {
	w.visible = false
	showWindow.Call(w.hwnd, swHide)
	if w.engine != nil {
		w.engine.Dispatch(driver.Event{Kind: driver.EventWindowPlacementFailed, Text: "the docked panel's display or size is unavailable"})
	}
}
