package windows

import "testing"

func TestInputIsolation(t *testing.T) {
	physical := []struct {
		name string
		msg  uint32
	}{
		{"WM_MOUSEACTIVATE", 0x0021},
		{"WM_CONTEXTMENU", 0x007B},
		{"WM_NCMOUSEMOVE", 0x00A0},
		{"WM_NCLBUTTONDOWN", 0x00A1},
		{"WM_KEYDOWN", 0x0100},
		{"WM_KEYUP", 0x0101},
		{"WM_CHAR", 0x0102},
		{"WM_SYSKEYDOWN", 0x0104},
		{"WM_SYSKEYUP", 0x0105},
		{"WM_UNICHAR", 0x0109},
		{"WM_GESTURE", 0x0119},
		{"WM_MOUSEMOVE", 0x0200},
		{"WM_LBUTTONDOWN", 0x0201},
		{"WM_LBUTTONUP", 0x0202},
		{"WM_RBUTTONDOWN", 0x0204},
		{"WM_RBUTTONUP", 0x0205},
		{"WM_MBUTTONDOWN", 0x0207},
		{"WM_MOUSEWHEEL", 0x020A},
		{"WM_XBUTTONDOWN", 0x020B},
		{"WM_MOUSEHWHEEL", 0x020E},
		{"WM_TOUCH", 0x0240},
		{"WM_POINTERUPDATE", 0x0245},
		{"WM_POINTERDOWN", 0x0246},
		{"WM_POINTERUP", 0x0247},
		{"WM_POINTERLEAVE", 0x024A},
		{"WM_POINTERCAPTURECHANGED", 0x024C},
		{"WM_MOUSEHOVER", 0x02A1},
		{"WM_NCMOUSELEAVE", 0x02A2},
		{"WM_MOUSELEAVE", 0x02A3},
		{"WM_APPCOMMAND", 0x0319},
	}
	nonInput := []struct {
		name string
		msg  uint32
	}{
		{"WM_DESTROY", 0x0002},
		{"WM_SIZE", 0x0005},
		{"WM_SETFOCUS", 0x0007},
		{"WM_KILLFOCUS", 0x0008},
		{"WM_PAINT", 0x000F},
		{"WM_CLOSE", 0x0010},
		{"WM_IME_STARTCOMPOSITION", 0x010D},
		{"WM_IME_ENDCOMPOSITION", 0x010E},
		{"WM_IME_COMPOSITION", 0x010F},
		{"WM_COMMAND", 0x0111},
		{"WM_SYSCOMMAND", 0x0112},
		{"WM_TIMER", 0x0113},
		{"WM_CAPTURECHANGED", 0x0215},
		{"WM_APP+1", 0x8001},
	}

	var gate testInputGate

	// Normal apps and live automation (isolated=false) accept every message.
	for _, tc := range physical {
		if !gate.allow(false, tc.msg) {
			t.Errorf("normal mode rejected %s (%#x)", tc.name, tc.msg)
		}
	}

	// Test mode (isolated=true) rejects physical input while keeping system/lifecycle messages.
	for _, tc := range physical {
		if gate.allow(true, tc.msg) {
			t.Errorf("test mode accepted physical %s (%#x)", tc.name, tc.msg)
		}
	}
	for _, tc := range nonInput {
		if !gate.allow(true, tc.msg) {
			t.Errorf("test mode suppressed non-input %s (%#x)", tc.name, tc.msg)
		}
	}

	// Scripted steps allow input even in test mode, including nested scopes.
	gate.enter()
	gate.enter()
	for _, tc := range physical {
		if !gate.allow(true, tc.msg) {
			t.Errorf("scripted step rejected %s (%#x)", tc.name, tc.msg)
		}
	}
	gate.leave()
	if !gate.allow(true, 0x0201) {
		t.Error("inner leave ended outer scripted scope")
	}
	gate.leave()
	if gate.allow(true, 0x0201) {
		t.Error("test mode stayed open after scripted scope ended")
	}

	// Extra leave does not underflow.
	gate.leave()
	gate.enter()
	if !gate.allow(true, 0x0100) {
		t.Error("extra leave corrupted scripted depth")
	}
	gate.leave()
	if gate.allow(true, 0x0100) {
		t.Error("test mode accepted key after leave")
	}
}
