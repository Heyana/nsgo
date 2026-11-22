//go:build linux && !android

package linux

// surfaceWindow finds an opened window in this app. IDs are local to the
// engine, and several engines can share GTK's main thread.
func (w *window) surfaceWindow(id uint32) *window {
	if id == 0 {
		return nil
	}
	app := w
	if w.opener != nil {
		app = w.opener
	}
	var found *window
	windowHandles.Range(func(_, value any) bool {
		other := value.(*window)
		if other.surfaceID == id && other.opener == app && !other.closing {
			found = other
		}
		return found == nil
	})
	return found
}

// parentVisible excludes minimized owners, but not owners merely covered or
// on another workspace: their compositor manages their transient windows.
func (w *window) parentVisible() bool {
	for parent := w.parentWindow; parent != nil; parent = parent.parentWindow {
		if parent.win == 0 || parent.closing || gtkWidgetGetVisible(parent.win) == 0 {
			return false
		}
		if top := gtkNativeGetSurface(parent.win); top != 0 && gdkToplevelGetState(top)&toplevelMinimized != 0 {
			return false
		}
	}
	return true
}

// syncOwnedVisibility keeps the app's requested visibility separate from a
// transient window temporarily hidden with its owner. Map/unmap and GDK
// toplevel-state notifications cover app commands and window-manager changes.
func (w *window) syncOwnedVisibility() {
	windowHandles.Range(func(_, value any) bool {
		child := value.(*window)
		if child.parentWindow != w || child.win == 0 || child.closing {
			return true
		}
		show := child.visible && child.parentVisible()
		if show != (gtkWidgetGetVisible(child.win) != 0) {
			if show {
				gtkWidgetSetVisible(child.win, 1)
			} else {
				gtkWidgetSetVisible(child.win, 0)
			}
		}
		child.syncOwnedVisibility()
		return true
	})
}
