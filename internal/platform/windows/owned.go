//go:build windows

package windows

// surfaceWindow finds an opened window in this app, never an embedding host
// or a window of another engine sharing the thread.
func (w *window) surfaceWindow(id uint32) *window {
	if id == 0 {
		return nil
	}
	for _, other := range windows {
		if other.surfaceID == id && other.appWindow() == w.appWindow() && !other.destroying {
			return other
		}
	}
	return nil
}

func (w *window) parentHandle() uintptr {
	if w.parentWindow != nil {
		return w.parentWindow.hwnd
	}
	return 0
}

// parentVisible also checks ancestors: an app may ask to show a child while
// its owner is hidden or minimized. Keep that intent until the owner returns.
func (w *window) parentVisible() bool {
	for parent := w.parentWindow; parent != nil; parent = parent.parentWindow {
		if parent.hwnd == 0 || parent.destroying || isIconicWindow(parent.hwnd) {
			return false
		}
		if shown, _, _ := isWindowVisible.Call(parent.hwnd); shown == 0 {
			return false
		}
	}
	return true
}

// syncOwnedVisibility supplements native ownership, which keeps stacking and
// follows minimization but does not hide owned windows when the owner hides.
// visible is the app's intent, so a child hidden by the app stays hidden.
func (w *window) syncOwnedVisibility() {
	for _, child := range windows {
		if child.parentWindow != w || child.hwnd == 0 || child.destroying {
			continue
		}
		show := child.visible && child.parentVisible()
		shown, _, _ := isWindowVisible.Call(child.hwnd)
		if show != (shown != 0) {
			mode := uintptr(swHide)
			if show {
				mode = swShowNA
			}
			showWindow.Call(child.hwnd, mode)
		}
		child.syncOwnedVisibility()
	}
}
