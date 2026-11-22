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
