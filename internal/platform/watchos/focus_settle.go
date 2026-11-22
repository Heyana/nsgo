package watchos

import "github.com/nsgo-dev/nsgo/internal/driver"

// fieldCanFocus reports whether a watchOS TextField/SecureField accepts focus
// in Host.swift, which disables both Disabled and Readonly fields.
func fieldCanFocus(c driver.InputConfig) bool {
	return !c.Disabled && !c.Readonly
}

// pressFocusAction describes the native focus transition a tap at a point makes
// before EventPress runs: blur any previously focused field when focus leaves
// it, and focus the hit field when it accepts focus.
type pressFocusAction struct {
	blur  driver.NodeID
	focus driver.NodeID
}

func resolvePressFocus(focused, hitField driver.NodeID, hitCanFocus bool) pressFocusAction {
	if hitField != 0 && hitCanFocus {
		if focused == hitField {
			return pressFocusAction{}
		}
		return pressFocusAction{blur: focused, focus: hitField}
	}
	if focused != 0 {
		return pressFocusAction{blur: focused}
	}
	return pressFocusAction{}
}

// testSurfaceBusy reports whether watchOS still has queued mount batches,
// running animations, layout motions, or scroll glides/animations in flight.
func testSurfaceBusy(queuedBatches int, animatorEnding bool, motions int, scrollBusy bool) bool {
	return queuedBatches > 0 || animatorEnding || motions > 0 || scrollBusy
}
