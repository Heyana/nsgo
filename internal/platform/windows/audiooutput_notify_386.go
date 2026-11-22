//go:build windows && 386

package windows

import "syscall"

// wasapiNotifyPropertyChangedSlot returns IMMNotificationClient::OnPropertyValueChanged. On 386 stdcall, the 20-byte PROPERTYKEY parameter is passed by value as five 32-bit words on the stack, so the callback must pop all seven words.
func wasapiNotifyPropertyChangedSlot() uintptr {
	return syscall.NewCallback(func(_ *wasapiNotifier, _, _, _, _, _, _ uintptr) uintptr { return sOK })
}
