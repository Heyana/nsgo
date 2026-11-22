package linux

import "testing"

func TestInputIsolation(t *testing.T) {
	physical := []struct {
		name string
		typ  int32
	}{
		{"GDK_MOTION_NOTIFY", 1},
		{"GDK_BUTTON_PRESS", 2},
		{"GDK_BUTTON_RELEASE", 3},
		{"GDK_KEY_PRESS", 4},
		{"GDK_KEY_RELEASE", 5},
		{"GDK_ENTER_NOTIFY", 6},
		{"GDK_LEAVE_NOTIFY", 7},
		{"GDK_PROXIMITY_IN", 9},
		{"GDK_PROXIMITY_OUT", 10},
		{"GDK_SCROLL", 15},
		{"GDK_TOUCH_BEGIN", 17},
		{"GDK_TOUCH_UPDATE", 18},
		{"GDK_TOUCH_END", 19},
		{"GDK_TOUCH_CANCEL", 20},
		{"GDK_TOUCHPAD_SWIPE", 21},
		{"GDK_TOUCHPAD_PINCH", 22},
		{"GDK_TOUCHPAD_HOLD", 23},
		{"GDK_PAD_BUTTON_PRESS", 24},
		{"GDK_PAD_STRIP", 27},
	}
	nonInput := []struct {
		name string
		typ  int32
	}{
		{"GDK_DELETE", 0},
		{"GDK_FOCUS_CHANGE", 8},
		{"GDK_DRAG_ENTER", 11},
		{"GDK_DROP_START", 14},
		{"GDK_GRAB_BROKEN", 16},
	}

	var gate testInputGate

	// Normal apps and live automation (isolated=false) accept all callbacks and events.
	if !gate.allow(false) {
		t.Error("normal mode rejected callback")
	}
	for _, tc := range physical {
		if !gate.allowGDKEvent(false, tc.typ) {
			t.Errorf("normal mode rejected %s (%d)", tc.name, tc.typ)
		}
	}

	// Test mode (isolated=true) rejects physical input while keeping non-input events.
	if gate.allow(true) {
		t.Error("test mode accepted physical callback")
	}
	for _, tc := range physical {
		if gate.allowGDKEvent(true, tc.typ) {
			t.Errorf("test mode accepted physical %s (%d)", tc.name, tc.typ)
		}
	}
	for _, tc := range nonInput {
		if !gate.allowGDKEvent(true, tc.typ) {
			t.Errorf("test mode suppressed non-input %s (%d)", tc.name, tc.typ)
		}
	}

	// Scripted steps allow input in test mode, including nested scopes.
	gate.enter()
	gate.enter()
	if !gate.allow(true) {
		t.Error("scripted step rejected callback")
	}
	for _, tc := range physical {
		if !gate.allowGDKEvent(true, tc.typ) {
			t.Errorf("scripted step rejected %s (%d)", tc.name, tc.typ)
		}
	}
	gate.leave()
	if !gate.allow(true) {
		t.Error("inner leave ended outer scripted scope")
	}
	gate.leave()
	if gate.allow(true) {
		t.Error("test mode stayed open after scripted scope ended")
	}

	// Extra leave does not underflow.
	gate.leave()
	gate.enter()
	if !gate.allow(true) {
		t.Error("extra leave corrupted scripted depth")
	}
	gate.leave()
	if gate.allow(true) {
		t.Error("test mode accepted callback after leave")
	}
}
