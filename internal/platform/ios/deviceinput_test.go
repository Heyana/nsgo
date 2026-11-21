//go:build !ios || nsgotest || nsgoautomation

package ios

import (
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestSimulatorDetectionAndHIDFallback(t *testing.T) {
	if isSimulatorEnv(func(string) string { return "" }) {
		t.Fatal("empty env reported as simulator")
	}
	if !isSimulatorEnv(func(k string) string {
		if k == "SIMULATOR_MODEL_IDENTIFIER" {
			return "iPhone17,1"
		}
		return ""
	}) {
		t.Fatal("SIMULATOR_MODEL_IDENTIFIER not detected as simulator")
	}
	if !isSimulatorEnv(func(k string) string {
		if k == "SIMULATOR_UDID" {
			return "00000000-0000-0000-0000-000000000000"
		}
		return ""
	}) {
		t.Fatal("SIMULATOR_UDID not detected as simulator")
	}

	called := false
	if shouldUseSimulatorHID(false, func() error {
		called = true
		return nil
	}) {
		t.Fatal("physical device used simulator HID")
	}
	if called {
		t.Fatal("physical device attempted to load private simulator HID")
	}

	if shouldUseSimulatorHID(true, func() error { return errors.New("missing BackBoardServices") }) {
		t.Fatal("simulator with missing HID did not fall back to device path")
	}
	if !shouldUseSimulatorHID(true, func() error { return nil }) {
		t.Fatal("simulator with working HID did not use simulator path")
	}
}

func TestResolveDevicePress(t *testing.T) {
	tests := []struct {
		name  string
		chain []deviceHitNode
		hold  bool
		mods  driver.Modifiers
		want  devicePressTarget
	}{
		{
			name: "text editor wins over enclosing pressable view",
			chain: []deviceHitNode{
				{isTextEditor: true},
				{isNSGoView: true, pressable: true},
			},
			want: devicePressTarget{kind: devicePressFocusText, index: 0},
		},
		{
			name: "native switch activates directly",
			chain: []deviceHitNode{
				{isSwitch: true, isControl: true},
				{isNSGoView: true, pressable: true},
			},
			want: devicePressTarget{kind: devicePressSwitch, index: 0},
		},
		{
			name: "native tab bar selects tapped tab",
			chain: []deviceHitNode{
				{isNSGoView: false, isAxElement: true},
				{isTabBar: true},
			},
			want: devicePressTarget{kind: devicePressTabBar, index: 1},
		},
		{
			name: "native UIControl fires control events",
			chain: []deviceHitNode{
				{isControl: true},
			},
			want: devicePressTarget{kind: devicePressControl, index: 0},
		},
		{
			name: "NSGo view with accessibility activate takes accessibilityActivate on plain tap",
			chain: []deviceHitNode{
				{isNSGoView: true, pressable: true, hasActivate: true},
			},
			want: devicePressTarget{kind: devicePressAccessibility, index: 0},
		},
		{
			name: "NSGo view with accessibility activate uses engine press on long press",
			chain: []deviceHitNode{
				{isNSGoView: true, pressable: true, hasActivate: true},
			},
			hold: true,
			want: devicePressTarget{kind: devicePressView, index: 0},
		},
		{
			name: "NSGo view with link at point uses engine press to preserve span index",
			chain: []deviceHitNode{
				{isNSGoView: true, pressable: true, hasActivate: true, hasLinkAtPoint: true},
			},
			want: devicePressTarget{kind: devicePressView, index: 0},
		},
		{
			name: "NSGo view with modifiers uses engine press to preserve modifiers",
			chain: []deviceHitNode{
				{isNSGoView: true, pressable: true, hasActivate: true},
			},
			mods: driver.Modifiers{Shift: true},
			want: devicePressTarget{kind: devicePressView, index: 0},
		},
		{
			name: "NSGo pressable ancestor found through non-pressable text child",
			chain: []deviceHitNode{
				{isNSGoView: true, isAxElement: true},
				{isNSGoView: true, pressable: true},
			},
			want: devicePressTarget{kind: devicePressView, index: 1},
		},
		{
			name: "non-NSGo accessibility element falls back to accessibilityActivate",
			chain: []deviceHitNode{
				{isAxElement: true},
				{},
			},
			want: devicePressTarget{kind: devicePressAccessibility, index: 0},
		},
		{
			name:  "empty chain resolves to none",
			chain: nil,
			want:  devicePressTarget{kind: devicePressNone, index: -1},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveDevicePress(tc.chain, tc.hold, tc.mods)
			if got != tc.want {
				t.Fatalf("resolveDevicePress = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestClampScrollOffset(t *testing.T) {
	// Vertical scroll view with top inset 20, content 500pt tall in a 200pt viewport:
	// valid Y range is [-20, 300].
	to, moved := clampScrollOffset(
		driver.Point{X: 0, Y: 0},
		driver.Point{X: 0, Y: 120},
		driver.Size{Width: 300, Height: 500},
		driver.Size{Width: 300, Height: 200},
		20, 0, 0, 0,
	)
	if !moved || to != (driver.Point{X: 0, Y: 120}) {
		t.Fatalf("scroll down = %+v, moved=%v", to, moved)
	}
	// Already at bottom edge: cannot move further down (lets nested outer scroll view take it).
	to, moved = clampScrollOffset(
		driver.Point{X: 0, Y: 300},
		driver.Point{X: 0, Y: 80},
		driver.Size{Width: 300, Height: 500},
		driver.Size{Width: 300, Height: 200},
		20, 0, 0, 0,
	)
	if moved || to != (driver.Point{X: 0, Y: 300}) {
		t.Fatalf("at bottom edge = %+v, moved=%v", to, moved)
	}
	// Scrolling past top clamps to -insetTop.
	to, moved = clampScrollOffset(
		driver.Point{X: 0, Y: 10},
		driver.Point{X: 0, Y: -100},
		driver.Size{Width: 300, Height: 500},
		driver.Size{Width: 300, Height: 200},
		20, 0, 0, 0,
	)
	if !moved || to != (driver.Point{X: 0, Y: -20}) {
		t.Fatalf("scroll to top inset = %+v, moved=%v", to, moved)
	}
}

func TestResolveAlertAction(t *testing.T) {
	actions := []driver.AlertAction{
		{Label: "Delete", Role: driver.AlertDestructive},
		{Label: "Cancel", Role: driver.AlertCancel},
	}
	if label, idx, err := resolveAlertAction(actions, 0); err != nil || label != "Delete" || idx != 0 {
		t.Fatalf("index 0 = %q, %d, %v", label, idx, err)
	}
	if label, idx, err := resolveAlertAction(actions, -1); err != nil || label != "Cancel" || idx != 1 {
		t.Fatalf("dismiss = %q, %d, %v", label, idx, err)
	}
	if label, idx, err := resolveAlertAction(nil, -1); err != nil || label != "OK" || idx != -1 {
		t.Fatalf("empty actions = %q, %d, %v", label, idx, err)
	}
	if _, _, err := resolveAlertAction([]driver.AlertAction{{Label: "Keep"}}, -1); err == nil {
		t.Fatal("undismissable alert unexpectedly succeeded")
	}
}

func TestParseDeviceKeyAndHeldState(t *testing.T) {
	name, mods, flags, ok := parseDeviceKey("Mod+Shift+K")
	if !ok || name != "k" || !mods.Platform || !mods.Shift || mods.Control || mods.Alt || flags != (deviceModMeta|deviceModShift) {
		t.Fatalf("parse Mod+Shift+K = %q, %+v, %#x, %v", name, mods, flags, ok)
	}
	if name, _, _, ok := parseDeviceKey("Return"); !ok || name != "enter" {
		t.Fatalf("parse Return = %q, %v", name, ok)
	}
	if _, _, _, ok := parseDeviceKey("nonexistent"); ok {
		t.Fatal("unknown key parsed as valid")
	}

	var st deviceKeyState
	mods, flags, _, wasHeld := st.down("shift", 0)
	if wasHeld || !mods.Shift || flags != deviceModShift {
		t.Fatalf("shift down = %+v, %#x, %v", mods, flags, wasHeld)
	}
	mods, flags, _, wasHeld = st.down("a", 7)
	if wasHeld || !mods.Shift || flags != deviceModShift {
		t.Fatalf("a down while shift held = %+v, %#x, %v", mods, flags, wasHeld)
	}
	id, wasHeld, mods, flags := st.up("a")
	if !wasHeld || id != 7 || !mods.Shift || flags != deviceModShift {
		t.Fatalf("a up = id=%d held=%v mods=%+v flags=%#x", id, wasHeld, mods, flags)
	}
	id, wasHeld, mods, flags = st.up("shift")
	if !wasHeld || id != 0 || mods != (driver.Modifiers{}) || flags != 0 {
		t.Fatalf("shift up = id=%d held=%v mods=%+v flags=%#x", id, wasHeld, mods, flags)
	}
}

func TestInterpolateDragPath(t *testing.T) {
	pts := interpolateDragPath([]driver.Point{{X: 10, Y: 20}, {X: 10, Y: 55}})
	if len(pts) < 4 || pts[0] != (driver.Point{X: 10, Y: 20}) || pts[len(pts)-1] != (driver.Point{X: 10, Y: 55}) {
		t.Fatalf("interpolated path = %+v", pts)
	}
}
