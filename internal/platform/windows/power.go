//go:build windows

package windows

import (
	"syscall"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	getSystemPowerStatus               = kernel32.NewProc("GetSystemPowerStatus")
	registerPowerSettingNotification   = user32.NewProc("RegisterPowerSettingNotification")
	unregisterPowerSettingNotification = user32.NewProc("UnregisterPowerSettingNotification")
	guidPowerSavingStatus              = syscall.GUID{Data1: 0xe00958c0, Data2: 0xc213, Data3: 0x4ace,
		Data4: [8]byte{0xac, 0x77, 0xfe, 0xcc, 0xed, 0x2e, 0xee, 0xa5}}
)

const (
	wmPowerBroadcast      = 0x0218
	pbtPowerSettingChange = 0x8013
)

func lowPowerMode() driver.LowPowerMode {
	type powerStatus struct {
		ac, battery, percent, saver uint8
		lifeTime, fullLifeTime      uint32
	}
	var status powerStatus
	ok, _, _ := getSystemPowerStatus.Call(uintptr(unsafe.Pointer(&status)))
	if ok != 0 {
		switch status.saver {
		case 0:
			return driver.LowPowerOff
		case 1:
			return driver.LowPowerOn
		}
	}
	return driver.LowPowerUnknown
}

// The engine's owning window receives battery-saver changes for its
// surfaces, including embedded surfaces with their own engine.
func (w *window) watchPower() {
	const deviceNotifyWindowHandle = 0
	w.powerNotification, _, _ = registerPowerSettingNotification.Call(w.hwnd,
		uintptr(unsafe.Pointer(&guidPowerSavingStatus)), deviceNotifyWindowHandle)
}

func (w *window) stopPower() {
	if w.powerNotification != 0 {
		unregisterPowerSettingNotification.Call(w.powerNotification)
		w.powerNotification = 0
	}
}

func (w *window) powerChanged(kind, data uintptr) bool {
	// The setting GUID is the first member of POWERBROADCAST_SETTING.
	if kind != pbtPowerSettingChange || data == 0 || **(**syscall.GUID)(unsafe.Pointer(&data)) != guidPowerSavingStatus {
		return false
	}
	for _, surface := range windows {
		if surface.engine != nil && (surface == w || surface.hostWindow == w) {
			surface.updateEnvironment()
		}
	}
	return true
}
