package watchos

import "testing"

func TestInputIsolationDirect(t *testing.T) {
	var gate testInputGate

	// Normal apps and live automation (isolated=false) accept all direct input.
	if !gate.allowDirect(false) {
		t.Error("normal mode rejected direct input")
	}

	// Test mode (isolated=true or gate.isolated=true) rejects physical touches, text edits and alerts.
	if gate.allowDirect(true) {
		t.Error("test mode accepted physical input")
	}
	gate.isolated = true
	if gate.allowDirect(false) {
		t.Error("host-isolated test mode accepted physical input before Open")
	}

	// Scripted steps allow input, including nested scopes.
	gate.enter()
	gate.enter()
	if !gate.allowDirect(true) {
		t.Error("scripted step rejected input")
	}
	gate.leave()
	if !gate.allowDirect(true) {
		t.Error("inner leave ended outer scripted scope")
	}
	gate.leave()
	if gate.allowDirect(true) {
		t.Error("test mode stayed open after scripted scope ended")
	}

	// Extra leave does not underflow.
	gate.leave()
	gate.enter()
	if !gate.allowDirect(true) {
		t.Error("extra leave corrupted scripted depth")
	}
	gate.leave()
	if gate.allowDirect(true) {
		t.Error("test mode accepted physical input after leave")
	}
}

func TestInputIsolationScroll(t *testing.T) {
	var gate testInputGate
	const scrollID uint32 = 42

	// Normal mode accepts all scroll phases (finger drag, Digital Crown, glide, offset).
	for _, phase := range []uint8{phaseOffset, phaseDragging, phaseGliding, phaseRest, phaseAnimating} {
		if !gate.allowScroll(false, scrollID, phase) {
			t.Errorf("normal mode rejected scroll phase %d", phase)
		}
	}

	// Test mode allows SwiftUI's initial layout offset report once.
	if !gate.allowScroll(true, scrollID, phaseOffset) {
		t.Error("test mode rejected initial layout scroll offset")
	}
	// Unexpected subsequent offset (e.g., Digital Crown without drag phase) is rejected.
	if gate.allowScroll(true, scrollID, phaseOffset) {
		t.Error("test mode accepted unexpected physical scroll offset")
	}

	// Physical finger drag / Digital Crown gesture is rejected end-to-end.
	if gate.allowScroll(true, scrollID, phaseDragging) {
		t.Error("test mode accepted physical scroll drag")
	}
	if gate.allowScroll(true, scrollID, phaseOffset) {
		t.Error("test mode accepted offset during physical drag")
	}
	if gate.allowScroll(true, scrollID, phaseGliding) {
		t.Error("test mode accepted physical scroll glide")
	}
	if gate.allowScroll(true, scrollID, phaseOffset) {
		t.Error("test mode accepted offset during physical glide")
	}
	if gate.allowScroll(true, scrollID, phaseRest) {
		t.Error("test mode accepted rest ending physical scroll")
	}

	// Programmatic opScrollTo (testSurface.Scroll or Ref.ScrollTo) is allowed.
	gate.expectScroll(scrollID)
	if !gate.allowScroll(true, scrollID, phaseAnimating) {
		t.Error("test mode rejected animating phase for expected ScrollTo")
	}
	if !gate.allowScroll(true, scrollID, phaseOffset) {
		t.Error("test mode rejected offset for expected ScrollTo")
	}
	if !gate.allowScroll(true, scrollID, phaseRest) {
		t.Error("test mode rejected rest ending expected ScrollTo")
	}
	// After settling, stray physical offsets are rejected again.
	if gate.allowScroll(true, scrollID, phaseOffset) {
		t.Error("test mode accepted stray offset after expected ScrollTo settled")
	}

	// forgetScroll resets state for a reused node ID.
	gate.forgetScroll(scrollID)
	if !gate.allowScroll(true, scrollID, phaseOffset) {
		t.Error("reused scroll ID after forgetScroll did not accept initial layout offset")
	}
}

func TestInputIsolationNativeTouchAndCrown(t *testing.T) {
	var gate testInputGate
	const scrollID uint32 = 77

	// Physical touches, text edits and Digital Crown events are blocked when isolated.
	if gate.allowTouch(true, touchBegan) {
		t.Error("test mode accepted physical touch")
	}
	if gate.allowText(true, textFocused) || gate.allowText(true, textChanged) {
		t.Error("test mode accepted physical text event")
	}
	if gate.allowCrown(true) {
		t.Error("test mode accepted physical crown rotation")
	}
	// Normal mode accepts all of them.
	if !gate.allowTouch(false, touchBegan) || !gate.allowText(false, textChanged) || !gate.allowCrown(false) {
		t.Error("normal mode rejected user input")
	}

	// Native simulator HID touch window admits SwiftUI DragGesture callbacks,
	// TextField focus/blur callbacks, and ScrollView drags that begin during it.
	seq := gate.beginNativeTouch()
	if !gate.allowTouch(true, touchBegan) {
		t.Error("native touch window rejected touchBegan")
	}
	if !gate.touchSeenSince(seq) || gate.touchEndedSince(seq) {
		t.Error("touch sequence tracking mismatch after touchBegan")
	}
	if !gate.allowText(true, textFocused) || !gate.allowText(true, textBlurred) {
		t.Error("native touch window rejected TextField focus/blur")
	}
	if gate.allowText(true, textChanged) {
		t.Error("native touch window admitted stray textChanged edit")
	}
	// Scroll drag starting inside native touch window stays admitted through glide and rest.
	if !gate.allowScroll(true, scrollID, phaseDragging) {
		t.Error("native touch window rejected scripted scroll drag")
	}
	if !gate.allowScroll(true, scrollID, phaseOffset) {
		t.Error("native touch window rejected scripted scroll offset")
	}
	if !gate.allowTouch(true, touchEnded) {
		t.Error("native touch window rejected touchEnded")
	}
	if !gate.touchEndedSince(seq) {
		t.Error("touchEnded did not advance touchEndedSeq")
	}
	gate.endNativeTouch()

	// After finger lifts, ongoing glide and rest for that scripted scroll stay admitted.
	if !gate.allowScroll(true, scrollID, phaseGliding) {
		t.Error("scripted scroll glide rejected after finger lift")
	}
	if !gate.allowScroll(true, scrollID, phaseOffset) {
		t.Error("scripted scroll glide offset rejected")
	}
	if !gate.allowScroll(true, scrollID, phaseRest) {
		t.Error("scripted scroll rest rejected")
	}
	// Once rested, stray physical scroll drags and touches are rejected again.
	if gate.allowScroll(true, scrollID, phaseDragging) {
		t.Error("physical scroll drag admitted after scripted scroll rested")
	}
	if gate.allowTouch(true, touchBegan) {
		t.Error("physical touch admitted after native touch window closed")
	}

	// Native focus window admits TextField focus/blur while settling, then closes.
	gate.setNativeFocusWindow(true)
	if !gate.allowText(true, textFocused) || gate.allowText(true, textSubmitted) {
		t.Error("native focus window gating mismatch")
	}
	gate.setNativeFocusWindow(false)
	if gate.allowText(true, textFocused) {
		t.Error("textFocused admitted after native focus window closed")
	}

	// Scripted Digital Crown scope admits hostCrown and closes cleanly.
	gate.enter()
	if !gate.allowCrown(true) {
		t.Error("scripted scope rejected hostCrown")
	}
	gate.leave()
	if gate.allowCrown(true) {
		t.Error("hostCrown stayed open after scripted scope ended")
	}
}
