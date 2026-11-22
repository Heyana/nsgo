package ios

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestRemoteArrowName(t *testing.T) {
	want := map[int]string{
		pressUpArrow:    "up",
		pressDownArrow:  "down",
		pressLeftArrow:  "left",
		pressRightArrow: "right",
		pressSelect:     "",
		pressMenu:       "",
		99:              "",
	}
	for pressType, w := range want {
		if got := remoteArrowName(pressType); got != w {
			t.Errorf("remoteArrowName(%d) = %q, want %q", pressType, got, w)
		}
	}
}

func TestIsTopmostCaptureProps(t *testing.T) {
	cases := []struct {
		name string
		p    driver.ViewProps
		want bool
	}{
		{"plain view", driver.ViewProps{}, false},
		{"topmost box-none", driver.ViewProps{Topmost: true, PointerEvents: driver.PointerEventsBoxNone}, false},
		{"topmost none", driver.ViewProps{Topmost: true, PointerEvents: driver.PointerEventsNone}, false},
		{"topmost auto", driver.ViewProps{Topmost: true, PointerEvents: driver.PointerEventsAuto}, true},
		{"topmost box-only", driver.ViewProps{Topmost: true, PointerEvents: driver.PointerEventsBoxOnly}, true},
	}
	for _, tc := range cases {
		if got := isTopmostCaptureProps(tc.p); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestTVMenuGoesBack(t *testing.T) {
	if tvMenuGoesBack(0, false, false) {
		t.Error("Menu went back with no history, modal, or capture overlay")
	}
	if !tvMenuGoesBack(1, false, false) {
		t.Error("Menu did not go back with stack history")
	}
	if !tvMenuGoesBack(0, true, false) {
		t.Error("Menu did not go back with a modal open")
	}
	if !tvMenuGoesBack(0, false, true) {
		t.Error("Menu did not go back with a topmost capture overlay active")
	}
}

func TestTVFocusAllowed(t *testing.T) {
	if !tvFocusAllowed(false, false, false, false, false) {
		t.Error("plain view blocked with no modal or capture")
	}
	// Modal open: modal content and pass-through Topmost overlays (layout audit readout) stay focusable; views behind the modal do not.
	if tvFocusAllowed(false, false, false, true, false) {
		t.Error("view behind modal stayed focusable")
	}
	if !tvFocusAllowed(false, false, true, true, false) {
		t.Error("view inside modal was blocked")
	}
	if !tvFocusAllowed(false, false, false, true, true) {
		t.Error("topmost overlay view was blocked while modal was open")
	}
	// Capture overlay open (ruler): only descendants of the capture overlay take focus, even over a modal or another topmost overlay.
	if tvFocusAllowed(false, true, true, true, true) {
		t.Error("view outside capture overlay took focus")
	}
	if !tvFocusAllowed(true, true, false, true, true) {
		t.Error("view inside capture overlay was blocked")
	}
}
