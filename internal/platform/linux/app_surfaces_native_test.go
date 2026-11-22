//go:build linux && !android

package linux

import (
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestAppSurfacesShareStateAndPanelsFollowNativeFocus runs the app as a
// function on its hidden own surface. A real X11 keyboard edits a panel's
// GtkEntry, its sibling window sees the shared state, and native focus loss
// hides the panel in both GTK and the app's Window handle.
func TestAppSurfacesShareStateAndPanelsFollowNativeFocus(t *testing.T) {
	if testing.Short() {
		t.Skip("opens windows and sends X11 input")
	}
	type appState struct {
		cx          *core.Scope
		main, panel *core.Window
		content     func(*core.Scope) core.Node
	}
	ready := make(chan appState, 1)
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		text := core.NewSignal(cx, "")
		content := func(cx *core.Scope) core.Node {
			cx.Effect(func() { cx.SetTitle("nsgo1374-window|" + text.Get()) })
			return core.Text{ContentFunc: text.Get}
		}
		main := cx.OpenWindow(core.WindowOptions{Title: "nsgo1374-window", Width: 240, Height: 120}, content)
		panel := cx.OpenPanel(core.PanelOptions{Width: 200, Height: 80, HideOnBlur: true}, func(cx *core.Scope) core.Node {
			cx.Effect(func() { cx.SetTitle("nsgo1374-panel|" + text.Get()) })
			return core.Input{Value: text.Get, OnChange: text.Set, AutoFocus: true, Style: core.Style{Width: core.Pt(180), Height: core.Pt(36)}}
		})
		ready <- appState{cx, main, panel, content}
	})
	own := startTestWindow(t, app, Options{Title: "nsgo1374-own", Width: 240, Height: 120, Hidden: true})
	defer own.onLoop(t, own.close)
	state := <-ready
	if state.main == nil || state.panel == nil {
		t.Fatal("the app function could not open its surfaces")
	}
	wait := func(what string, condition func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			ok := false
			own.onLoop(t, func() { ok = condition() })
			if ok {
				return
			}
		}
		t.Fatalf("timed out waiting for %s", what)
	}
	var main, panel *window
	wait("both attached surfaces", func() bool {
		windowHandles.Range(func(_, value any) bool {
			w := value.(*window)
			if w.opener == own && !w.closing {
				switch w.kind {
				case driver.SurfaceWindow:
					main = w
				case driver.SurfacePanel:
					panel = w
				}
			}
			return true
		})
		return main != nil && panel != nil && len(panel.entries) > 0
	})
	own.onLoop(t, func() {
		if gtkWidgetGetMapped(own.win) != 0 || gtkWidgetGetVisible(own.win) != 0 {
			t.Error("the app's own surface showed a window")
		}
		if gtkWidgetGetVisible(panel.win) != 0 {
			t.Error("a newly opened panel was visible before Show")
		}
		if main.engine == nil || panel.engine == nil || main.opener != own || panel.opener != own {
			t.Error("surfaces did not attach to the app")
		}
	})
	waitForTitle(t, main, "nsgo1374-window|")

	var display, mainXID, panelXID uintptr
	own.onLoop(t, func() {
		display, mainXID, _ = main.x11Window()
		_, panelXID, _ = panel.x11Window()
	})
	if display == 0 || mainXID == 0 || panelXID == 0 {
		t.Skip("native keyboard/focus acceptance needs an X11 display")
	}
	libX, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Skip(err)
	}
	defer purego.Dlclose(libX)
	libTest, err := purego.Dlopen("libXtst.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Skip("native keyboard acceptance needs XTest")
	}
	defer purego.Dlclose(libTest)
	var focus func(uintptr, uintptr, int32, uintptr) int32
	var code func(uintptr, uintptr) uint8
	var key func(uintptr, uint32, int32, uintptr) int32
	purego.RegisterLibFunc(&focus, libX, "XSetInputFocus")
	purego.RegisterLibFunc(&code, libX, "XKeysymToKeycode")
	purego.RegisterLibFunc(&key, libTest, "XTestFakeKeyEvent")
	showAndType := func(letter byte, want string) {
		t.Helper()
		state.cx.Post(state.panel.Show)
		wait("panel map", func() bool { return gtkWidgetGetMapped(panel.win) != 0 })
		own.onLoop(t, func() { focus(display, panelXID, 1, 0); x11Flush(display) })
		wait("panel keyboard focus", func() bool { return gtkWindowIsActive(panel.win) != 0 && panel.focusedEntry() != nil })
		own.onLoop(t, func() {
			keycode := uint32(code(display, uintptr(letter)))
			key(display, keycode, 1, 0)
			key(display, keycode, 0, 0)
			x11Flush(display)
		})
		waitForTitle(t, main, "nsgo1374-window|"+want)
		waitForTitle(t, panel, "nsgo1374-panel|"+want)
	}
	blur := func() {
		t.Helper()
		own.onLoop(t, func() { focus(display, mainXID, 1, 0); x11Flush(display) })
		wait("native blur hides the panel", func() bool {
			return gtkWindowIsActive(main.win) != 0 && gtkWidgetGetVisible(panel.win) == 0 && !panel.visible
		})
		visibility := make(chan bool, 1)
		state.cx.Post(func() { visibility <- state.panel.Visible() })
		select {
		case visible := <-visibility:
			if visible {
				t.Error("the app's panel handle stayed visible after native blur")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("panel visibility was not reported")
		}
	}
	showAndType('a', "a")
	blur()
	showAndType('b', "ab")
	blur()
	state.cx.Post(func() { state.panel.Close(); state.main.Close() })
	wait("closed surfaces detach", func() bool {
		return panel.win == 0 && main.win == 0 && windowOf(panel.handle) == nil && windowOf(main.handle) == nil
	})
	own.onLoop(t, func() {
		if own.closing || own.lifecycle() != driver.LifecycleBackground {
			t.Error("the windowless explicit-shutdown app did not remain in the background")
		}
	})
	state.cx.Post(func() {
		state.cx.OpenWindow(core.WindowOptions{Title: "nsgo1374-window", Width: 240, Height: 120}, state.content)
	})
	var reopened *window
	wait("a window can reopen on the same app", func() bool {
		windowHandles.Range(func(_, value any) bool {
			w := value.(*window)
			if w.opener == own && w.kind == driver.SurfaceWindow && !w.closing {
				reopened = w
			}
			return true
		})
		return reopened != nil && reopened != main
	})
	waitForTitle(t, reopened, "nsgo1374-window|ab")
}
