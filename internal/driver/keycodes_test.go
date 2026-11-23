package driver

import "testing"

// Every position's platform codes name it again, so a test host that presses a key by its position reports the position it pressed.
func TestKeyPositionCodesRoundTrips(t *testing.T) {
	for _, p := range keyPositions {
		pos, ok := KeyPositionCodes(p.name)
		if !ok {
			t.Fatalf("KeyPositionCodes(%q) found nothing", p.name)
		}
		if got := USBKeyCode(pos.USB); got != p.name {
			t.Errorf("%s: USB usage %#x names %q", p.name, pos.USB, got)
		}
		if pos.XKB != 0 {
			if got := XKBKeyCode(pos.XKB); got != p.name {
				t.Errorf("%s: XKB code %#x names %q", p.name, pos.XKB, got)
			}
		}
		if pos.Windows != 0 {
			if got := WinKeyCode(pos.Windows); got != p.name {
				t.Errorf("%s: scan code %#x names %q", p.name, pos.Windows, got)
			}
		}
		if pos.Mac != 0xffff {
			if got := AppKitKeyCode(pos.Mac); got != p.name {
				t.Errorf("%s: AppKit key code %#x names %q", p.name, pos.Mac, got)
			}
		}
	}
	if _, ok := KeyPositionCodes("KeyQwerty"); ok {
		t.Error("KeyPositionCodes found an unknown code")
	}
}
