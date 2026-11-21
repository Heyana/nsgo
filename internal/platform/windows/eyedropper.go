//go:build windows

package windows

import (
	"sync"
	"syscall"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	eyedropperGetDC              = user32.NewProc("GetDC")
	eyedropperReleaseDC          = user32.NewProc("ReleaseDC")
	eyedropperGetCapture         = user32.NewProc("GetCapture")
	eyedropperIsWindow           = user32.NewProc("IsWindow")
	eyedropperSetDisplayAffinity = user32.NewProc("SetWindowDisplayAffinity")
	eyedropperGetPixel           = gdi32.NewProc("GetPixel")
)

const (
	smXVirtualScreen      = 76
	smYVirtualScreen      = 77
	smCXVirtualScreen     = 78
	smCYVirtualScreen     = 79
	wmCancelMode          = 0x001F
	wdaExcludeFromCapture = 0x00000011
	colorPickerClassName  = "NsgoColorPicker"
)

var (
	colorPickerClassOnce sync.Once
	activeColorPicker    *winColorPicker
)

type winColorPicker struct {
	w         *window
	id        uint32
	hwnd      uintptr
	prevFocus uintptr
	cursor    uintptr
	bounds    virtualScreenRect
	excluded  bool
	state     colorPickerState
}

func registerColorPickerClass(inst uintptr) *uint16 {
	className := wide(colorPickerClassName)
	colorPickerClassOnce.Do(func() {
		cursor, _, _ := loadCursorW.Call(0, idcCross)
		wc := wndClassEx{
			Size:      uint32(unsafe.Sizeof(wndClassEx{})),
			WndProc:   syscall.NewCallback(colorPickerWndProc),
			Instance:  inst,
			Cursor:    cursor,
			ClassName: className,
		}
		registerClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	})
	return className
}

func virtualScreenBounds() virtualScreenRect {
	metric := func(index uintptr) int32 {
		r, _, _ := getSystemMetrics.Call(index)
		return int32(r)
	}
	return virtualScreenRect{
		x:      metric(smXVirtualScreen),
		y:      metric(smYVirtualScreen),
		width:  metric(smCXVirtualScreen),
		height: metric(smCYVirtualScreen),
	}
}

// pickScreenColor opens a transparent topmost overlay across the virtual
// screen that captures the pointer with a crosshair cursor, samples the screen
// pixel under the left click via GetDC(0)+GetPixel, and cancels on Escape,
// right-click or focus/capture loss.
func (w *window) pickScreenColor(id uint32) {
	if w.destroying || w.engine == nil {
		return
	}
	if activeColorPicker != nil {
		activeColorPicker.finish(0, false)
	}
	bounds := virtualScreenBounds()
	if !bounds.valid() {
		w.engine.Dispatch(colorPickerResultEvent(id, 0, false))
		return
	}
	inst, _, _ := getModuleHandleW.Call(0)
	className := registerColorPickerClass(inst)
	cursor, _, _ := loadCursorW.Call(0, idcCross)
	prevFocus, _, _ := getFocus.Call()

	p := &winColorPicker{
		w:         w,
		id:        id,
		prevFocus: prevFocus,
		cursor:    cursor,
		bounds:    bounds,
		state:     colorPickerState{starting: true},
	}
	ex := uintptr(wsExTopmost | wsExToolWindow | wsExLayered)
	hwnd, _, _ := createWindowExW.Call(
		ex,
		uintptr(unsafe.Pointer(className)),
		0,
		wsPopup,
		uintptr(bounds.x),
		uintptr(bounds.y),
		uintptr(bounds.width),
		uintptr(bounds.height),
		w.owner(),
		0,
		inst,
		0,
	)
	if hwnd == 0 {
		w.engine.Dispatch(colorPickerResultEvent(id, 0, false))
		return
	}
	p.hwnd = hwnd
	// Alpha 1/255 is visually invisible while still hit-testing pointer input
	// across every monitor so clicks never leak to underlying windows.
	setLayeredWindowAttributes.Call(hwnd, 0, 1, lwaAlpha)
	// Exclude the overlay from DWM screen capture on Windows 10 2004+ so
	// GetDC(0)+GetPixel samples the untouched desktop beneath it.
	if eyedropperSetDisplayAffinity.Find() == nil {
		if r, _, _ := eyedropperSetDisplayAffinity.Call(hwnd, wdaExcludeFromCapture); r != 0 {
			p.excluded = true
		}
	}

	activeColorPicker = p
	w.colorPicker = p
	showWindow.Call(hwnd, swShow)
	setForegroundWindow.Call(hwnd)
	setFocus.Call(hwnd)
	setCapture.Call(hwnd)
	if cursor != 0 {
		setCursor.Call(cursor)
	}
	p.state.starting = false
}

