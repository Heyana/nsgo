//go:build linux && !android

package microphone

import (
	"syscall"
	"testing"
	"unsafe"
)

func TestLinuxAudioLayoutsAndHelpers(t *testing.T) {
	if got := unsafe.Sizeof(paSampleSpec{}); got != 12 {
		t.Errorf("sizeof(pa_sample_spec) = %d, want 12", got)
	}
	if got := unsafe.Sizeof(paBufferAttr{}); got != 20 {
		t.Errorf("sizeof(pa_buffer_attr) = %d, want 20", got)
	}
	pcm := bytesToInt16([]byte{0x00, 0x80, 0xff, 0x7f, 0x01, 0x00})
	if len(pcm) != 3 || pcm[0] != -32768 || pcm[1] != 32767 || pcm[2] != 1 {
		t.Errorf("bytesToInt16 = %v", pcm)
	}
	if free := availableDiskSpace(t.TempDir()); free <= 0 {
		t.Errorf("availableDiskSpace(TempDir) = %d, want > 0", free)
	}
	if !isNoSpaceError(syscall.ENOSPC) || !isNoSpaceError(syscall.EDQUOT) {
		t.Error("isNoSpaceError did not recognize ENOSPC/EDQUOT")
	}
	if _, err := Capture(0, func([]int16) {}); err == nil {
		t.Error("Capture(0) succeeded, want error")
	}
}
