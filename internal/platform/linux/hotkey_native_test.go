//go:build linux && !android

package linux

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

// TestGlobalShortcutsReachTheBackend runs an app that registers a global shortcut in a real GTK window: the shortcut reaches X11's grabs, a grabbed press runs it, and so does the portal's activation of its session.
func TestGlobalShortcutsReachTheBackend(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DISPLAY") == "" {
		t.Skip("grabs keys on an X11 display")
	}
	title := "nsgo-hotkeys"
	app := func(cx *core.Scope) core.Node {
		fired := core.NewSignal(cx, 0)
		cx.RegisterHotKey("ctrl+alt+k", func() { fired.Update(func(n int) int { return n + 1 }) })
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%d", title, fired.Get())) })
		return core.View{Style: core.Style{Background: driver.RGB(0xffffff)}}
	}
	w := startTestWindow(t, app, Options{Title: title, Width: 200, Height: 100})
	waitForTitle(t, w, title+"|0")

	var combo x11HotKey
	var id uint32
	deadline := time.Now().Add(5 * time.Second)
	for id == 0 {
		w.onLoop(t, func() {
			if w.hotKeys != nil && w.hotKeys.x11 != nil {
				for c, i := range w.hotKeys.x11.bindings {
					combo, id = c, i
				}
			}
		})
		if id == 0 && time.Now().After(deadline) {
			t.Fatal("the app's shortcut did not reach X11's grabs")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if combo.modifiers != x11ControlMask|x11Mod1Mask {
		t.Errorf("the grab's modifiers are %#x, want Control and Mod1", combo.modifiers)
	}

	// A press as the server reports it, with NumLock on and the second layout group.
	var ev [24]uint64 // sizeof(XEvent) on LP64
	p := unsafe.Pointer(&ev[0])
	*(*int32)(p) = x11KeyPress
	*(*uint32)(unsafe.Add(p, 80)) = combo.modifiers | x11Mod2Mask | 1<<13
	*(*uint32)(unsafe.Add(p, 84)) = uint32(combo.keycode)
	w.onLoop(t, func() {
		if x11HotKeyEvent(0, uintptr(p), w.handle) == 0 {
			t.Error("GTK kept the grabbed press")
		}
	})
	waitForTitle(t, w, title+"|1")

	// The portal names the session in Activated's first argument, on its own object.
	session := globalShortcutPath + "/session/1_42/nsgo_create_1"
	activate := func(of string) {
		params := portalVariant("(osta{sv})", fmt.Sprintf("(objectpath %s, %s, uint64 0, @a{sv} {})", portalQuote(of), portalQuote("1")))
		if params == 0 {
			t.Error("the signal's arguments did not parse")
			return
		}
		defer gVariantUnref(params)
		globalHotKeyActivated(0, 0, uintptr(unsafe.Pointer(cstr(globalShortcutPath))), 0, 0, params, w.handle)
	}
	w.onLoop(t, func() {
		bindGlobalHotKeys()
		w.closeHotKeys()
		w.hotKeys = &globalHotKeys{session: session, ids: map[string]uint32{"1": id}}
		activate(globalShortcutPath + "/session/1_42/other")
		activate(session)
	})
	waitForTitle(t, w, title+"|2")
	time.Sleep(200 * time.Millisecond)
	if got := w.titleOf(t); got != title+"|2" {
		t.Errorf("another session's activation ran the shortcut: %q", got)
	}
}

// TestGlobalShortcutFromAnotherClient sends real XTest key presses while a
// different X11 client owns keyboard focus. Disposing the registration returns
// its key to that client instead of dispatching the app's callback.
func TestGlobalShortcutFromAnotherClient(t *testing.T) {
	if testing.Short() {
		t.Skip("opens X11 windows")
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DISPLAY") == "" {
		t.Skip("needs an X11 display")
	}
	const title = "nsgo-background-hotkey"
	owners := make(chan *core.Scope, 1)
	fired := make(chan struct{}, 8)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		owner := cx.Child()
		var h *core.HotKey
		owner.Run(func() { h = owner.RegisterHotKey("ctrl+alt+k", func() { fired <- struct{}{} }) })
		cx.Effect(func() { cx.SetTitle(title + "|" + h.State().String()) })
		owners <- owner
		return core.Text{Content: "Background shortcut"}
	}, Options{Title: title, Width: 200, Height: 100})
	owner := <-owners
	waitForTitle(t, w, title+"|registered")
	w.onLoop(t, func() { bindX11WebInput() })
	if !bindDesktopXlib() || !bindXPointer() || !xWebInput.ready {
		t.Skip("needs Xlib and XTest")
	}
	x := &desktopXlib
	display := x.openDisplay(nil)
	if display == 0 {
		t.Fatal("cannot open the other client's X11 connection")
	}
	defer x.closeDisplay(display)
	other := x.createWindow(display, x.rootWindow(display), 0, 0, 100, 100, 0, 0, 0)
	defer x.destroyWindow(display, other)
	x.selectInput(display, other, 1) // KeyPressMask
	// Map independently of a window manager, which may otherwise defer the
	// MapRequest until after the test asks the server to focus this window.
	x11ChangeWindowAttributes(display, other, x11CWOverrideRedirect, &x11WindowAttributes{overrideRedirect: 1})
	xPointer.mapWindow(display, other)
	x.sync(display, 0)
	xWebInput.setInputFocus(display, other, 1, 0)
	x.sync(display, 0)
	var focus uintptr
	var revert int32
	xWebInput.getInputFocus(display, &focus, &revert)
	if focus != other {
		t.Fatalf("keyboard focus = %#x, want the other client %#x", focus, other)
	}
	press := func() {
		t.Helper()
		codes := []uint32{uint32(xWebInput.keysymToKeycode(display, 0xffe3)), uint32(xWebInput.keysymToKeycode(display, 0xffe9)), uint32(xWebInput.keysymToKeycode(display, 'k'))}
		for _, code := range codes {
			if code == 0 || xWebInput.fakeKey(display, code, 1, 0) == 0 {
				t.Fatal("XTest refused the key press")
			}
		}
		for i := len(codes) - 1; i >= 0; i-- {
			xWebInput.fakeKey(display, codes[i], 0, 0)
		}
		x.sync(display, 0)
	}
	press()
	select {
	case <-fired:
	case <-time.After(5 * time.Second):
		t.Fatal("the global shortcut did not fire with another client focused")
	}
	xWebInput.getInputFocus(display, &focus, &revert)
	if focus != other {
		t.Errorf("the shortcut took keyboard focus from the other client")
	}
	owner.Post(owner.Dispose)
	deadline := time.Now().Add(5 * time.Second)
	for {
		var released bool
		w.onLoop(t, func() { released = w.hotKeys == nil })
		if released {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("disposing the owner kept its global shortcut")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Remove the modifier events that the grabbed press left with the client.
	for x.pending(display) != 0 {
		var event desktopEvent
		x.nextEvent(display, &event)
	}
	press()
	var received bool
	for x.pending(display) != 0 {
		var event desktopEvent
		x.nextEvent(display, &event)
		if event.typ == x11KeyPress && *(*uint32)(unsafe.Add(unsafe.Pointer(&event), 84)) == uint32(xWebInput.keysymToKeycode(display, 'k')) {
			received = true
		}
	}
	if !received {
		t.Error("the released shortcut did not return to the focused client")
	}
	select {
	case <-fired:
		t.Error("the disposed registration still dispatched its callback")
	case <-time.After(200 * time.Millisecond):
	}
}

// holdX11Combo grabs combo, a trigger as portalTrigger writes it, on a display connection of its own, as another client would, until the test ends.
func holdX11Combo(t *testing.T, combo string) {
	t.Helper()
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		t.Skip("no libX11")
	}
	var (
		openDisplay  func(name *byte) uintptr
		closeDisplay func(display uintptr) int32
		defaultRoot  func(display uintptr) uintptr
		stringToSym  func(name *byte) uintptr
		symToCode    func(display, keysym uintptr) uint8
		grabKey      func(display uintptr, keycode int32, modifiers uint32, root uintptr, ownerEvents, pointerMode, keyboardMode int32) int32
		syncDisplay  func(display uintptr, discard int32) int32
	)
	purego.RegisterLibFunc(&openDisplay, lib, "XOpenDisplay")
	purego.RegisterLibFunc(&closeDisplay, lib, "XCloseDisplay")
	purego.RegisterLibFunc(&defaultRoot, lib, "XDefaultRootWindow")
	purego.RegisterLibFunc(&stringToSym, lib, "XStringToKeysym")
	purego.RegisterLibFunc(&symToCode, lib, "XKeysymToKeycode")
	purego.RegisterLibFunc(&grabKey, lib, "XGrabKey")
	purego.RegisterLibFunc(&syncDisplay, lib, "XSync")
	display := openDisplay(nil)
	if display == 0 {
		t.Skip("no X11 display")
	}
	t.Cleanup(func() { closeDisplay(display) })
	parts := strings.Split(combo, "+")
	mods, ok := x11TriggerModifiers(parts[:len(parts)-1])
	keycode := symToCode(display, stringToSym(cstr(parts[len(parts)-1])))
	if !ok || keycode == 0 {
		t.Fatalf("no grab for %s", combo)
	}
	grabKey(display, int32(keycode), mods, defaultRoot(display), 0, x11GrabModeAsync, x11GrabModeAsync)
	syncDisplay(display, 0)
}

