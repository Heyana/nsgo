//go:build windows

package windows

import (
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestEmbeddedSystemKeysReachTheApp drives the window procedure of a NewView
// child, which receives keys after the host's loop has translated them.
func TestEmbeddedSystemKeysReachTheApp(t *testing.T) {
	if testing.Short() {
		t.Skip("opens windows")
	}
	events := make(chan string, 8)
	app := func(cx *core.Scope) core.Node {
		cx.OnKeyWith("alt+s", core.KeyOptions{When: core.KeyAlways}, func() { events <- "shortcut" })
		return core.View{Focusable: func() bool { return true },
			OnKeyDown: func(e core.KeyEvent) bool {
				events <- e.Shortcut()
				return e.Key == "f10"
			}, Style: core.Style{Width: driver.Pt(240), Height: driver.Pt(100)}}
	}
	host := startApp(t, "nsgo-embedded-system-keys", func(*core.Scope) core.Node { return core.View{} }, 300, 160)
	var embedded *View
	var err error
	onMain(t, host, func(*window) {
		embedded, err = NewView(app, Options{Parent: host})
		if err != nil {
			return
		}
		setWindowPos.Call(embedded.Handle(), 0, 0, 0, 240, 100, swpNoActivate|swpNoZOrder)
		setFocus.Call(embedded.Handle())
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { onMain(t, host, func(*window) { embedded.Close() }) })
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		var ready bool
		onMain(t, host, func(*window) {
			w := embedded.w
			if w.focusedView() == nil {
				w.tab(false)
			}
			ready = w.focusedView() != nil && len(w.keys) != 0
		})
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the embedded view did not mount and focus")
		}
	}
	waitEvent := func(want string) {
		t.Helper()
		select {
		case got := <-events:
			if got != want {
				t.Fatalf("event = %q, want %q", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %q", want)
		}
	}
	const altContext = 1 << 29
	getState, setState := user32.NewProc("GetKeyboardState"), user32.NewProc("SetKeyboardState")
	onMain(t, host, func(*window) {
		var saved [256]byte
		getState.Call(uintptr(unsafe.Pointer(&saved)))
		defer setState.Call(uintptr(unsafe.Pointer(&saved)))
		var state [256]byte
		setState.Call(uintptr(unsafe.Pointer(&state)))

		hwnd := embedded.Handle()
		wndProc(hwnd, wmSysKeyDown, vkF10, 1)
		if !embedded.w.skipSysChar || embedded.w.skipChar {
			t.Error("consumed F10 did not isolate system character translation")
		}

		state[vkMenu] = 0x80
		setState.Call(uintptr(unsafe.Pointer(&state)))
		wndProc(hwnd, wmSysKeyDown, vkSpace, altContext|1)
		if embedded.w.skipSysChar {
			t.Error("unhandled Alt+Space was swallowed after F10")
		}

		wndProc(hwnd, wmSysKeyDown, 'S', altContext|1)
		if !embedded.w.skipSysChar {
			t.Error("handled Alt+S did not suppress its translated mnemonic")
		}
		wndProc(hwnd, wmSysChar, 's', altContext|1)
		if embedded.w.skipSysChar {
			t.Error("the suppression survived the translated character")
		}
	})
	waitEvent("f10")
	waitEvent("alt+space")
	waitEvent("alt+s")
	waitEvent("shortcut")
}
