//go:build windows

package windows

// The pointer on hosted content, as NSH's input.rs gives it to hosted
// items: the window finds the innermost view under the pointer, the one
// drawn on top there, and when it is a composed web view, the page takes
// the pointer as it comes (every button, the moves and the release while
// one is held, the wheel unless the page does not scroll); when it is a
// video's control bar, the bar takes presses and drags. Everything else
// goes on as before: presses and gestures for the engine.

import (
	"runtime"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/hover"
)

var (
	getMessageTime  = user32.NewProc("GetMessageTime")
	getCursorPos    = user32.NewProc("GetCursorPos")
	windowFromPoint = user32.NewProc("WindowFromPoint")
	getCapture      = user32.NewProc("GetCapture")
)

const (
	wmSetCursor    = 0x0020
	wmRButtonUp    = 0x0205
	wmMButtonDown  = 0x0207
	wmMButtonUp    = 0x0208
	wmXButtonDown  = 0x020B
	wmXButtonUp    = 0x020C
	wmMouseLeave   = 0x02A3
	htClientArea   = 1
	idcArrow       = 32512
	idcWait        = 32514
	idcCross       = 32515
	idcSizeNWSE    = 32642
	idcSizeNESW    = 32643
	idcSizeWE      = 32644
	idcSizeNS      = 32645
	idcSizeAll     = 32646
	idcNo          = 32648
	idcHand        = 32649
	idcAppStarting = 32650
	idcHelp        = 32651
	xButtonForward = 2
)

// pick returns the innermost view at p, in the coordinates of v's
// parent's content, drawn on top there; nil when p is outside v.
func pick(v *view, p driver.Point) *view {
	if v.style.Hidden || v.style.Opacity <= 0 {
		return nil
	}
	self, children := v.reach(p)
	if children {
		local := v.contentPoint(v.local(p))
		for i := len(v.children) - 1; i >= 0; i-- {
			if h := pick(v.children[i], local); h != nil {
				return h
			}
		}
	}
	if self {
		return v
	}
	return nil
}

// pickAt returns the view under client pixel x, y.
func (w *window) pickAt(x, y int32) *view {
	root := w.view(w.root)
	if root == nil {
		return nil
	}
	return pick(root, w.point(x, y))
}

// webAt returns the composed web view under client pixel x, y, and the
// point in its pixels.
func (w *window) webAt(x, y int32) (*webView, int32, int32) {
	v := w.pickAt(x, y)
	if v == nil || v.web == nil || !v.web.composed() || v.web.at == nil {
		return nil, 0, 0
	}
	px, py := v.web.local(x, y)
	return v.web, px, py
}

// local maps client pixel x, y to the page's pixels, where the last frame
// put it.
func (wv *webView) local(x, y int32) (int32, int32) {
	if wv.at == nil {
		return x, y
	}
	lx, ly := wv.at.m.invert().apply(float64(x), float64(y))
	return int32(lx + 0.5), int32(ly + 0.5)
}

// barAt returns the video whose control bar is under client pixel x, y,
// and the point in its view's points.
func (w *window) barAt(x, y int32) (*video, float32, float32) {
	v := w.pickAt(x, y)
	if v == nil || v.video == nil {
		return nil, 0, 0
	}
	at := v.surfacePoint(w.point(x, y))
	lx, ly := float32(at.X), float32(at.Y)
	if !v.video.barAt(lx, ly, float32(v.frame.Size.Width), float32(v.frame.Size.Height)) {
		return nil, 0, 0
	}
	return v.video, lx, ly
}

// mouseButton returns the button of a mouse button message, 0 left, 1
// right, 2 middle, 3 back and 4 forward, and whether it went down.
func mouseButton(message uint32, wParam uintptr) (button int, down, ok bool) {
	switch message {
	case wmLButtonDown, wmLButtonUp:
		return 0, message == wmLButtonDown, true
	case wmRButtonDown, wmRButtonUp:
		return 1, message == wmRButtonDown, true
	case wmMButtonDown, wmMButtonUp:
		return 2, message == wmMButtonDown, true
	case wmXButtonDown, wmXButtonUp:
		button = 3
		if wParam>>16&0xffff == xButtonForward {
			button = 4
		}
		return button, message == wmXButtonDown, true
	}
	return 0, false, false
}

