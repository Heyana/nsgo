//go:build windows

package windows

// Where a window takes the pointer (driver.WindowPointerEvents), as NSH's click-through windows: a layered window with WS_EX_TRANSPARENT passes every press, wheel turn and move to the window below, of any process; HTTRANSPARENT from WM_NCHITTEST would pass them only to the windows of its own thread. A window that takes the pointer where it draws toggles WS_EX_TRANSPARENT from a hit test of the views under the pointer, which a timer makes at the display's pace, as a window that lets the pointer through hears no moves.

import (
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	setLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	getAsyncKeyState           = user32.NewProc("GetAsyncKeyState")
)

const (
	gwlExStyle      = ^uintptr(19) // -20
	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	lwaAlpha        = 0x2
	// passTimer follows the pointer over a window that takes it where it draws, every passMillis.
	passTimer  = 5
	passMillis = 16
	vkRButton  = 0x02
	vkMButton  = 0x04
)

// setPointerEvents makes the window take the pointer as mode says, from now.
func (w *window) setPointerEvents(mode driver.WindowPointerEvents) {
	if w.hwnd == 0 || w.embedded {
		return
	}
	w.pointerMode = mode
	switch mode {
	case driver.WindowPointerNone:
		killTimer.Call(w.hwnd, passTimer)
		w.passPointer(true)
	case driver.WindowPointerPainted:
		setTimer.Call(w.hwnd, passTimer, passMillis, 0)
		w.passUnpainted()
	default:
		killTimer.Call(w.hwnd, passTimer)
		w.passPointer(false)
	}
}

// passPointer lets the pointer through the window, or takes it again. The window is layered, at full opacity since a layered window draws nothing until it is given one, while its mode lets the pointer through anywhere, so a painted window does not leave and enter the layered redirection at each toggle; it leaves it as it takes the pointer everywhere again, as NSH's click-through windows do.
func (w *window) passPointer(on bool) {
	ex, _, _ := getWindowLongPtrW.Call(w.hwnd, gwlExStyle)
	next := ex &^ (wsExLayered | wsExTransparent)
	if w.pointerMode != driver.WindowPointerAuto {
		next |= wsExLayered
	}
	if on {
		next |= wsExTransparent
	}
	if next == ex {
		return
	}
	setWindowLongPtrW.Call(w.hwnd, gwlExStyle, next)
	if next&wsExLayered != 0 && ex&wsExLayered == 0 {
		setLayeredWindowAttributes.Call(w.hwnd, 0, 255, lwaAlpha)
	}
	setWindowPos.Call(w.hwnd, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
}

// passUnpainted lets the pointer through while it is over nothing painted, by the hit test of the views under it. A button held keeps what the window does, so a drag that began on a view goes on to its end and one from another app passes over the window.
func (w *window) passUnpainted() {
	if !w.canPassUnpainted() {
		return
	}
	if r, _, _ := isWindowVisible.Call(w.hwnd); r == 0 {
		return
	}
	at, ok := w.pointerScreenPosition()
	if !ok {
		return
	}
	screenToClient.Call(w.hwnd, uintptr(unsafe.Pointer(&at)))
	w.passPointer(!w.paintedAt(w.point(at.X, at.Y)))
}

func (w *window) canPassUnpainted() bool {
	if w.pointerMode != driver.WindowPointerPainted {
		return false
	}
	if w.kind == driver.SurfacePanel && w.opts.test != nil {
		return w.held&(mkLButton|mkRButton|mkMButton|mkXButton1|mkXButton2) == 0
	}
	return !mouseButtonDown()
}

// paintedAt reports whether a view under p, in the window's points, takes the pointer in a painted window: the innermost view there, or one of the views it is in (driver.Painted).
func (w *window) paintedAt(p driver.Point) bool {
	root := w.view(w.root)
	if root == nil {
		return false
	}
	for v := pick(root, p); v != nil; v = v.parent {
		if driver.Painted(v.kind, &v.style, v.events, v.props.WindowDrag) {
			return true
		}
	}
	return false
}

// mouseButtonDown reports whether a mouse button is held, wherever the pointer is.
func mouseButtonDown() bool {
	for _, vk := range []uintptr{vkLButton, vkRButton, vkMButton} {
		if r, _, _ := getAsyncKeyState.Call(vk); r&0x8000 != 0 {
			return true
		}
	}
	return false
}
