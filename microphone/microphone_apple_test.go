//go:build darwin

package microphone

import (
	"syscall"
	"testing"
	"unsafe"
)

func TestAppleStructLayouts(t *testing.T) {
	if got := unsafe.Sizeof(streamDescription{}); got != 40 {
		t.Errorf("sizeof(AudioStreamBasicDescription) = %d, want 40", got)
	}
	if !Supported() {
		t.Error("Supported() = false on Apple platform")
	}
	if _, err := Capture(0, func([]int16) {}); err == nil {
		t.Error("Capture(0) succeeded, want error")
	}
	if !isNoSpaceError(syscall.ENOSPC) || !isNoSpaceError(syscall.EDQUOT) {
		t.Error("isNoSpaceError did not recognize ENOSPC/EDQUOT")
	}
}
