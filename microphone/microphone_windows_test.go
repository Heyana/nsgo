//go:build windows

package microphone

import (
	"errors"
	"testing"
)

func TestWindowsGUIDsAndConsent(t *testing.T) {
	if clsidMMDeviceEnumerator.a != 0xBCDE0395 || clsidMMDeviceEnumerator.b != 0xE52F || clsidMMDeviceEnumerator.c != 0x467C {
		t.Errorf("CLSID_MMDeviceEnumerator = %+x", clsidMMDeviceEnumerator)
	}
	if iidIAudioCaptureClient.a != 0xC8ADBD64 || iidIAudioCaptureClient.b != 0xE71E || iidIAudioCaptureClient.c != 0x48A0 {
		t.Errorf("IID_IAudioCaptureClient = %+x", iidIAudioCaptureClient)
	}
	if !windowsConsentDenied("Deny", "Allow", "Allow") || !windowsConsentDenied("Allow", "deny", "Allow") || !windowsConsentDenied("Allow", "Allow", "DENY") {
		t.Error("windowsConsentDenied missed a Deny entry")
	}
	if windowsConsentDenied("Allow", "Allow", "") {
		t.Error("windowsConsentDenied rejected allowed entries")
	}
	if !errors.Is(mapWASAPIError("test", hrAccessDenied), ErrDenied) {
		t.Error("hrAccessDenied did not map to ErrDenied")
	}
	if !errors.Is(mapWASAPIError("test", hrDeviceInUse), ErrBusy) {
		t.Error("hrDeviceInUse did not map to ErrBusy")
	}
	if !errors.Is(mapWASAPIError("test", hrNotFound), ErrUnavailable) {
		t.Error("hrNotFound did not map to ErrUnavailable")
	}
	if !isNoSpaceError(errDiskFull) || !isNoSpaceError(errHandleDiskFull) {
		t.Error("isNoSpaceError did not recognize disk full errnos")
	}
	if !Supported() {
		t.Error("Supported() = false on Windows")
	}
	if _, err := Capture(0, func([]int16) {}); err == nil {
		t.Error("Capture(0) succeeded, want error")
	}
}