// hotKeyStatesApp registers keys as global shortcuts and shows their states in the window's title, after title and each separated by |.
func hotKeyStatesApp(title string, keys ...string) engine.App {
	return func(cx *core.Scope) core.Node {
		handles := make([]*core.HotKey, len(keys))
		for i, key := range keys {
			handles[i] = cx.RegisterHotKey(key, nil)
		}
		cx.Effect(func() {
			states := []string{title}
			for _, h := range handles {
				states = append(states, h.State().String())
			}
			cx.SetTitle(strings.Join(states, "|"))
		})
		return core.View{Style: core.Style{Background: driver.RGB(0xffffff)}}
	}
}

// TestGlobalShortcutAnswers registers global shortcuts on an X11 display where another client grabbed one of them: that one is taken, as a second registration of the app's own is, a free one registers, and a key X11 has no keysym for is unsupported.
func TestGlobalShortcutAnswers(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DISPLAY") == "" {
		t.Skip("grabs keys on an X11 display")
	}
	holdX11Combo(t, "CTRL+ALT+j")
	title := "nsgo-hotkey-answers"
	w := startTestWindow(t, hotKeyStatesApp(title, "ctrl+alt+j", "ctrl+alt+k", "ctrl+alt+k", "ctrl+alt+nosuchkey"),
		Options{Title: title, Width: 200, Height: 100})
	waitForTitle(t, w, title+"|taken|registered|taken|unsupported")
}

