//go:build windows

package windows

import "unsafe"

// A transparent window is a popup without a frame, as NSH's is: a system frame around see-through content would draw a box nothing is in, and its system menu stays for the taskbar's menu. It draws through DirectComposition with alpha, as a window with a material does (material.go), clearing to nothing where the app draws nothing, and DWM draws it bare: no shadow and square corners. Only a window opened transparent has the composition, which WS_EX_NOREDIRECTIONBITMAP holds from the window's creation, so only such a window changes between transparent and framed later, clearing to white while framed, and only its environment reports CanMakeWindowsTransparent. A window with the system's title bar keeps its frame and the shadow DWM casts with it, NoShadow or not: DWM draws that shadow with the frame's non-client rendering, which has no switch of its own, and turning the rendering off would trade the Windows 11 frame for the classic one DefWindowProc draws. A window with a hidden title bar draws its own chrome, so with NoShadow it is shadowless: it has no non-client area at all, its client covering its whole frame with the resize edges hit-tested in it (chrome.go), and DWM's non-client rendering is off. A transparent panel (SurfaceConfig.Transparent) opens the same way, a popup whose content draws its own corners and shadow, and stays so; without DirectComposition it stays opaque, as its environment's WindowTransparent says.
//
// A window off the taskbar is a tool window, which neither the taskbar nor Alt+Tab lists; a framed one then shows a tool window's smaller caption. The taskbar adds or deletes the button of a window that shows as the style changes (ITaskbarList).

const (
	dwmwaNCRenderingPolicy      = 2
	dwmwaWindowCornerPreference = 33
	dwmNCRPUseWindowStyle       = 0
	dwmNCRPDisabled             = 1
	dwmCornerDefault            = 0
	dwmCornerDoNotRound         = 1

	taskbarAddTab    = 4
	taskbarDeleteTab = 5
)

// composeTransparent has a window opening transparent draw through DirectComposition, where the system has it; else it opens framed and opaque, and its environment says so (CanMakeWindowsTransparent).
func (w *window) composeTransparent() {
	if !compositionAvailable() {
		return
	}
	w.baseComposed, w.transparent, w.canTransparent = true, true, true
	if w.backdrop == 0 {
		w.backdrop = backdropNone
	}
}

// frameStyle is current with a transparent window's frame bits, the popup's, or a framed window's within its constraints.
func (w *window) frameStyle(current uintptr) uintptr {
	current &^= wsOverlappedWindow | wsPopup
	if !w.transparent {
		return constrainedStyle(current|wsOverlappedWindow, w.chrome.constraints)
	}
	style := current | wsPopup | wsSysMenu | wsMinimizeBox
	if w.chrome.constraints.NoMinimize {
		style &^= wsMinimizeBox
	}
	return style
}

// shadowless reports whether the window draws without DWM's shadow though it is not transparent: NoShadow on a window whose title bar is hidden, out of full screen.
func (w *window) shadowless() bool {
	return w.opts.NoShadow && w.hidden()
}

// applyBare has DWM draw a transparent window without a shadow or rounded corners, a shadowless one without its non-client rendering, or a framed one as its style says.
func (w *window) applyBare() {
	policy, corners := int32(dwmNCRPUseWindowStyle), int32(dwmCornerDefault)
	switch {
	case w.transparent:
		policy, corners = dwmNCRPDisabled, dwmCornerDoNotRound
	case w.shadowless():
		policy = dwmNCRPDisabled
	}
	// Each fails quietly where the system lacks it.
	dwmSetWindowAttribute.Call(w.hwnd, dwmwaNCRenderingPolicy, uintptr(unsafe.Pointer(&policy)), 4)
	dwmSetWindowAttribute.Call(w.hwnd, dwmwaWindowCornerPreference, uintptr(unsafe.Pointer(&corners)), 4)
}

// setShadow keeps DWM's shadow around the window, or takes it away where the window draws its own chrome (shadowless), which it does as its title bar hides too.
func (w *window) setShadow(on bool) {
	if w.opts.NoShadow == !on {
		return
	}
	w.opts.NoShadow = !on
	w.frameChanged()
}

// setTransparent changes a window opened transparent between transparent and framed, keeping its content's size and place; others stay as they are.
func (w *window) setTransparent(on bool) {
	if !w.canTransparent || on == w.transparent {
		return
	}
	w.transparent = on
	w.clearCaptionState()
	w.applyBare()
	if w.chrome.fullScreen {
		w.chrome.savedStyle = w.frameStyle(w.chrome.savedStyle)
		w.chromeChanged()
		return
	}
	var client rect
	getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
	at := point{}
	clientToScreen.Call(w.hwnd, uintptr(unsafe.Pointer(&at)))
	current, _, _ := getWindowLongPtrW.Call(w.hwnd, gwlStyle)
	style := w.frameStyle(current)
	setWindowLongPtrW.Call(w.hwnd, gwlStyle, style)
	w.frameChanged()
	if !w.zoomed() {
		// What the new frame takes is measured, as placeNew does, rather than AdjustWindowRect's: that misses the hidden title bar's caption in the client area (ncCalcSize), a tool window's smaller caption and the monitor's DPI.
		var outer, inner rect
		getWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&outer)))
		getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&inner)))
		origin := point{}
		clientToScreen.Call(w.hwnd, uintptr(unsafe.Pointer(&origin)))
		cx := outer.Right - outer.Left - inner.Right + client.Right
		cy := outer.Bottom - outer.Top - inner.Bottom + client.Bottom
		x, y := at.X-(origin.X-outer.Left), at.Y-(origin.Y-outer.Top)
		setWindowPos.Call(w.hwnd, 0, uintptr(x), uintptr(y), uintptr(cx), uintptr(cy), swpNoZOrder|swpNoActivate)
	}
	w.chromeChanged()
}

// setSkipTaskbar makes the window a tool window, off the taskbar and Alt+Tab, or an app window again.
func (w *window) setSkipTaskbar(on bool) {
	ex, _, _ := getWindowLongPtrW.Call(w.hwnd, gwlExStyle)
	next := ex &^ wsExToolWindow
	if on {
		next |= wsExToolWindow
	}
	if next == ex {
		return
	}
	setWindowLongPtrW.Call(w.hwnd, gwlExStyle, next)
	w.frameChanged()
	if visible, _, _ := isWindowVisible.Call(w.hwnd); visible == 0 {
		return
	}
	if list := taskbar(); list != nil {
		if on {
			list.call(taskbarDeleteTab, w.hwnd)
		} else {
			list.call(taskbarAddTab, w.hwnd)
		}
	}
}
