//go:build windows

package windows

import "unsafe"

// A touchscreen's touches are the views' fingers, one per pointer ID, from the WM_POINTER messages Windows sends for touches since Windows 8. Unless a page or a scroll view takes the touch, each message still goes on to DefWindowProc, which keeps making the window's WM_GESTURE pinches and rotations and the primary touch's mouse messages, so presses, pans and touch scrolling go on as before. Those mouse messages, which carry the touch signature in their extra information, report no mouse finger, so a finger is never reported twice; a pen's stay the mouse's finger 0.
//
// RegisterTouchWindow's WM_TOUCH would end the window's WM_GESTURE messages, and EnableMouseInPointer would turn the mouse's own messages into pointer messages for the whole process, so neither is used.

var getMessageExtraInfo = user32.NewProc("GetMessageExtraInfo")

const (
	wmPointerCaptureChanged = 0x024C

	pointerFlagInContact = 0x0004
	pointerFlagCanceled  = 0x8000

	// The extra information of the mouse messages Windows makes from a touch: MI_WP_SIGNATURE and the touch bit.
	touchSignatureMask = 0xFFFFFF80
	touchSignature     = 0xFF515780
)

// touchMouse reports whether the mouse message being handled came from a touch.
func touchMouse() bool {
	info, _, _ := getMessageExtraInfo.Call()
	return uint32(info)&touchSignatureMask == touchSignature
}

// touchFinger reports a touch's pointer message as a finger, while a view listens.
func (w *window) touchFinger(message uint32, wParam, lParam uintptr) {
	id := uint32(wParam & 0xffff)
	flags := uint32(wParam >> 16 & 0xffff)
	key := uintptr(id)
	if message == wmPointerCaptureChanged || message == wmPointerLeave {
		// The touch went to another window, or left this one without lifting.
		if p, down := w.fingers.TouchAt(key); down {
			w.touch(w.fingers.TouchCancel(key, p))
		}
		return
	}
	var kind uint32
	if r, _, _ := getPointerType.Call(uintptr(id), uintptr(unsafe.Pointer(&kind))); r == 0 || kind != ptTouch {
		return
	}
	at := point{int32(int16(lParam & 0xffff)), int32(int16(lParam >> 16 & 0xffff))}
	screenToClient.Call(w.hwnd, uintptr(unsafe.Pointer(&at)))
	p := w.point(at.X, at.Y)
	switch {
	case message == wmPointerDown:
		// Touches on the caption buttons, a video's controls or a page are theirs, as mouse messages handle the press Windows makes from the touch.
		if w.captionAt(p) != 0 {
			return
		}
		if vh := videos.Load(); vh != nil && vh.barAt(w, at.X, at.Y) {
			return
		}
		if wh := webViews.Load(); wh != nil && wh.hitAt(w, at.X, at.Y) {
			return
		}
		w.touch(w.fingers.TouchDown(key, w.listensTouches(), p))
	case message == wmPointerUpdate && flags&pointerFlagInContact != 0:
		w.touch(w.fingers.TouchMove(key, p))
	case message == wmPointerUp && flags&pointerFlagCanceled != 0:
		w.touch(w.fingers.TouchCancel(key, p))
	case message == wmPointerUp:
		w.touch(w.fingers.TouchUp(key, p))
	}
}

// cancelTouches reports the touchscreen's fingers taken away, as a scroll view takes them.
func (w *window) cancelTouches() {
	w.touch(w.fingers.CancelTouches())
}
