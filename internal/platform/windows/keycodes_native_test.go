//go:build windows

package windows

import (
	"slices"
	"syscall"
	"testing"
	"unsafe"
)

// TestLayoutCodeNamesFollowTheLayout translates the French (AZERTY) and German (QWERTZ) keyboard layouts without activating either: the key in QWERTY's W position keeps its physical code KeyW, from its scan code, while the name the layout gives it follows the layout (#1242). A layout the user has not enabled is unloaded again.
func TestLayoutCodeNamesFollowTheLayout(t *testing.T) {
	n, _, _ := getKeyboardLayoutList.Call(0, 0)
	enabled := make([]uintptr, n)
	if n > 0 {
		n, _, _ = getKeyboardLayoutList.Call(n, uintptr(unsafe.Pointer(&enabled[0])))
		enabled = enabled[:n]
	}
	for _, c := range []struct {
		klid string
		want map[string]string
	}{
		{"00000409", map[string]string{"KeyW": "w", "KeyQ": "q", "KeyY": "y", "KeyZ": "z", "ArrowLeft": "left", "ShiftRight": "shift"}},
		{"0000040C", map[string]string{"KeyW": "z", "KeyQ": "a", "KeyA": "q", "KeyZ": "w", "ArrowLeft": "left", "ShiftRight": "shift"}},
		{"00000407", map[string]string{"KeyW": "w", "KeyY": "z", "KeyZ": "y", "ArrowLeft": "left", "ShiftRight": "shift"}},
	} {
		t.Run(c.klid, func(t *testing.T) {
			klid, err := syscall.UTF16PtrFromString(c.klid)
			if err != nil {
				t.Fatal(err)
			}
			const klfNoTellShell = 0x80
			hkl, _, _ := loadKeyboardLayoutW.Call(uintptr(unsafe.Pointer(klid)), klfNoTellShell)
			if hkl == 0 {
				t.Skipf("Windows cannot load the keyboard layout %s", c.klid)
			}
			if !slices.Contains(enabled, hkl) {
				defer unloadKeyboardLayout.Call(hkl)
			}
			names := layoutCodeNames(hkl)
			for code, want := range c.want {
				if got := names[code]; got != want {
					t.Errorf("%s names %s %q, want %q", c.klid, code, got, want)
				}
			}
		})
	}
}
