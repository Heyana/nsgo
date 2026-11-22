//go:build windows

package windows

import (
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	versionDLL = syscall.NewLazyDLL("version.dll")

	setWinEventHook            = user32.NewProc("SetWinEventHook")
	unhookWinEvent             = user32.NewProc("UnhookWinEvent")
	getClassNameW              = user32.NewProc("GetClassNameW")
	openProcess                = kernel32.NewProc("OpenProcess")
	queryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	getFileVersionInfoSizeW    = versionDLL.NewProc("GetFileVersionInfoSizeW")
	getFileVersionInfoW        = versionDLL.NewProc("GetFileVersionInfoW")
	verQueryValueW             = versionDLL.NewProc("VerQueryValueW")
)

const (
	eventSystemForeground     = 0x0003
	eventObjectLocationChange = 0x800B
	objidWindow               = 0
	// winEventOutOfContext has the events posted to the thread that set the hook, through its message loop.
	winEventOutOfContext           = 0
	processQueryLimitedInformation = 0x1000
)

// The application in front, which every window's environment reports. Windows says when another window comes to the front (EVENT_SYSTEM_FOREGROUND); a second hook, on the foreground frame owner, hears its windows move and resize, as they do entering and leaving full screen. Both post to the thread that set them, the main thread, which alone uses these.
var (
	frontmost         driver.FrontmostAppInfo
	frontmostHwnd     uintptr
	frontmostMoves    uintptr // the location hook on the foreground frame owner
	frontmostMovesPID int     // a UWP client PID may differ from its frame owner
	// frontmostFront is the foreground hook, and frontmostWatching whether the hooks are set, from the first call until unwatchFrontmost.
	frontmostFront    uintptr
	frontmostWatching bool
	// frontmostEvent is the hooks' callback, made with the first hook.
	frontmostEvent uintptr
	// appNames are the display names of the executables seen in front, by path.
	appNames = map[string]string{}
)

// frontmostApp returns the application in front, following it from the first call on.
func frontmostApp() driver.FrontmostAppInfo {
	if !frontmostWatching {
		frontmostWatching = true
		if frontmostEvent == 0 {
			frontmostEvent = syscall.NewCallback(frontmostChanged)
		}
		frontmostFront, _, _ = setWinEventHook.Call(eventSystemForeground, eventSystemForeground, 0, frontmostEvent, 0, 0, winEventOutOfContext)
		frontmost, frontmostHwnd = readFrontmost(driver.FrontmostAppInfo{})
		hookFrontmostMoves(int(windowProcessID(frontmostHwnd)))
	}
	return frontmost
}

// unwatchFrontmost removes the hooks as the main thread's loop ends. Windows delivers their events through whatever message processing the thread still does, and a thread whose goroutine has ended, as a test's window thread does as it exits, would run frontmostChanged with no goroutine and crash; a later Run sets them again on its own thread.
func unwatchFrontmost() {
	if !frontmostWatching {
		return
	}
	hookFrontmostMoves(0)
	if frontmostFront != 0 {
		unhookWinEvent.Call(frontmostFront)
		frontmostFront = 0
	}
	frontmost, frontmostHwnd, frontmostWatching = driver.FrontmostAppInfo{}, 0, false
}

// frontmostChanged is a WinEventProc: a window came to the front, or a window of the process in front moved.
func frontmostChanged(_, event, hwnd, object, _, _, _ uintptr) uintptr {
	if event == eventObjectLocationChange && (int32(object) != objidWindow || hwnd != frontmostHwnd) {
		return 0
	}
	updateFrontmost()
	return 0
}

// updateFrontmost reads the application in front again, and reports it to every window when it changed.
func updateFrontmost() {
	app, hwnd := readFrontmost(frontmost)
	frontmostHwnd = hwnd
	if owner := int(windowProcessID(hwnd)); owner != frontmostMovesPID {
		hookFrontmostMoves(owner)
	}
	if app == frontmost {
		return
	}
	frontmost = app
	for _, w := range windows {
		if w.engine != nil && w.hwnd != 0 && !w.destroying {
			w.updateEnvironment()
		}
	}
}

// hookFrontmostMoves hears the location changes of process pid's windows alone, in place of the last process's.
func hookFrontmostMoves(pid int) {
	frontmostMovesPID = pid
	if frontmostMoves != 0 {
		unhookWinEvent.Call(frontmostMoves)
		frontmostMoves = 0
	}
	if pid > 0 {
		frontmostMoves, _, _ = setWinEventHook.Call(eventObjectLocationChange, eventObjectLocationChange, 0, frontmostEvent, uintptr(pid), 0, winEventOutOfContext)
	}
}

// readFrontmost reads the active window and its application. A stable process/AUMID keeps the last name; a UWP frame always resolves its client.
func readFrontmost(last driver.FrontmostAppInfo) (driver.FrontmostAppInfo, uintptr) {
	hwnd, _, _ := getForegroundWindow.Call()
	if hwnd == 0 {
		return driver.FrontmostAppInfo{}, 0
	}
	app := readWindowFrontmost(hwnd, last)
	return app, hwnd
}

