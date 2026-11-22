//go:build windows

package windows

import (
	"runtime"
	"strings"
	"syscall"
	"testing"
	"unsafe"
)

func TestSetForcedUILanguageUpdatesThreadPreferred(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer clearForcedUILanguage()

	getThreadPreferred := kernel32.NewProc("GetThreadPreferredUILanguages")
	const muiThreadLanguages = 0x00000010

	readThread := func() []string {
		var count, size uint32
		r, _, _ := getThreadPreferred.Call(muiLanguageName|muiThreadLanguages, uintptr(unsafe.Pointer(&count)), 0, uintptr(unsafe.Pointer(&size)))
		if r == 0 || size == 0 {
			return nil
		}
		buf := make([]uint16, size)
		r, _, _ = getThreadPreferred.Call(muiLanguageName|muiThreadLanguages, uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
		if r == 0 {
			return nil
		}
		var out []string
		for start, i := 0, 0; i < len(buf); i++ {
			if buf[i] == 0 {
				if i == start {
					break
				}
				out = append(out, syscall.UTF16ToString(buf[start:i]))
				start = i + 1
			}
		}
		return out
	}

	setForcedUILanguage("de-DE")
	got := readThread()
	if len(got) == 0 || !strings.EqualFold(got[0], "de-DE") {
		t.Fatalf("after forcing de-DE, thread preferred UI languages = %v", got)
	}

	setForcedUILanguage("")
	if got := readThread(); len(got) != 0 {
		t.Fatalf("after clearing forced language, thread preferred UI languages = %v, want empty", got)
	}
}
