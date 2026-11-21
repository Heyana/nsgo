//go:build windows

package windows

import (
	"testing"
	"unsafe"
)

// TestWASAPIGUIDs verifies every WASAPI class and interface ID against
// Windows' CLSIDFromString using the canonical strings from mmdeviceapi.h,
// audioclient.h and ksmedia.h.
func TestWASAPIGUIDs(t *testing.T) {
	for _, tc := range []struct {
		name       string
		id         *guid
		documented string
	}{
		{"CLSID_MMDeviceEnumerator", &clsidMMDeviceEnumerator, "{BCDE0395-E52F-467C-8E3D-C4579291692E}"},
		{"IID_IMMDeviceEnumerator", &iidMMDeviceEnumerator, "{A95664D2-9614-4F35-A746-DE8DB63617E6}"},
		{"IID_IMMNotificationClient", &iidMMNotificationClient, "{7991EEC9-7E89-4D85-8390-6C703CEC60C0}"},
		{"IID_IAudioClient", &iidAudioClient, "{1CB9AD4C-DBFA-4C32-B178-C2F568A703B2}"},
		{"IID_IAudioRenderClient", &iidAudioRenderClient, "{F294ACFC-3146-4483-A7BF-ADDCA7C260E2}"},
		{"IID_IAudioClock", &iidAudioClock, "{CD63314F-3FBA-4A1B-812C-EF96358728E7}"},
	} {
		var want guid
		if hr, _, _ := clsidFromString.Call(uintptr(unsafe.Pointer(wide(tc.documented))), uintptr(unsafe.Pointer(&want))); failed(hr) {
			t.Fatalf("CLSIDFromString(%s): HRESULT %#x", tc.documented, uint32(hr))
		}
		if *tc.id != want {
			t.Errorf("%s is %s, want %s", tc.name, formatGUID(*tc.id), tc.documented)
		}
	}
	var wantFloat guid
	if hr, _, _ := clsidFromString.Call(uintptr(unsafe.Pointer(wide("{00000003-0000-0010-8000-00AA00389B71}"))), uintptr(unsafe.Pointer(&wantFloat))); failed(hr) {
		t.Fatalf("CLSIDFromString(KSDATAFORMAT_SUBTYPE_IEEE_FLOAT): HRESULT %#x", uint32(hr))
	}
	gotFloat := guid{
		a: ksDataFormatSubtypeIEEEFloat.a,
		b: ksDataFormatSubtypeIEEEFloat.b,
		c: ksDataFormatSubtypeIEEEFloat.c,
		d: ksDataFormatSubtypeIEEEFloat.d,
	}
	if gotFloat != wantFloat {
		t.Errorf("KSDATAFORMAT_SUBTYPE_IEEE_FLOAT is %s, want {00000003-0000-0010-8000-00AA00389B71}", formatGUID(gotFloat))
	}
}
