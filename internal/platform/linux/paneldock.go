//go:build linux && !android

package linux

import (
	"fmt"
	"math"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// setupDock assigns the Wayland role before GTK first realizes the panel.
func (w *window) setupDock() {
	if w.dock.Edge == driver.WindowPlaceDefault || drawsOnX11() {
		return
	}
	loaded := loadLayerShell()
	if loaded.api == nil {
		w.dockIssue = loaded.issue
		return
	}
	api := loaded.api
	if api.supported() == 0 {
		w.dockIssue = "the Wayland compositor does not support layer-shell panel docking"
		return
	}
	api.initWindow(w.win)
	if api.isLayerWindow(w.win) == 0 {
		w.dockIssue = "GTK could not assign the panel's layer-shell role"
		return
	}
	w.dockLayer = true
	name := w.appID
	if name == "" {
		name = "nsgo"
	}
	api.setNamespace(w.win, cstr(name+".panel"))
	api.setLayer(w.win, 2) // GTK_LAYER_SHELL_LAYER_TOP
	edge := int32(2)
	switch w.dock.Edge {
	case driver.WindowPlaceBottom:
		edge = 3
	case driver.WindowPlaceLeft:
		edge = 0
	case driver.WindowPlaceRight:
		edge = 1
	}
	api.setAnchor(w.win, edge, 1)
	api.setExclusiveZone(w.win, 0) // A popup panel reserves no desktop space.
	mode := int32(1)               // EXCLUSIVE: a focusable layer keeps the keyboard until hidden.
	if w.noActivate {
		mode = 0
	}
	api.setKeyboardMode(w.win, mode)
	if api.setRespectClose != nil {
		api.setRespectClose(w.win, 1)
	}
}

func dockMonitor(id string) uintptr {
	display := gdkDisplayGetDefault()
	if display == 0 {
		return 0
	}
	model := gdkDisplayGetMonitors(display)
	if model == 0 {
		return 0
	}
	for i := uint32(0); i < gListModelGetNItems(model); i++ {
		monitor := gListModelGetItem(model, i)
		if monitor == 0 {
			continue
		}
		name := ""
		if connector := gdkMonitorGetConnector(monitor); connector != 0 {
			name = goString(connector)
		}
		if name == "" {
			name = fmt.Sprintf("%x", monitor)
		}
		if id == name || id == "" && i == 0 {
			return monitor
		}
		gObjectUnref(monitor)
	}
	return 0
}

func (w *window) showDockedPanel() {
	if w.dockIssue != "" {
		w.dockFailed(w.dockIssue)
		return
	}
	if !w.placeDockedPanel() {
		return
	}
	w.visible = true
	w.applyFit()
	gtkWidgetSetVisible(w.win, 1)
	if !w.noActivate {
		gtkWindowPresent(w.win)
	}
	if !w.dockLayer && !w.unmanaged {
		w.placeDockedPanel()
	}
}

func (w *window) placeDockedPanel() bool {
	if w.dockLayer {
		monitor := dockMonitor(w.dock.Display)
		if monitor == 0 {
			w.dockFailed("the docked panel's display is unavailable")
			return false
		}
		defer gObjectUnref(monitor)
		api := loadLayerShell().api
		api.setMonitor(w.win, monitor)
		var geometry gdkMonitorRectangle
		gdkMonitorGetGeometry(monitor, &geometry)
		var width, height int32
		gtkWindowGetDefaultSize(w.win, &width, &height)
		edge, limit := int32(2), geometry.Height-height
		switch w.dock.Edge {
		case driver.WindowPlaceBottom:
			edge = 3
		case driver.WindowPlaceLeft:
			edge, limit = 0, geometry.Width-width
		case driver.WindowPlaceRight:
			edge, limit = 1, geometry.Width-width
		}
		margin := min(max(w.dock.Margin, 0), driver.Points(max(limit, 0)))
		api.setMargin(w.win, edge, int32(math.Ceil(float64(margin))))
		return true
	}
	display, xid, ok := w.x11Window()
	if !ok {
		w.dockFailed("the display cannot place an edge-docked panel")
		return false
	}
	var width, height int32
	gtkWindowGetDefaultSize(w.win, &width, &height)
	frame, ok := driver.DockWindow(driver.Size{Width: driver.Points(width), Height: driver.Points(height)}, displays(), w.dock)
	if !ok {
		w.dockFailed("the docked panel's display is unavailable")
		return false
	}
	scale := float64(max(gtkWidgetGetScaleFactor(w.win), 1))
	x11MoveWindow(display, xid, int32(math.Round(float64(frame.X)*scale)), int32(math.Round(float64(frame.Y)*scale)))
	x11Flush(display)
	return true
}

func (w *window) dockFailed(message string) {
	w.visible = false
	gtkWidgetSetVisible(w.win, 0)
	if w.engine != nil {
		w.engine.Dispatch(driver.Event{Kind: driver.EventWindowPlacementFailed, Text: message})
	}
}
