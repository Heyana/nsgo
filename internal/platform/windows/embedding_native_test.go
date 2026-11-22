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

// TestEmbeddedLifecycleFollowsRootWindow embeds two views in a plain Win32
// host window (not an NSGo window) and checks that minimizing and restoring
// that root window updates Env.Minimized and Env.Lifecycle for both views,
// and keeps updating the second after the first closes.
func TestEmbeddedLifecycleFollowsRootWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("opens windows")
	}
	driverHost := startApp(t, "nsgo-embedded-lifecycle-driver", func(*core.Scope) core.Node { return core.View{} }, 240, 160)

	var plainRoot uintptr
	var first, second *View
	firstScope, secondScope := make(chan *core.Scope, 1), make(chan *core.Scope, 1)
	makeApp := func(out chan *core.Scope) func(*core.Scope) core.Node {
		return func(cx *core.Scope) core.Node {
			select {
			case out <- cx:
			default:
			}
			return core.View{Style: core.Style{Width: driver.Pt(120), Height: driver.Pt(80)}}
		}
	}

	var err error
	onMain(t, driverHost, func(*window) {
		instance, _, _ := getModuleHandleW.Call(0)
		className := wide("NsgoEmbeddedLifecycleHost")
		wc := wndClassEx{
			WndProc:   defWindowProcW.Addr(),
			Instance:  instance,
			ClassName: className,
		}
		wc.Size = uint32(unsafe.Sizeof(wc))
		registerClassExW.Call(uintptr(unsafe.Pointer(&wc)))

		plainRoot, _, _ = createWindowExW.Call(0, uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(wide("nsgo-plain-host"))), wsOverlappedWindow|wsClipChildren,
			cwUseDefault, cwUseDefault, 320, 200, 0, 0, instance, 0)
		if plainRoot == 0 {
			return
		}
		showWindow.Call(plainRoot, swShow)

		first, err = NewView(makeApp(firstScope), Options{Parent: plainRoot})
		if err != nil {
			return
		}
		setWindowPos.Call(first.Handle(), 0, 0, 0, 120, 80, swpNoActivate|swpNoZOrder)

		second, err = NewView(makeApp(secondScope), Options{Parent: plainRoot})
		if err != nil {
			return
		}
		setWindowPos.Call(second.Handle(), 0, 120, 0, 120, 80, swpNoActivate|swpNoZOrder)
	})
	if plainRoot == 0 {
		t.Fatal("could not create plain Win32 host window")
	}
	t.Cleanup(func() {
		onMain(t, driverHost, func(*window) {
			if first != nil {
				first.Close()
			}
			if second != nil {
				second.Close()
			}
			destroyWindow.Call(plainRoot)
		})
	})
	if err != nil {
		t.Fatal(err)
	}

	cx1, cx2 := <-firstScope, <-secondScope
	type state struct {
		lifecycle core.Lifecycle
		minimized bool
	}
	read := func(cx *core.Scope) state {
		got := make(chan state, 1)
		cx.Post(func() {
			got <- core.Untracked(cx, func() state { return state{cx.Env().Lifecycle(), cx.Env().Minimized()} })
		})
		return <-got
	}
	shown := func(s state) bool { return s.lifecycle != core.LifecycleBackground && !s.minimized }
	hidden := func(s state) bool { return s.lifecycle == core.LifecycleBackground && s.minimized }
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); !ok(); time.Sleep(20 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s", what)
			}
		}
	}

	waitFor("embedded views did not report shown", func() bool {
		return shown(read(cx1)) && shown(read(cx2))
	})

	onMain(t, driverHost, func(*window) { showWindow.Call(plainRoot, swMinimize) })
	waitFor("embedded views did not follow root minimize", func() bool {
		return hidden(read(cx1)) && hidden(read(cx2))
	})

	// Close the first embedded view while minimized; the second must still follow restore.
	onMain(t, driverHost, func(*window) {
		first.Close()
		first = nil
		showWindow.Call(plainRoot, swRestore)
	})
	waitFor("remaining embedded view did not follow root restore", func() bool {
		return shown(read(cx2))
	})
}
