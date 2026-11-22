package ios

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWebClickViewPoint(t *testing.T) {
	x, y := webClickViewPoint(driver.Point{X: 12, Y: 20}, 1.5)
	if x != 18 || y != 30 {
		t.Fatalf("webClickViewPoint = (%v, %v), want (18, 30)", x, y)
	}
	x, y = webClickViewPoint(driver.Point{X: 10, Y: 5}, 0)
	if x != 10 || y != 5 {
		t.Fatalf("webClickViewPoint fallback = (%v, %v), want (10, 5)", x, y)
	}
}

func TestShouldDeliverWebTap(t *testing.T) {
	if !shouldDeliverWebTap(webTapRecognizerInfo{enabled: true, isTap: true, tapsRequired: 1, touchesRequired: 1}) {
		t.Error("single-tap recognizer should receive tap")
	}
	if shouldDeliverWebTap(webTapRecognizerInfo{enabled: true, isTap: true, tapsRequired: 2, touchesRequired: 1}) {
		t.Error("double-tap recognizer should not receive single tap")
	}
	if shouldDeliverWebTap(webTapRecognizerInfo{enabled: true, isTap: true, tapsRequired: 1, touchesRequired: 2}) {
		t.Error("two-finger tap recognizer should not receive single tap")
	}
	if shouldDeliverWebTap(webTapRecognizerInfo{enabled: false, isTap: true, tapsRequired: 1, touchesRequired: 1}) {
		t.Error("disabled recognizer should not receive tap")
	}
	if shouldDeliverWebTap(webTapRecognizerInfo{enabled: true, excluded: true}) {
		t.Error("excluded recognizer should not receive tap")
	}
	if !shouldDeliverWebTap(webTapRecognizerInfo{enabled: true}) {
		t.Error("non-excluded touch recognizer should receive tap")
	}
}

func TestWebIOSKeyLookup(t *testing.T) {
	tests := []struct {
		name      string
		wantChars string
		wantUsage int
		wantType  int
		wantMods  int
	}{
		{"Enter", "\r", 0x28, webIOSPressTypeNone, 0},
		{"return", "\r", 0x28, webIOSPressTypeNone, 0},
		{"Tab", "\t", 0x2b, webIOSPressTypeNone, 0},
		{"Escape", "\x1b", 0x29, webIOSPressTypeNone, 0},
		{"Backspace", "\x7f", 0x2a, webIOSPressTypeNone, 0},
		{"Delete", "\uf728", 0x4c, webIOSPressTypeNone, 0},
		{"Space", " ", 0x2c, webIOSPressTypeNone, 0},
		{"ArrowLeft", "\uf702", 0x50, webIOSPressTypeLeftArrow, 0},
		{"right", "\uf703", 0x4f, webIOSPressTypeRightArrow, 0},
		{"ArrowUp", "\uf700", 0x52, webIOSPressTypeUpArrow, 0},
		{"down", "\uf701", 0x51, webIOSPressTypeDownArrow, 0},
		{"Home", "\uf729", 0x4a, webIOSPressTypeNone, 0},
		{"End", "\uf72b", 0x4d, webIOSPressTypeNone, 0},
		{"PageUp", "\uf72c", 0x4b, webIOSPressTypeNone, 0},
		{"PageDown", "\uf72d", 0x4e, webIOSPressTypeNone, 0},
		{"F1", "\uf704", 0x3a, webIOSPressTypeNone, 0},
		{"f12", "\uf70f", 0x45, webIOSPressTypeNone, 0},
		{"a", "a", 0x04, webIOSPressTypeNone, 0},
		{"A", "A", 0x04, webIOSPressTypeNone, webIOSModifierShift},
	}
	for _, tc := range tests {
		got, ok := webIOSKeyLookup(tc.name)
		if !ok {
			t.Fatalf("webIOSKeyLookup(%q) not found", tc.name)
		}
		if got.chars != tc.wantChars || got.usage != tc.wantUsage || got.pressType != tc.wantType || got.modifierFlags != tc.wantMods {
			t.Errorf("webIOSKeyLookup(%q) = %+v, want chars=%q usage=%#x type=%d mods=%#x",
				tc.name, got, tc.wantChars, tc.wantUsage, tc.wantType, tc.wantMods)
		}
	}
	if _, ok := webIOSKeyLookup(""); ok {
		t.Error("empty key name should not resolve")
	}
	if _, ok := webIOSKeyLookup("NotAKey"); ok {
		t.Error("unknown key name should not resolve")
	}
}

func TestWebIOSTextSteps(t *testing.T) {
	steps := webIOSTextSteps("aZ0!\r\n日")
	if len(steps) != 6 {
		t.Fatalf("len(steps) = %d, want 6: %+v", len(steps), steps)
	}
	if steps[0].usage != 0x04 || steps[0].modifierFlags != 0 || steps[0].insertText {
		t.Errorf("step 0 ('a') = %+v", steps[0])
	}
	if steps[1].usage != 0x1d || steps[1].plain != "z" || steps[1].modifierFlags != webIOSModifierShift {
		t.Errorf("step 1 ('Z') = %+v", steps[1])
	}
	if steps[2].usage != 0x27 || steps[2].modifierFlags != 0 {
		t.Errorf("step 2 ('0') = %+v", steps[2])
	}
	if steps[3].usage != 0x1e || steps[3].plain != "1" || steps[3].modifierFlags != webIOSModifierShift {
		t.Errorf("step 3 ('!') = %+v", steps[3])
	}
	if steps[4].usage != 0x28 || steps[4].chars != "\r" {
		t.Errorf("step 4 (CRLF) = %+v", steps[4])
	}
	if !steps[5].insertText || steps[5].chars != "日" {
		t.Errorf("step 5 ('日') = %+v", steps[5])
	}
	if got := webIOSTextSteps(""); len(got) != 0 {
		t.Errorf("empty text steps = %+v", got)
	}
}
