package linux

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWebCSSPointAndSurfacePixel(t *testing.T) {
	x, y := webCSSPoint(driver.Point{X: 12, Y: 20}, 1.5)
	if x != 18 || y != 30 {
		t.Fatalf("webCSSPoint = (%v, %v), want (18, 30)", x, y)
	}
	x, y = webCSSPoint(driver.Point{X: 10, Y: 5}, 0)
	if x != 10 || y != 5 {
		t.Fatalf("webCSSPoint fallback = (%v, %v), want (10, 5)", x, y)
	}
	px, py := webSurfacePixel(18, 30, 4, 12, 2)
	if px != 44 || py != 84 {
		t.Fatalf("webSurfacePixel = (%d, %d), want (44, 84)", px, py)
	}
	px, py = webSurfacePixel(10.4, 20.6, 0, 0, -1)
	if px != 10 || py != 21 {
		t.Fatalf("webSurfacePixel fallback = (%d, %d), want (10, 21)", px, py)
	}
}

func TestWebKeyvalForNameAndRune(t *testing.T) {
	tests := []struct {
		name string
		want uint32
		ok   bool
	}{
		{"Enter", webKeyReturn, true},
		{"return", webKeyReturn, true},
		{"Tab", webKeyTab, true},
		{"Escape", webKeyEscape, true},
		{"Backspace", webKeyBackSpace, true},
		{"Delete", webKeyDelete, true},
		{"Space", webKeySpace, true},
		{" ", webKeySpace, true},
		{"ArrowLeft", webKeyLeft, true},
		{"right", webKeyRight, true},
		{"ArrowUp", webKeyUp, true},
		{"down", webKeyDown, true},
		{"Home", webKeyHome, true},
		{"End", webKeyEnd, true},
		{"PageUp", webKeyPageUp, true},
		{"PageDown", webKeyPageDown, true},
		{"F1", webKeyF1, true},
		{"f12", webKeyF1 + 11, true},
		{"a", 'a', true},
		{"A", 'A', true},
		{"!", '!', true},
		{"日", 0x01000000 | uint32('日'), true},
		{"", 0, false},
		{"UnknownKey", 0, false},
	}
	for _, tc := range tests {
		got, ok := webKeyvalForName(tc.name)
		if ok != tc.ok || got != tc.want {
			t.Errorf("webKeyvalForName(%q) = (%#x, %v), want (%#x, %v)", tc.name, got, ok, tc.want, tc.ok)
		}
	}
	if got := webKeyvalForRune('\n'); got != webKeyReturn {
		t.Errorf("webKeyvalForRune('\\n') = %#x, want %#x", got, webKeyReturn)
	}
	if got := webKeyvalForRune('\t'); got != webKeyTab {
		t.Errorf("webKeyvalForRune('\\t') = %#x, want %#x", got, webKeyTab)
	}
	if got := webKeyvalForRune(0x01); got != 0 {
		t.Errorf("webKeyvalForRune(0x01) = %#x, want 0", got)
	}
}

func TestChooseWebKeymapKey(t *testing.T) {
	code, shift, ok := chooseWebKeymapKey([]gdkKeymapKey{
		{keycode: 50, group: 1, level: 0},
		{keycode: 38, group: 0, level: 1},
		{keycode: 38, group: 0, level: 0},
	})
	if !ok || code != 38 || shift {
		t.Fatalf("chooseWebKeymapKey = (%d, %v, %v), want (38, false, true)", code, shift, ok)
	}
	code, shift, ok = chooseWebKeymapKey([]gdkKeymapKey{
		{keycode: 10, group: 0, level: 1},
		{keycode: 10, group: 0, level: 2},
	})
	if !ok || code != 10 || !shift {
		t.Fatalf("shifted chooseWebKeymapKey = (%d, %v, %v), want (10, true, true)", code, shift, ok)
	}
	if _, _, ok := chooseWebKeymapKey([]gdkKeymapKey{{keycode: 26, group: 0, level: 2}}); ok {
		t.Fatal("level 2 (AltGr) match should not be chosen")
	}
	if _, _, ok := chooseWebKeymapKey(nil); ok {
		t.Fatal("empty matches should not succeed")
	}
}

func TestWebInputTextSteps(t *testing.T) {
	steps := webInputTextSteps("Hi\r\n日")
	if len(steps) != 4 {
		t.Fatalf("len(steps) = %d, want 4: %+v", len(steps), steps)
	}
	if steps[0].keyval != 'H' || steps[0].text != "H" {
		t.Errorf("step 0 = %+v", steps[0])
	}
	if steps[1].keyval != 'i' || steps[1].text != "i" {
		t.Errorf("step 1 = %+v", steps[1])
	}
	if steps[2].keyval != webKeyReturn || steps[2].text != "\n" {
		t.Errorf("step 2 = %+v", steps[2])
	}
	if steps[3].keyval != 0x01000000|uint32('日') || steps[3].text != "日" {
		t.Errorf("step 3 = %+v", steps[3])
	}
	if got := webInputTextSteps(""); len(got) != 0 {
		t.Errorf("empty text steps = %+v", got)
	}
}
