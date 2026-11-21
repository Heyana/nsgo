//go:build windows && (amd64 || arm64)

package windows

import "syscall"

// wasapiNotifyPropertyChangedSlot returns IMMNotificationClient::OnPropertyValueChanged.
// PROPERTYKEY is 20 bytes, which both Windows amd64 (>8 bytes) and arm64 (>16 bytes)
// pass by pointer as a single register argument.
func wasapiNotifyPropertyChangedSlot() uintptr {
	return syscall.NewCallback(func(_ *wasapiNotifier, _, _ uintptr) uintptr { return sOK })
}
