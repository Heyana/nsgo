//go:build windows

package windows

import (
	"sync"
	"unsafe"
)

var enumDisplaySettingsW = user32.NewProc("EnumDisplaySettingsW")

// displayMode has the layout of DEVMODEW. Only the size and display
// fields used by EnumDisplaySettingsW are named here.
type displayMode struct {
	_         [68]byte
	size      uint16
	_         [98]byte
	bits      uint32
	width     uint32
	height    uint32
	flags     uint32
	frequency uint32
	_         [32]byte
}

type displayModeKey struct {
	device              [32]uint16
	width, height, bits uint32
}

var displayModes struct {
	sync.Mutex
	maximum map[displayModeKey]float64
}

func displayRates(device [32]uint16) (rate, maximum float64) {
	const currentSettings uint32 = ^uint32(0)
	mode := displayMode{size: uint16(unsafe.Sizeof(displayMode{}))}
	if ok, _, _ := enumDisplaySettingsW.Call(uintptr(unsafe.Pointer(&device[0])), uintptr(currentSettings), uintptr(unsafe.Pointer(&mode))); ok == 0 {
		return 0, 0
	}
	if mode.frequency > 1 {
		rate = float64(mode.frequency)
	}
	maximum = rate
	key := displayModeKey{device, mode.width, mode.height, mode.bits}
	displayModes.Lock()
	defer displayModes.Unlock()
	if maximum, ok := displayModes.maximum[key]; ok {
		return rate, maximum
	}
	for index := uint32(0); ; index++ {
		mode.size = uint16(unsafe.Sizeof(mode))
		if ok, _, _ := enumDisplaySettingsW.Call(uintptr(unsafe.Pointer(&device[0])), uintptr(index), uintptr(unsafe.Pointer(&mode))); ok == 0 {
			break
		}
		if mode.width == key.width && mode.height == key.height && mode.bits == key.bits && mode.frequency > 1 {
			maximum = max(maximum, float64(mode.frequency))
		}
	}
	if displayModes.maximum == nil {
		displayModes.maximum = make(map[displayModeKey]float64)
	}
	displayModes.maximum[key] = maximum
	return rate, maximum
}

func resetDisplayModes() {
	displayModes.Lock()
	clear(displayModes.maximum)
	displayModes.Unlock()
}