// hostedMouse gives a mouse message to the page or the control bar under
// it, or holding it, and reports whether one took it.
func (w *window) hostedMouse(message uint32, wParam, lParam uintptr) bool {
	wh := webViews.Load()
	vh := videos.Load()
	if wh == nil && vh == nil {
		return false
	}
	x, y := int32(int16(lParam&0xffff)), int32(int16(lParam>>16&0xffff))
	tm, _, _ := getMessageTime.Call()
	if message == wmMouseMove {
		if wh != nil && wh.rawMove(w, x, y, uint32(tm)) {
			return true
		}
		if vh != nil && vh.drag(w, x, y) {
			return true
		}
		if wh != nil {
			wh.hoverMove(w, x, y, uint32(tm))
		}
		return false
	}
	button, down, ok := mouseButton(message, wParam)
	if !ok {
		return false
	}
	if wh != nil && wh.rawButton(w, button, down, x, y, uint32(tm)) {
		return true
	}
	if vh != nil && vh.heldButton(w, button, down) {
		return true
	}
	if !down {
		return false
	}
	if wh != nil && wh.pressAt(w, button, x, y, uint32(tm)) {
		return true
	}
	// A press on the window's own content takes the keyboard from a page.
	if wh != nil {
		wh.blurFocused(w)
	}
	if button == 0 && vh != nil && vh.pressAt(w, x, y) {
		return true
	}
	return false
}

// pointerScreenPosition keeps scripted test input separate from the real
// cursor. The recorded screen point stays still between scripted moves.
func (w *window) pointerScreenPosition() (point, bool) {
	if w.kind == driver.SurfacePanel && w.opts.test != nil {
		return w.testPanelPointer, w.testPanelPointerSet
	}
	var at point
	ok, _, _ := getCursorPos.Call(uintptr(unsafe.Pointer(&at)))
	return at, ok != 0
}

func windowAtScreenPoint(at point) uintptr {
	// WindowFromPoint takes POINT by value, in two stack slots on 386
	// and one register on the 64-bit targets.
	if runtime.GOARCH == "386" {
		hwnd, _, _ := windowFromPoint.Call(uintptr(at.X), uintptr(at.Y))
		return hwnd
	}
	hwnd, _, _ := windowFromPoint.Call(uintptr(uint64(uint32(at.X)) | uint64(uint32(at.Y))<<32))
	return hwnd
}

// repickPanelHover tracks the view that moved under a still pointer.
// Only hover state follows the frame; this sends no mouse move or drag.
func (w *window) repickPanelHover() {
	if w.engine == nil || w.hwnd == 0 || lockedPointer.owner == w || w.popupTracking || w.alert != nil || w.scanner != nil {
		return
	}
	at, ok := w.panelHoverPoint()
	if !ok {
		if w.pointerIn {
			w.mouseLeft()
		}
		return
	}
	if w.opts.test != nil {
		w.pointer, w.pointerIn = w.point(at.X, at.Y), true
		w.hoverViews()
	} else {
		w.pointerMoved(at.X, at.Y)
	}
	w.hoverBars(w.pointer)
}

