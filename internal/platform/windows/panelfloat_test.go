//go:build windows

package windows

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestPanelStyleFloatsWhicheverWayItTakesFocus checks the extended style a
// panel opens with: it floats above the ordinary windows either way, and only
// one that never takes the focus says so. A panel without the topmost style
// sinks under a maximized window in front, which left the app running with no
// window in sight.
func TestPanelStyleFloatsWhicheverWayItTakesFocus(t *testing.T) {
	for _, noActivate := range []bool{false, true} {
		ex := panelExStyle(noActivate)
		if ex&wsExTopmost == 0 {
			t.Errorf("noActivate %v: ex style %#x; a panel floats above the ordinary windows", noActivate, ex)
		}
		if takes := ex&wsExNoActivate != 0; takes != noActivate {
			t.Errorf("noActivate %v: ex style %#x; WS_EX_NOACTIVATE set %v", noActivate, ex, takes)
		}
	}
	// The floating style is the only one it adds for a panel that takes the
	// focus: nothing else of a no-activate panel's.
	if plain := panelExStyle(false); plain != wsExTopmost {
		t.Errorf("a panel that takes the focus opens with %#x, not only the topmost style", plain)
	}
}

// TestPanelsFrontAtTheTopOfTheTopmostWindows checks when showing or fronting
// puts a window back at the top of the topmost ones: a panel does, whether or
// not it takes the focus, and a window does not.
func TestPanelsFrontAtTheTopOfTheTopmostWindows(t *testing.T) {
	cases := []struct {
		name string
		w    *window
		want bool
	}{
		{"a panel", &window{kind: driver.SurfacePanel, level: driver.WindowLevelFloating}, true},
		{"a panel that never takes the focus", &window{kind: driver.SurfacePanel, noActivate: true, level: driver.WindowLevelFloating}, true},
		{"a panel the app put among the windows", &window{kind: driver.SurfacePanel, level: driver.WindowLevelNormal}, false},
		{"a panel the app put under them", &window{kind: driver.SurfacePanel, level: driver.WindowLevelDesktop}, false},
		{"a window", &window{kind: driver.SurfaceWindow, level: driver.WindowLevelFloating}, false},
		{"a window at no level", &window{kind: driver.SurfaceWindow}, false},
	}
	for _, c := range cases {
		if got := c.w.raisesTopmost(); got != c.want {
			t.Errorf("%s raises to the topmost windows %v; want %v", c.name, got, c.want)
		}
	}
}

// TestTheAppSetsAPanelsLevel checks that SetWindowLevel keeps a panel at the
// level the app asks for: moved among the ordinary windows, it no longer goes
// back up when it shows, and asked to float again, it does. The style calls go
// to no window, which leaves the level the setLevel bookkeeping.
func TestTheAppSetsAPanelsLevel(t *testing.T) {
	panel := &window{kind: driver.SurfacePanel, level: driver.WindowLevelFloating}
	for _, c := range []struct {
		level  driver.WindowLevel
		floats bool
	}{
		{driver.WindowLevelNormal, false},
		{driver.WindowLevelDesktop, false},
		{driver.WindowLevelFloating, true},
	} {
		panel.setLevel(c.level)
		if panel.level != c.level {
			t.Errorf("level %v, not %v", panel.level, c.level)
		}
		if panel.raisesTopmost() != c.floats {
			t.Errorf("level %v: raises to the topmost windows %v; want %v", c.level, panel.raisesTopmost(), c.floats)
		}
	}
}
