//go:build linux && !android

package linux

import (
	"os"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// TestHiddenApplicationMenuKeyboard opens the application menu through the backend's GTK key callbacks while its title bar and menu bar are hidden.
func TestHiddenApplicationMenuKeyboard(t *testing.T) {
	if testing.Short() {
		t.Skip("opens GTK menus")
	}
	for _, mode := range []struct {
		name  string
		value core.MenuBarMode
	}{
		{"shown", core.MenuBarShown}, {"auto-hide", core.MenuBarAutoHide},
	} {
		for _, tc := range []struct {
			name    string
			key     uint32
			alt     bool
			state   uint32
			popup   bool
			chosen  string
			bindF10 bool
		}{
			{name: "alt", alt: true, popup: true},
			{name: "f10", key: gdkF10, popup: true},
			{name: "mnemonic", key: 'f', alt: true, popup: true},
			{name: "unicode", key: 'É', alt: true, popup: true},
			{name: "command", key: 'q', alt: true, chosen: "command"},
			{name: "unknown", key: 'j', alt: true},
			{name: "disabled", key: 'd', alt: true},
			{name: "window-menu", key: ' ', alt: true},
			{name: "close-window", key: 0xffc1, alt: true}, // F4
			{name: "context-menu", key: gdkF10, state: gdkShift},
			{name: "modified-alt", alt: true, state: gdkControl},
			{name: "binding", key: 'x', alt: true, chosen: "binding"},
			{name: "bound-f10", key: gdkF10, bindF10: true, chosen: "binding"},
		} {
			t.Run(mode.name+"/"+tc.name, func(t *testing.T) {
				chosen := make(chan string, 4)
				w := startTestWindow(t, func(cx *core.Scope) core.Node {
					cx.SetTitleBar(core.TitleBarHidden, 0)
					cx.SetMenuBar(mode.value)
					cx.OnKey("alt+x", func() { chosen <- "binding" })
					if tc.bindF10 {
						cx.OnKey("f10", func() { chosen <- "binding" })
					}
					cx.SetApplicationMenu([]core.MenuItem{
						{Label: "File", Items: []core.MenuItem{{Label: "Run", OnSelect: func() { chosen <- "file" }}}},
						{Label: "Édition", Items: []core.MenuItem{{Label: "Copy", OnSelect: func() { chosen <- "edition" }}}},
						{Label: "Quick", OnSelect: func() { chosen <- "command" }},
						{Label: "Disabled", Disabled: true, OnSelect: func() { chosen <- "disabled" }},
					})
					return core.Text{Content: "Hidden menu"}
				}, Options{Title: "nsgo-hidden-menu", Width: 320, Height: 180})
				deadline := time.Now().Add(5 * time.Second)
				for {
					var ready bool
					w.onLoop(t, func() {
						ready = w.chrome.titleBar == core.TitleBarHidden && len(w.chrome.menu) != 0 && gtkWidgetGetMapped(w.surface) != 0
					})
					if ready {
						break
					}
					if time.Now().After(deadline) {
						t.Fatal("hidden application menu did not mount")
					}
					time.Sleep(10 * time.Millisecond)
				}
				w.onLoop(t, func() {
					if tc.alt {
						keyPressedCallback(0, gdkAltL, 64, tc.state, w.handle)
					}
					if tc.key != 0 {
						state := tc.state
						if tc.alt {
							state |= gdkAlt
						}
						keyPressedCallback(0, tc.key, 41, state, w.handle)
						keyReleasedCallback(0, tc.key, 41, state, w.handle)
					}
					if tc.alt {
						keyReleasedCallback(0, gdkAltL, 64, tc.state|gdkAlt, w.handle)
					}
					if opened := w.popover != 0 && gtkWidgetGetVisible(w.popover) != 0; opened != tc.popup {
						t.Errorf("popup = %v, want %v", opened, tc.popup)
					}
					if w.menuBar != 0 {
						t.Error("the hidden window grew a menu bar")
					}
					if w.popover != 0 {
						gtkPopoverPopdown(w.popover)
					}
				})
				w.engine.Sync()
				if tc.chosen == "" {
					select {
					case got := <-chosen:
						t.Errorf("opening the menu ran %q", got)
					default:
					}
				}
				if tc.chosen != "" {
					select {
					case got := <-chosen:
						if got != tc.chosen {
							t.Errorf("chose %q, want %q", got, tc.chosen)
						}
					case <-time.After(5 * time.Second):
						t.Fatal("the command did not run")
					}
				}
			})
		}
	}
}

// TestHiddenApplicationMenuNativeKeys drives GTK's real event stream with XTest: the popup owns Escape and Return, and its first item is selected.
func TestHiddenApplicationMenuNativeKeys(t *testing.T) {
	if testing.Short() || os.Getenv("DISPLAY") == "" || os.Getenv("WAYLAND_DISPLAY") != "" {
		t.Skip("needs an X11 desktop with XTest")
	}
	for _, tc := range []struct {
		name    string
		open    []uint32
		returns int
		editor  bool
	}{
		{"alt", []uint32{gdkAltL}, 2, false},
		{"f10", []uint32{gdkF10}, 2, false},
		{"mnemonic", []uint32{gdkAltL, 'f'}, 1, false},
		{"editor", []uint32{gdkAltL, 'f'}, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chosen := make(chan struct{}, 4)
			w := startTestWindow(t, func(cx *core.Scope) core.Node {
				cx.SetTitleBar(core.TitleBarHidden, 0)
				item := core.MenuItem{Label: "Run", OnSelect: func() { chosen <- struct{}{} }}
				if tc.editor {
					item.Role = core.MenuSelectAll
				}
				cx.SetApplicationMenu([]core.MenuItem{{Label: "File", Items: []core.MenuItem{item}}})
				if tc.editor {
					return core.Input{DefaultValue: "hello", AutoFocus: true}
				}
				return core.Text{Content: "Native menu keys"}
			}, Options{Title: "nsgo-hidden-menu-native", Width: 320, Height: 180})
			wait := func(what string, ready func() bool) {
				t.Helper()
				deadline := time.Now().Add(5 * time.Second)
				for {
					var yes bool
					w.onLoop(t, func() { yes = ready() })
					if yes {
						return
					}
					if time.Now().After(deadline) {
						t.Fatal(what)
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			wait("the window did not map", func() bool {
				return w.chrome.titleBar == core.TitleBarHidden && len(w.chrome.menu) != 0 && gtkWidgetGetMapped(w.surface) != 0
			})
			var display uintptr
			var available bool
			w.onLoop(t, func() {
				available = w.canSendWebInput()
				if !available {
					return
				}
				var xid uintptr
				display, xid, _ = w.x11Window()
				w.focusWebX11(gdkDisplayGetDefault(), display, xid)
			})
			if !available {
				t.Skip("XTest is unavailable")
			}
			press := func(keys ...uint32) {
				t.Helper()
				w.onLoop(t, func() {
					for _, key := range keys {
						xWebInput.fakeKey(display, uint32(xWebInput.keysymToKeycode(display, uintptr(key))), 1, 0)
					}
					for i := len(keys) - 1; i >= 0; i-- {
						xWebInput.fakeKey(display, uint32(xWebInput.keysymToKeycode(display, uintptr(keys[i]))), 0, 0)
					}
					xWebInput.sync(display, 0)
				})
			}
			opened := func() bool { return w.popover != 0 && gtkWidgetGetMapped(w.popover) != 0 }
			if tc.editor {
				wait("the input did not focus", func() bool { return w.focusedEntry() != nil })
			}
			press(tc.open...)
			wait("the native menu key did not map a popup", opened)
			press(gdkEscape)
			wait("Escape did not close the popup", func() bool { return w.popover == 0 })
			press(tc.open...)
			wait("the menu did not reopen", opened)
			for range tc.returns {
				press(0xff0d) // Return
				time.Sleep(50 * time.Millisecond)
			}
			if tc.editor {
				wait("the editing role did not select the input", func() bool {
					v := w.focusedEntry()
					if v == nil {
						return false
					}
					var start, end int32
					return gtkEditableGetSelectionBounds(v.entry, &start, &end) != 0 && start == 0 && end == 5
				})
				w.engine.Sync()
				select {
				case <-chosen:
					t.Error("the app fallback ran instead of editing the field")
				default:
				}
			} else {
				select {
				case <-chosen:
				case <-time.After(5 * time.Second):
					t.Fatal("Return did not activate the menu's first item")
				}
			}
			wait("choosing the item did not close its menu", func() bool { return w.popover == 0 })
		})
	}
}