func (w *window) panelHoverPoint() (point, bool) {
	if w.pointerMode == driver.WindowPointerNone || !w.panelFrameVisible() {
		return point{}, false
	}
	at, ok := w.pointerScreenPosition()
	if !ok {
		return point{}, false
	}
	client := at
	var bounds rect
	if ok, _, _ := screenToClient.Call(w.hwnd, uintptr(unsafe.Pointer(&client))); ok == 0 {
		return point{}, false
	}
	if ok, _, _ := getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&bounds))); ok == 0 ||
		client.X < bounds.Left || client.X >= bounds.Right || client.Y < bounds.Top || client.Y >= bounds.Bottom {
		return point{}, false
	}
	if w.canPassUnpainted() {
		// The pass-through timer may still describe the previous frame.
		w.passPointer(!w.paintedAt(w.point(client.X, client.Y)))
	}
	ex, _, _ := getWindowLongPtrW.Call(w.hwnd, gwlExStyle)
	if ex&(wsExLayered|wsExTransparent) == wsExLayered|wsExTransparent {
		return point{}, false
	}
	if w.opts.test == nil {
		capture, _, _ := getCapture.Call()
		if capture != 0 && capture != w.hwnd || windowAtScreenPoint(at) != w.hwnd {
			return point{}, false
		}
	}
	// A window drag area is non-client to Windows even inside client pixels.
	hit := uintptr(uint32(uint16(at.X)) | uint32(uint16(at.Y))<<16)
	return client, w.hwnd != 0 && w.ncHitTest(hit) == htClient
}

// mouseLeft follows the mouse out of the client area.
func (w *window) mouseLeft() {
	w.tracking = false
	w.pointerIn = false
	w.hoverViews()
	if wh := webViews.Load(); wh != nil {
		wh.mouseLeft(w)
	}
	if w.barDrag == nil {
		w.hoverBars(driver.Point{X: -1, Y: -1})
	}
}

// captureLost lets the page or bar holding the mouse go when another
// window took it.
func (w *window) captureLost(to uintptr) {
	if to == w.hwnd {
		return
	}
	if wh := webViews.Load(); wh != nil {
		wh.captureLost(w)
	}
	if vh := videos.Load(); vh != nil {
		vh.captureLost(w)
	}
}

// hostedWheel gives a wheel turn at client pixel x, y to
// the page under it, reporting whether the page took it.
func (w *window) hostedWheel(x, y int32, delta int16, horizontal bool) bool {
	if wh := webViews.Load(); wh != nil {
		return wh.wheel(w, x, y, delta, horizontal)
	}
	return false
}

// hostedPointer gives a touch or pen contact to the page it went down
// on, reporting whether the page took it; others become mouse messages.
func (w *window) hostedPointer(message uint32, wParam, lParam uintptr) bool {
	if wh := webViews.Load(); wh != nil {
		return wh.pointer(w, message, wParam, lParam)
	}
	return false
}

func (w *window) webPointer(message uint32, wParam, lParam uintptr) bool {
	id := uint32(wParam & 0xffff)
	var kind uint32
	if r, _, _ := getPointerType.Call(uintptr(id), uintptr(unsafe.Pointer(&kind))); r == 0 ||
		kind != ptTouch && kind != ptPen {
		return false
	}
	at := point{int32(int16(lParam & 0xffff)), int32(int16(lParam >> 16 & 0xffff))}
	screenToClient.Call(w.hwnd, uintptr(unsafe.Pointer(&at)))
	wv := w.touchWebs[id]
	if wv == nil {
		if message != wmPointerDown {
			return false
		}
		if wv, _, _ = w.webAt(at.X, at.Y); wv == nil {
			return false
		}
	}
	px, py := wv.local(at.X, at.Y)
	if !wv.pointer(message, id, kind == ptTouch, px, py) {
		delete(w.touchWebs, id)
		return false
	}
	if message == wmPointerUp || message == wmPointerLeave {
		delete(w.touchWebs, id)
	} else {
		w.touchWebs[id] = wv
	}
	return true
}

// viewCursor shows the cursor the views name under the pointer (cursorAt), and reports whether they name one.
func (w *window) viewCursor() bool {
	var at point
	if r, _, _ := getCursorPos.Call(uintptr(unsafe.Pointer(&at))); r == 0 {
		return false
	}
	screenToClient.Call(w.hwnd, uintptr(unsafe.Pointer(&at)))
	c, custom, ok := w.cursorAt(w.point(at.X, at.Y))
	if !ok {
		if w.customCursors != nil {
			w.customCursors.active = 0
		}
		return false
	}
	if custom != nil {
		if h := w.customHCURSOR(custom); h != 0 {
			if w.customCursors != nil {
				w.customCursors.active = h
			}
			setCursor.Call(h)
			return true
		}
		c = custom.EffectiveFallback(c)
	}
	if w.customCursors != nil {
		w.customCursors.active = 0
	}
	if c == driver.CursorAuto {
		return false
	}
	setCursor.Call(systemCursor(c))
	return true
}

