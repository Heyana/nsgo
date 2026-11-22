package ios

import "testing"

func TestInputIsolation(t *testing.T) {
	physicalKinds := []struct {
		name string
		kind int
	}{
		{"touches", uiEventTouches},
		{"motion", uiEventMotion},
		{"remoteControl", uiEventRemoteControl},
		{"presses", uiEventPresses},
		{"scroll", uiEventScroll},
		{"hover", uiEventHover},
		{"transform", uiEventTransform},
	}

	var gate testInputGate
	const winA, winB uintptr = 0x1000, 0x2000

	// Normal apps and live automation (isolated=false) accept every event.
	for _, tc := range physicalKinds {
		if !gate.allowEvent(false, tc.kind, winA, false, 0, 0) {
			t.Errorf("normal mode rejected %s without HID", tc.name)
		}
		if !gate.allowEvent(false, tc.kind, winA, true, 0xdeadbeef, 1234) {
			t.Errorf("normal mode rejected %s with foreign HID", tc.name)
		}
	}

	// Test mode (isolated=true) rejects physical events without scripted identity.
	for _, tc := range physicalKinds {
		if gate.allowEvent(true, tc.kind, winA, false, 0, 0) {
			t.Errorf("test mode accepted physical %s without HID", tc.name)
		}
		if gate.allowEvent(true, tc.kind, winA, true, 0xdeadbeef, 1234) {
			t.Errorf("test mode accepted physical %s with foreign sender", tc.name)
		}
	}

	// Non-input UIEvent types remain allowed in test mode.
	if !gate.allowEvent(true, 99, winA, false, 0, 0) {
		t.Error("test mode suppressed non-input UIEventType")
	}

	// Simulator HID events tagged with testSenderID pass in test mode.
	for _, tc := range physicalKinds {
		if !gate.allowEvent(true, tc.kind, winA, true, testSenderID, 5000) {
			t.Errorf("test mode rejected scripted HID %s with testSenderID", tc.name)
		}
	}

	// Remembered (window, timestamp) passes even if sender was cleared, but not for another window or after eviction.
	gate.rememberHID(winA, 7777)
	if !gate.allowEvent(true, uiEventTouches, winA, true, 0, 7777) {
		t.Error("test mode rejected remembered HID timestamp on matching window")
	}
	if gate.allowEvent(true, uiEventTouches, winB, true, 0, 7777) {
		t.Error("test mode accepted remembered HID timestamp on another window")
	}
	for i := range len(gate.hid) {
		gate.rememberHID(winA, uint64(8000+i))
	}
	if gate.allowEvent(true, uiEventTouches, winA, true, 0, 7777) {
		t.Error("evicted HID timestamp was still accepted")
	}
	if !gate.allowEvent(true, uiEventTouches, winA, true, 0, uint64(8000+len(gate.hid)-1)) {
		t.Error("most recent HID timestamp was not kept")
	}

	// Synchronous scripted scope (physical device path) allows events without HID, including nested scopes.
	gate.enter()
	gate.enter()
	for _, tc := range physicalKinds {
		if !gate.allowEvent(true, tc.kind, winA, false, 0, 0) {
			t.Errorf("scripted step rejected %s", tc.name)
		}
	}
	gate.leave()
	if !gate.allowEvent(true, uiEventTouches, winA, false, 0, 0) {
		t.Error("inner leave ended outer scripted scope")
	}
	gate.leave()
	if gate.allowEvent(true, uiEventTouches, winA, false, 0, 0) {
		t.Error("test mode stayed open after scripted scope ended")
	}

	// Extra leave does not underflow.
	gate.leave()
	gate.enter()
	if !gate.allowEvent(true, uiEventPresses, winA, false, 0, 0) {
		t.Error("extra leave corrupted scripted depth")
	}
	gate.leave()
	if gate.allowEvent(true, uiEventPresses, winA, false, 0, 0) {
		t.Error("test mode accepted physical key after leave")
	}
}
