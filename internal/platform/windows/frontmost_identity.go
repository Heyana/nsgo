//go:build windows

package windows

import (
	"runtime"
	"syscall"
	"unsafe"
)

var (
	getApplicationUserModelID   = kernel32.NewProc("GetApplicationUserModelId")
	enumChildWindows            = user32.NewProc("EnumChildWindows")
	shGetPropertyStoreForWindow = shell32.NewProc("SHGetPropertyStoreForWindow")
	propVariantClear            = ole32.NewProc("PropVariantClear")
	frameChildCallback          = syscall.NewCallback(frontmostFrameChild)
	packagedAppNames            = map[string]string{}
)

func windowProcessID(hwnd uintptr) uint32 {
	var pid uint32
	if hwnd != 0 {
		getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	}
	return pid
}

func windowClassName(hwnd uintptr) string {
	var class [256]uint16
	if n, _, _ := getClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&class[0])), uintptr(len(class))); n != 0 {
		return syscall.UTF16ToString(class[:n])
	}
	return ""
}

// processApplicationID returns a packaged process's AUMID, not a Win32 process's optional explicit taskbar ID. The latter keeps its path ID.
func processApplicationID(pid uint32) string {
	if pid == 0 || getApplicationUserModelID.Find() != nil {
		return ""
	}
	process, _, _ := openProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if process == 0 {
		return ""
	}
	defer closeHandle.Call(process)
	var size uint32
	const insufficientBuffer = 122
	if code, _, _ := getApplicationUserModelID.Call(process, uintptr(unsafe.Pointer(&size)), 0); code != insufficientBuffer || size == 0 || size > 32768 {
		return ""
	}
	buf := make([]uint16, size)
	if code, _, _ := getApplicationUserModelID.Call(process, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&buf[0]))); code != 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func windowApplicationID(hwnd uintptr) string {
	var store *comObject
	hr, _, _ := shGetPropertyStoreForWindow.Call(hwnd, uintptr(unsafe.Pointer(&iidPropertyStore)), uintptr(unsafe.Pointer(&store)))
	if failed(hr) || store == nil {
		return ""
	}
	defer store.release()
	var value propVariant
	defer propVariantClear.Call(uintptr(unsafe.Pointer(&value)))
	const propertyStoreGetValue = 5
	if failed(store.call(propertyStoreGetValue, uintptr(unsafe.Pointer(&pkeyAppUserModelID)), uintptr(unsafe.Pointer(&value)))) || value.vt != 31 {
		return ""
	}
	return wideString((*uint16)(value.value))
}

// A frame can host different UWP processes while its own PID stays the same. Read its real child for every foreground/window event; never cache the frame host's identity as the app. CoreWindow wins over render helpers.
func framedApplication(hwnd uintptr, host uint32, expected string) (uint32, string) {
	query := new(frontmostFrameQuery)
	query.host = host
	utf16 := syscall.StringToUTF16(expected)
	if len(utf16) > len(query.expected) {
		return 0, expected
	}
	copy(query.expected[:], utf16)
	var pin runtime.Pinner
	pin.Pin(query)
	defer pin.Unpin()
	enumChildWindows.Call(hwnd, frameChildCallback, uintptr(unsafe.Pointer(query)))
	return query.pid, syscall.UTF16ToString(query.id[:])
}

type frontmostFrameQuery struct {
	host, pid    uint32
	expected, id [512]uint16
}

func frontmostFrameChild(hwnd uintptr, data *frontmostFrameQuery) uintptr {
	pid := windowProcessID(hwnd)
	if pid == 0 || pid == data.host {
		return 1
	}
	if visible, _, _ := isWindowVisible.Call(hwnd); visible == 0 {
		return 1
	}
	id := processApplicationID(pid)
	if id == "" {
		return 1
	}
	expected := syscall.UTF16ToString(data.expected[:])
	if expected != "" && expected != id {
		return 1
	}
	core := windowClassName(hwnd) == "Windows.UI.Core.CoreWindow"
	if data.pid == 0 || core {
		data.pid = pid
		copy(data.id[:], syscall.StringToUTF16(id))
	}
	if core {
		return 0
	}
	return 1
}

// The AppsFolder supplies the localized display name for a registered packaged AUMID. Missing registration falls back to the actual executable.
func packagedApplicationName(id string) string {
	if name, ok := packagedAppNames[id]; ok {
		return name
	}
	var item *comObject
	hr, _, _ := shCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(wide(`shell:AppsFolder\`+id))), 0,
		uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
	name := ""
	if !failed(hr) && item != nil {
		var display *uint16
		const shellItemGetDisplayName = 5
		if !failed(item.call(shellItemGetDisplayName, 0, uintptr(unsafe.Pointer(&display)))) {
			name = takeString(display)
		}
		item.release()
	}
	if name != "" {
		packagedAppNames[id] = name
	}
	return name
}
