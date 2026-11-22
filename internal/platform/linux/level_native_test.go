//go:build linux && !android

package linux

import (
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestWindowLevelAsksTheWindowManager listens on the X11 root window, as a window manager hears it, for the _NET_WM_STATE requests SetWindowLevel sends: ABOVE without BELOW for a floating window, the same again as the window maps after it hid, BELOW without ABOVE for the desktop's level and neither for the normal one. The environment says levels and placement work on X11; on Wayland, which GDK may choose with DISPLAY set too, it says neither works.
func TestWindowLevelAsksTheWindowManager(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	scopes := make(chan *core.Scope, 1)
	app := func(cx *core.Scope) core.Node {
		scopes <- cx
		return core.View{Style: core.Style{Background: driver.RGB(0xffffff)}}
	}
	w := startTestWindow(t, app, Options{Title: "nsgo-window-level", Width: 200, Height: 100})
	cx := <-scopes
	var xid uintptr
	w.onLoop(t, func() {
		if drawsOnX11() {
			_, xid, _ = w.x11Window()
		}
	})
	env := make(chan [2]bool, 1)
	cx.Post(func() { env <- [2]bool{cx.Env().CanSetWindowLevel(), cx.Env().CanPlaceWindows()} })
	if got, want := <-env, [2]bool{xid != 0, xid != 0}; got != want {
		t.Errorf("CanSetWindowLevel, CanPlaceWindows = %v, want %v", got, want)
	}
	if xid == 0 {
		return
	}

	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Skip("no libX11")
	}
	var (
		openDisplay  func(name *byte) uintptr
		closeDisplay func(display uintptr) int32
		rootWindow   func(display uintptr) uintptr
		selectInput  func(display, window uintptr, mask int) int32
		xSync        func(display uintptr, discard int32) int32
		pending      func(display uintptr) int32
		nextEvent    func(display uintptr, event *x11ClientMessage) int32
		internAtom   func(display uintptr, name *byte, onlyIfExists int32) uintptr
	)
	purego.RegisterLibFunc(&openDisplay, lib, "XOpenDisplay")
	purego.RegisterLibFunc(&closeDisplay, lib, "XCloseDisplay")
	purego.RegisterLibFunc(&rootWindow, lib, "XDefaultRootWindow")
	purego.RegisterLibFunc(&selectInput, lib, "XSelectInput")
	purego.RegisterLibFunc(&xSync, lib, "XSync")
	purego.RegisterLibFunc(&pending, lib, "XPending")
	purego.RegisterLibFunc(&nextEvent, lib, "XNextEvent")
	purego.RegisterLibFunc(&internAtom, lib, "XInternAtom")
	// A connection of the test's own, which hears what is sent to the root window apart from GTK's.
	display := openDisplay(nil)
	if display == 0 {
		t.Skip("cannot open the X11 display")
	}
	defer closeDisplay(display)
	selectInput(display, rootWindow(display), x11SubstructureNotifyMask)
	xSync(display, 0)
	atom := func(name string) int { return int(internAtom(display, cstr(name), 0)) }
	state, above, below := atom("_NET_WM_STATE"), atom("_NET_WM_STATE_ABOVE"), atom("_NET_WM_STATE_BELOW")

	// next returns the action and the two states of the window's next _NET_WM_STATE request for ABOVE or BELOW.
	next := func(what string) [3]int {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if pending(display) == 0 {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			var ev x11ClientMessage
			nextEvent(display, &ev)
			// GDK sends requests of its own for other states, such as as the window shows.
			if ev.typ == x11ClientMessageType && ev.window == xid && int(ev.messageType) == state && (ev.data[1] == above || ev.data[1] == below) {
				if ev.format != 32 || ev.data[3] != netWMSourceApplication {
					t.Errorf("%s: format %d, source %d", what, ev.format, ev.data[3])
				}
				return [3]int{ev.data[0], ev.data[1], ev.data[2]}
			}
		}
		t.Fatalf("%s: no _NET_WM_STATE request", what)
		return [3]int{}
	}
	expect := func(what string, want ...[3]int) {
		t.Helper()
		for _, req := range want {
			if got := next(what); got != req {
				t.Errorf("%s: request %v, want %v", what, got, req)
			}
		}
	}

	cx.Post(func() { cx.SetWindowLevel(core.WindowLevelFloating) })
	expect("floating", [3]int{netWMStateRemove, below, 0}, [3]int{netWMStateAdd, above, 0})
	w.onLoop(t, func() {
		gtkWidgetSetVisible(w.win, 0)
		gtkWidgetSetVisible(w.win, 1)
	})
	expect("shown again", [3]int{netWMStateRemove, below, 0}, [3]int{netWMStateAdd, above, 0})
	cx.Post(func() { cx.SetWindowLevel(core.WindowLevelDesktop) })
	expect("desktop", [3]int{netWMStateRemove, above, 0}, [3]int{netWMStateAdd, below, 0})
	cx.Post(func() { cx.SetWindowLevel(core.WindowLevelNormal) })
	expect("normal", [3]int{netWMStateRemove, above, below})
}

// TestAppSurfaceHasNoWindowLevel checks that the app's own surface, the hidden window an nsgo.Run(App) function runs on, reports neither levels nor placement, as on macOS, where it has no window.
func TestAppSurfaceHasNoWindowLevel(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	scopes := make(chan *core.Scope, 1)
	app := func(cx *core.Scope) core.Node {
		scopes <- cx
		return core.View{}
	}
	w := startTestWindow(t, app, Options{Title: "nsgo-app-level", Width: 120, Height: 80, Hidden: true})
	defer w.onLoop(t, w.close)
	cx := <-scopes
	env := make(chan [2]bool, 1)
	cx.Post(func() { env <- [2]bool{cx.Env().CanSetWindowLevel(), cx.Env().CanPlaceWindows()} })
	if got := <-env; got != [2]bool{} {
		t.Errorf("CanSetWindowLevel, CanPlaceWindows = %v, want both false", got)
	}
}