func readWindowFrontmost(hwnd uintptr, last driver.FrontmostAppInfo) driver.FrontmostAppInfo {
	if hwnd == 0 {
		return driver.FrontmostAppInfo{}
	}
	pid := windowProcessID(hwnd)
	var id string
	if windowClassName(hwnd) == "ApplicationFrameWindow" {
		frameID := windowApplicationID(hwnd)
		pid, id = framedApplication(hwnd, pid, frameID)
		if id == "" {
			id = frameID
		}
	} else {
		id = processApplicationID(pid)
	}
	app := driver.FrontmostAppInfo{PID: int(pid), ID: id, FullScreen: coversMonitor(hwnd)}
	if app.PID != 0 && app.PID == last.PID && (id == "" || id == last.ID) {
		app.ID, app.Name = last.ID, last.Name
		return app
	}
	if id != "" {
		app.Name = packagedApplicationName(id)
	}
	if path := processPath(pid); path != "" {
		if app.ID == "" {
			app.ID = path
		}
		if app.Name == "" {
			app.Name = appName(path)
		}
	}
	if app.Name == "" {
		app.Name = id
	}
	return app
}

// processPath is the full path of process pid's executable, "" where Windows refuses it.
func processPath(pid uint32) string {
	if pid == 0 {
		return ""
	}
	h, _, _ := openProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer closeHandle.Call(h)
	buf := make([]uint16, syscall.MAX_LONG_PATH)
	size := uint32(len(buf))
	if r, _, _ := queryFullProcessImageNameW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size))); r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:size])
}

// appName is the display name of the executable at path: its version resource's file description, else its file name without the extension.
func appName(path string) string {
	if name, ok := appNames[path]; ok {
		return name
	}
	name := fileDescription(path)
	if name == "" {
		name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	appNames[path] = name
	return name
}

// fileDescription reads the FileDescription of the executable's version resource, in its first language, "" without one.
func fileDescription(path string) string {
	file := wide(path)
	size, _, _ := getFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(file)), 0)
	if size == 0 {
		return ""
	}
	data := make([]byte, size)
	if r, _, _ := getFileVersionInfoW.Call(uintptr(unsafe.Pointer(file)), 0, size, uintptr(unsafe.Pointer(&data[0]))); r == 0 {
		return ""
	}
	var block unsafe.Pointer
	var n uint32
	if r, _, _ := verQueryValueW.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(wide(`\VarFileInfo\Translation`))),
		uintptr(unsafe.Pointer(&block)), uintptr(unsafe.Pointer(&n))); r == 0 || n < 4 {
		return ""
	}
	lang, codepage := *(*uint16)(block), *(*uint16)(unsafe.Add(block, 2))
	key := `\StringFileInfo\` + hex4(lang) + hex4(codepage) + `\FileDescription`
	if r, _, _ := verQueryValueW.Call(uintptr(unsafe.Pointer(&data[0])), uintptr(unsafe.Pointer(wide(key))),
		uintptr(unsafe.Pointer(&block)), uintptr(unsafe.Pointer(&n))); r == 0 || n == 0 {
		return ""
	}
	return strings.TrimSpace(syscall.UTF16ToString(unsafe.Slice((*uint16)(block), n)))
}

// hex4 is v as four lowercase hexadecimal digits.
func hex4(v uint16) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[v>>12], digits[v>>8&0xf], digits[v>>4&0xf], digits[v&0xf]})
}

// coversMonitor reports whether hwnd covers its monitor whole as a full-screen window does: a maximized one leaves the taskbar, or with the taskbar hidden keeps its caption or sizing frame. A window an app takes to full screen from maximized by stripping its frame and sizing it to the monitor, as many do, stays IsZoomed, so only a maximized window with a caption or sizing frame is ruled out. The desktop's own windows, which cover it too, are not.
func coversMonitor(hwnd uintptr) bool {
	if zoomed, _, _ := isZoomed.Call(hwnd); zoomed != 0 {
		if style, _, _ := getWindowLongPtrW.Call(hwnd, gwlStyle); style&(wsCaption|wsThickFrame) != 0 {
			return false
		}
	}
	switch windowClassName(hwnd) {
	case "Progman", "WorkerW":
		return false
	}
	var r rect
	if ok, _, _ := getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return false
	}
	var mi monitorInfo
	mi.Size = uint32(unsafe.Sizeof(mi))
	monitor, _, _ := monitorFromWindow.Call(hwnd, monitorDefaultNear)
	if ok, _, _ := getMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&mi))); ok == 0 {
		return false
	}
	return covers(r, mi.Monitor)
}

// covers reports whether r covers all of m.
func covers(r, m rect) bool {
	return !m.empty() && r.Left <= m.Left && r.Top <= m.Top && r.Right >= m.Right && r.Bottom >= m.Bottom
}