// TestGlobalShortcutTakenWithoutPortal registers only a combination another X11 client grabbed: with no grab of its own the backend tries the portal, and a portal that cannot take it either, such as GNOME's before 48, reports it taken rather than unsupported.
func TestGlobalShortcutTakenWithoutPortal(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("WAYLAND_DISPLAY") != "" || os.Getenv("DISPLAY") == "" {
		t.Skip("grabs keys on an X11 display")
	}
	holdX11Combo(t, "CTRL+ALT+j")
	title := "nsgo-hotkey-portal"
	w := startTestWindow(t, hotKeyStatesApp(title, "ctrl+alt+j"), Options{Title: title, Width: 200, Height: 100})
	deadline := time.Now().Add(10 * time.Second)
	for {
		switch got := w.titleOf(t); got {
		case title + "|taken":
			return
		case title + "|pending":
			if time.Now().After(deadline) {
				t.Skip("the portal is asking the user to bind the shortcut")
			}
			time.Sleep(20 * time.Millisecond)
		default:
			if strings.HasPrefix(got, title+"|") {
				t.Fatalf("the held combination answered %q, want taken", got)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// TestPortalShortcutTwiceIsTaken registers one combination twice where X11 grabs nothing, so both go to the portal path: the second is taken at once, as it is on X11 and the other desktops, rather than sent to the portal again.
func TestPortalShortcutTwiceIsTaken(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if os.Getenv("WAYLAND_DISPLAY") == "" && os.Getenv("DISPLAY") == "" {
		t.Skip("needs a display")
	}
	bindX11HotKeys()
	ready := x11HotKeysReady
	x11HotKeysReady = false
	t.Cleanup(func() { x11HotKeysReady = ready })
	title := "nsgo-hotkey-portal-twice"
	w := startTestWindow(t, hotKeyStatesApp(title, "ctrl+alt+k", "ctrl+alt+k"), Options{Title: title, Width: 200, Height: 100})
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := w.titleOf(t)
		if first, second, ok := strings.Cut(strings.TrimPrefix(got, title+"|"), "|"); ok && strings.HasPrefix(got, title+"|") && second != "pending" {
			if second != "taken" || first == "taken" {
				t.Fatalf("the combination asked for twice answered %q, want the second taken and the first not", got)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("window title = %q, want the second shortcut answered", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
