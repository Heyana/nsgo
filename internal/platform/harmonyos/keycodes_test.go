package harmonyos

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestKeyPosition checks InputKit's KeyCodes and the emulator's Linux codes against KeyboardEvent.code names, and that every name the table gives is one the other targets report too.
func TestKeyPosition(t *testing.T) {
	for code, want := range map[driver.NodeID]string{
		2017: "KeyA", 2039: "KeyW", 2042: "KeyZ", 2000: "Digit0", 2009: "Digit9", 2090: "F1", 2101: "F12",
		2103: "Numpad0", 2112: "Numpad9", 2014: "ArrowLeft", 2047: "ShiftLeft", 2073: "ControlRight", 2055: "Backspace", 2071: "Delete",
		17: "KeyW", 30: "KeyA", 103: "ArrowUp", 42: "ShiftLeft",
		2010: "", 2065: "", 0: "", 3000: "",
	} {
		if got := keyPosition(code); got != want {
			t.Errorf("keyPosition(%d) = %q, want %q", code, got, want)
		}
	}
	for code, name := range ohosKeyPositions {
		if !knownPosition(name) {
			t.Errorf("KeyCode %d names %q, which no other target reports", code, name)
		}
	}
}

func knownPosition(name string) bool {
	for code := uint16(1); code < 0x300; code++ {
		if driver.USBKeyCode(code) == name {
			return true
		}
	}
	return false
}
