//go:build windows

package windows

import (
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	wmRawInput    = 0x00ff
	rawMousePage  = 1
	rawMouseUsage = 2
)

var (
	clipCursor         = user32.NewProc("ClipCursor")
	getClipCursor      = user32.NewProc("GetClipCursor")
	warpCursor         = user32.NewProc("SetCursorPos")
	showCursor         = user32.NewProc("ShowCursor")
	registerRawInput   = user32.NewProc("RegisterRawInputDevices")
	registeredRawInput = user32.NewProc("GetRegisteredRawInputDevices")
	getRawInput        = user32.NewProc("GetRawInputData")
)

type rawInputDevice struct {
	UsagePage, Usage uint16
	Flags            uint32
	Target           uintptr
}

type rawInputHeader struct {
	Type, Size    uint32
	Device, Param uintptr
}

type rawMouseInput struct {
	Header              rawInputHeader
	Flags, _            uint16
	Buttons, RawButtons uint32
	X, Y                int32
	Extra               uint32
}

// The message loop owns the process's raw mouse registration and the
// desktop's cursor clip while a window holds it.
var lockedPointer struct {
	owner        *window
	token        driver.PointerLock
	start        point
	clip         rect
	registration rawInputDevice
	hides        int
}

func (w *window) setPointerLock(it *driver.MountItem) {
	token := driver.PointerLock{ID: it.ID, Count: it.Count}
	if !it.On {
		if lockedPointer.owner == w && lockedPointer.token == token {
			w.releasePointer(false)
		}
		return
	}

	foreground, _, _ := getForegroundWindow.Call()
	if foreground == 0 || foreground != w.owner() || w.destroying {
		w.engine.Dispatch(token.Unlocked())
		return
	}
	if old := lockedPointer.owner; old != nil {
		if old == w {
			lockedPointer.token = token
			return
		}
		old.releasePointer(true)
	}

	registration, ok := previousMouseRegistration()
	if !ok {
		w.engine.Dispatch(token.Unlocked())
		return
	}
	var start point
	var prior rect
	at, _, _ := getCursorPos.Call(uintptr(unsafe.Pointer(&start)))
	clipped, _, _ := getClipCursor.Call(uintptr(unsafe.Pointer(&prior)))
	if at == 0 || clipped == 0 {
		w.engine.Dispatch(token.Unlocked())
		return
	}
	lock := rawInputDevice{UsagePage: rawMousePage, Usage: rawMouseUsage, Target: w.hwnd}
	if !setRawMouse(lock) {
		w.engine.Dispatch(token.Unlocked())
		return
	}
	box := rect{Left: start.X, Top: start.Y, Right: start.X + 1, Bottom: start.Y + 1}
	if ok, _, _ := clipCursor.Call(uintptr(unsafe.Pointer(&box))); ok == 0 {
		setRawMouse(registration)
		w.engine.Dispatch(token.Unlocked())
		return
	}

	lockedPointer.owner, lockedPointer.token = w, token
	lockedPointer.start, lockedPointer.clip, lockedPointer.registration = start, prior, registration
	for {
		count, _, _ := showCursor.Call(0)
		lockedPointer.hides++
		if int32(count) < 0 {
			break
		}
	}
}

func previousMouseRegistration() (rawInputDevice, bool) {
	const remove = 1 // RIDEV_REMOVE
	missing := rawInputDevice{UsagePage: rawMousePage, Usage: rawMouseUsage, Flags: remove}
	var count uint32
	const size = unsafe.Sizeof(rawInputDevice{})
	// Windows reports ERROR_INSUFFICIENT_BUFFER for this size probe.
	if result, _, err := registeredRawInput.Call(0, uintptr(unsafe.Pointer(&count)), size); uint32(result) == 0xffffffff && err != errInsufficientBuffer {
		return missing, false
	}
	if count == 0 {
		return missing, true
	}
	devices := make([]rawInputDevice, count)
	result, _, _ := registeredRawInput.Call(uintptr(unsafe.Pointer(&devices[0])), uintptr(unsafe.Pointer(&count)), size)
	if uint32(result) == 0xffffffff || result > uintptr(len(devices)) {
		return missing, false
	}
	for _, device := range devices[:result] {
		if device.UsagePage == rawMousePage && device.Usage == rawMouseUsage {
			return device, true
		}
	}
	return missing, true
}

func setRawMouse(registration rawInputDevice) bool {
	ok, _, _ := registerRawInput.Call(uintptr(unsafe.Pointer(&registration)), 1, unsafe.Sizeof(registration))
	return ok != 0
}

func (w *window) releasePointer(report bool) {
	if lockedPointer.owner != w {
		return
	}
	lock := lockedPointer
	lockedPointer.owner, lockedPointer.hides = nil, 0
	if lock.clip == virtualCursorBounds() {
		clipCursor.Call(0)
	} else {
		clipCursor.Call(uintptr(unsafe.Pointer(&lock.clip)))
	}
	warpCursor.Call(uintptr(lock.start.X), uintptr(lock.start.Y))
	setRawMouse(lock.registration)
	for range lock.hides {
		showCursor.Call(1)
	}
	if report && w.engine != nil {
		w.engine.Dispatch(lock.token.Unlocked())
	}
}

func virtualCursorBounds() rect {
	const xVirtualScreen, yVirtualScreen, cxVirtualScreen, cyVirtualScreen = 76, 77, 78, 79
	metric := func(n uintptr) int32 {
		v, _, _ := getSystemMetrics.Call(n)
		return int32(v)
	}
	x, y := metric(xVirtualScreen), metric(yVirtualScreen)
	return rect{Left: x, Top: y, Right: x + metric(cxVirtualScreen), Bottom: y + metric(cyVirtualScreen)}
}

func (w *window) escapePointer(vk uintptr) bool {
	if lockedPointer.owner != w || vk != vkEscape {
		return false
	}
	w.releasePointer(true)
	return true
}

func (w *window) rawPointerMotion(handle uintptr) {
	if lockedPointer.owner != w {
		return
	}
	const input = 0x10000003 // RID_INPUT
	const absolute = 1       // MOUSE_MOVE_ABSOLUTE
	var raw rawMouseInput
	size := uint32(unsafe.Sizeof(raw))
	n, _, _ := getRawInput.Call(handle, input, uintptr(unsafe.Pointer(&raw)), uintptr(unsafe.Pointer(&size)), unsafe.Sizeof(raw.Header))
	if n >= unsafe.Sizeof(raw) && n != 0xffffffff && raw.Header.Type == 0 && raw.Flags&absolute == 0 {
		w.pointerMotion(float64(raw.X), float64(raw.Y))
	}
}

func (w *window) pointerMotion(dx, dy float64) {
	if lockedPointer.owner == w && (dx != 0 || dy != 0) {
		w.engine.Dispatch(lockedPointer.token.Motion(dx, dy))
	}
}