// cursorAt returns the cursor the views name at p, a point of the client area: the nearest view's from the one under p up, a text's span under p ahead of the text's own; false where none names one.
func (w *window) cursorAt(p driver.Point) (driver.Cursor, *driver.CustomCursor, bool) {
	root := w.view(w.root)
	if root == nil {
		return driver.CursorAuto, nil, false
	}
	for v := pick(root, p); v != nil; v = v.parent {
		if l, ok := textOf(v); ok {
			if _, shown := w.surfaceBounds(v); shown {
				local := v.surfacePoint(p)
				i := l.indexAt(float32(local.X-v.content.Origin.X), float32(local.Y-v.content.Origin.Y))
				if c, ok := driver.SpanCursor(v.spans, i); ok {
					return c, nil, true
				}
			}
		}
		if v.style.CursorImage != nil {
			return v.style.Cursor, v.style.CursorImage, true
		}
		if c := v.style.Cursor; c != driver.CursorAuto {
			return c, nil, true
		}
	}
	return driver.CursorAuto, nil, false
}

// refreshCursor shows the cursor under a still pointer again, as WM_SETCURSOR would, once a batch changed what is under it; not while a button is held, as a drag keeps its cursor.
func (w *window) refreshCursor() {
	if !w.pointerIn || hover.Pressed(uint32(w.held), mouseButtons) != driver.MouseButtonNone {
		return
	}
	if !w.hostedCursor() && !w.fieldCursor() && !w.viewCursor() {
		arrow, _, _ := loadCursorW.Call(0, idcArrow)
		setCursor.Call(arrow)
	}
}

// systemCursor loads and keeps the Windows cursor of c; 0 hides the
// pointer.
var cursors [driver.CursorNone + 1]uintptr

func systemCursor(c driver.Cursor) uintptr {
	if c == driver.CursorNone {
		return 0
	}
	id := uintptr(idcArrow)
	switch c {
	case driver.CursorPointer, driver.CursorGrab, driver.CursorGrabbing:
		id = idcHand
	case driver.CursorText:
		id = idcIBeam
	case driver.CursorWait:
		id = idcWait
	case driver.CursorProgress:
		id = idcAppStarting
	case driver.CursorHelp:
		id = idcHelp
	case driver.CursorCrosshair:
		id = idcCross
	case driver.CursorMove:
		id = idcSizeAll
	case driver.CursorNotAllowed:
		id = idcNo
	case driver.CursorColResize:
		id = idcSizeWE
	case driver.CursorRowResize:
		id = idcSizeNS
	case driver.CursorNwseResize:
		id = idcSizeNWSE
	case driver.CursorNeswResize:
		id = idcSizeNESW
	}
	if cursors[c] == 0 {
		h, _, _ := loadCursorW.Call(0, id)
		cursors[c] = h
	}
	return cursors[c]
}

// hostedCursor sets the cursor over the client area: the page's, or a
// hand over a control bar's button. It reports whether it set one.
func (w *window) hostedCursor() bool {
	wh := webViews.Load()
	vh := videos.Load()
	if wh == nil && vh == nil {
		return false
	}
	var at point
	getCursorPos.Call(uintptr(unsafe.Pointer(&at)))
	screenToClient.Call(w.hwnd, uintptr(unsafe.Pointer(&at)))
	if wh != nil && wh.cursor(w, at.X, at.Y) {
		return true
	}
	if vh != nil && vh.cursor(w, at.X, at.Y) {
		return true
	}
	return false
}

// forget drops what the window's pointer holds of wv, which closes.
func (w *window) forget(wv *webView) {
	if w.rawWeb == wv {
		w.rawWeb = nil
		releaseCapture.Call()
	}
	if w.hoverWeb == wv {
		w.hoverWeb = nil
	}
	for id, t := range w.touchWebs {
		if t == wv {
			delete(w.touchWebs, id)
		}
	}
}