func (w *window) cancelColorPicker() {
	if w.colorPicker != nil {
		w.colorPicker.finish(0, false)
	}
}

func (p *winColorPicker) sampleAtCursor() (driver.Color, bool) {
	var pt point
	if r, _, _ := getCursorPos.Call(uintptr(unsafe.Pointer(&pt))); r == 0 {
		return 0, false
	}
	x, y := p.bounds.clamp(pt.X, pt.Y)
	if !p.excluded && p.hwnd != 0 {
		showWindow.Call(p.hwnd, swHide)
	}
	dc, _, _ := eyedropperGetDC.Call(0)
	if dc == 0 {
		return 0, false
	}
	defer eyedropperReleaseDC.Call(0, dc)
	cr, _, _ := eyedropperGetPixel.Call(dc, uintptr(x), uintptr(y))
	return colorFromColorRef(uint32(cr))
}

func (p *winColorPicker) finish(c driver.Color, ok bool) {
	color, picked, first := p.state.finish(c, ok)
	if !first {
		return
	}
	if activeColorPicker == p {
		activeColorPicker = nil
	}
	if p.w != nil && p.w.colorPicker == p {
		p.w.colorPicker = nil
	}
	if p.hwnd != 0 {
		if capHwnd, _, _ := eyedropperGetCapture.Call(); capHwnd == p.hwnd {
			releaseCapture.Call()
		}
		if valid, _, _ := eyedropperIsWindow.Call(p.hwnd); valid != 0 {
			destroyWindow.Call(p.hwnd)
		}
		p.hwnd = 0
	}
	if p.prevFocus != 0 {
		if valid, _, _ := eyedropperIsWindow.Call(p.prevFocus); valid != 0 {
			setFocus.Call(p.prevFocus)
		}
	} else if p.w != nil && p.w.hwnd != 0 {
		if valid, _, _ := eyedropperIsWindow.Call(p.w.hwnd); valid != 0 {
			setFocus.Call(p.w.hwnd)
		}
	}
	arrow, _, _ := loadCursorW.Call(0, idcArrow)
	if arrow != 0 {
		setCursor.Call(arrow)
	}
	if p.w != nil && p.w.engine != nil && !p.w.destroying {
		p.w.engine.Dispatch(colorPickerResultEvent(p.id, color, picked))
	}
}

func colorPickerWndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	p := activeColorPicker
	if p == nil || p.hwnd != hwnd {
		r, _, _ := defWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
		return r
	}
	switch message {
	case wmEraseBkgnd:
		return 1
	case wmSetCursor:
		if p.cursor != 0 {
			setCursor.Call(p.cursor)
			return 1
		}
	case wmMouseMove:
		if p.cursor != 0 {
			setCursor.Call(p.cursor)
		}
		return 0
	case wmLButtonDown:
		if p.state.handle(colorPickLeftDown) == colorPickSample {
			c, ok := p.sampleAtCursor()
			p.finish(c, ok)
		}
		return 0
	case wmRButtonDown:
		if p.state.handle(colorPickRightDown) == colorPickCancel {
			p.finish(0, false)
		}
		return 0
	case wmMButtonDown:
		if p.state.handle(colorPickMiddleDown) == colorPickCancel {
			p.finish(0, false)
		}
		return 0
	case wmKeyDown, wmSysKeyDown:
		ev := colorPickOtherKey
		if wParam == vkEscape {
			ev = colorPickEscape
		}
		if p.state.handle(ev) == colorPickCancel {
			p.finish(0, false)
		}
		return 0
	case wmCaptureChanged:
		if p.state.handle(colorPickCaptureLost) == colorPickCancel {
			p.finish(0, false)
		}
		return 0
	case wmCancelMode:
		if p.state.handle(colorPickCancelMode) == colorPickCancel {
			p.finish(0, false)
		}
		return 0
	case wmActivate:
		if wParam&0xffff == 0 && p.state.handle(colorPickDeactivate) == colorPickCancel {
			p.finish(0, false)
		}
		return 0
	case wmDestroy:
		if p.state.handle(colorPickDestroy) == colorPickCancel {
			p.finish(0, false)
		}
		return 0
	}
	r, _, _ := defWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}
