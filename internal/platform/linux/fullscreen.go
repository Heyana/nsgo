//go:build linux && !android

package linux

type fullScreenRequest struct {
	on        bool
	displayID string
}

func (w *window) toggleFullScreen() {
	on := w.chrome.fullScreen
	if requested := w.chrome.fullScreenRequest; requested != nil {
		on = requested.on
	}
	w.setFullScreen(!on, "")
}

// setFullScreen requests a state from the window manager. GtkWindow's
// fullscreened notification reports when that state actually changes.
func (w *window) setFullScreen(on bool, displayID string) {
	if !w.ownWindow() {
		return
	}
	c := &w.chrome
	request := fullScreenRequest{on, displayID}
	pending := c.fullScreenRequest
	if pending != nil && *pending == request {
		return
	}
	if !on {
		if c.fullScreen || pending != nil {
			c.fullScreenRequest = &request
			gtkWindowUnfullscreen(w.win)
		}
		return
	}
	if displayID == "" {
		if !c.fullScreen || pending != nil {
			c.fullScreenRequest = &request
			gtkWindowFullscreen(w.win)
		}
		return
	}
	display := gdkDisplayGetDefault()
	if display == 0 {
		return
	}
	monitor := findFullScreenMonitor(display, displayID)
	if monitor == 0 {
		return
	}
	defer gObjectUnref(monitor)
	if c.fullScreen && pending == nil {
		if surface := gtkNativeGetSurface(w.win); surface != 0 && gdkDisplayGetMonitorAtSurface(display, surface) == monitor {
			return
		}
	}
	c.fullScreenRequest = &request
	gtkWindowFullscreenOnMonitor(w.win, monitor)
}

// findFullScreenMonitor returns a reference to the monitor Env.Displays
// named id. The caller releases it after GTK takes the request.
func findFullScreenMonitor(display uintptr, id string) uintptr {
	model := gdkDisplayGetMonitors(display)
	if model == 0 {
		return 0
	}
	for i, count := uint32(0), gListModelGetNItems(model); i < count; i++ {
		monitor := gListModelGetItem(model, i)
		if monitor == 0 {
			continue
		}
		if monitorDisplayID(monitor) == id {
			return monitor
		}
		gObjectUnref(monitor)
	}
	return 0
}
