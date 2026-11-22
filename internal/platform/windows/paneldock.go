//go:build windows

package windows

import "github.com/nsgo-dev/nsgo/internal/driver"

func (w *window) placeDockedPanel() bool {
	all := monitorLayout()
	for _, on := range all {
		if w.dock.Display != "" && on.display.ID != w.dock.Display {
			continue
		}
		if w.dock.Display == "" && !on.display.Main {
			continue
		}
		width, height, ok := w.panelPixels(on.monitor)
		if !ok {
			break
		}
		size := driver.Size{Width: driver.Points(float64(width) / on.scale), Height: driver.Points(float64(height) / on.scale)}
		frame, ok := driver.DockWindow(size, []driver.DisplayInfo{on.display}, w.dock)
		if !ok {
			break
		}
		x, y := on.pixel(frame.X, frame.Y)
		w.movePanel(x, y, width, height)
		return true
	}
	w.visible = false
	showWindow.Call(w.hwnd, swHide)
	if w.engine != nil {
		w.engine.Dispatch(driver.Event{Kind: driver.EventWindowPlacementFailed, Text: "the docked panel's display or size is unavailable"})
	}
	return false
}
