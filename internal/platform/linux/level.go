//go:build linux && !android

package linux

import (
	"sync"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// GTK 4 dropped gtk_window_set_keep_above and has no stacking of its own, so on X11 a window's level is the window manager's _NET_WM_STATE_ABOVE or _NET_WM_STATE_BELOW, asked for with EWMH's client message through the window's X window, as its panels are placed (panel.go). A window manager forgets the state as the window is withdrawn, and GDK writes the property afresh as it maps one, so the level is asked for again each time the window maps. Wayland has no protocol for stacking a toplevel, and there the environment says so (CanSetWindowLevel).
var (
	x11InternAtom   func(display uintptr, name *byte, onlyIfExists int32) uintptr
	x11LevelRoot    func(display uintptr) uintptr
	x11QueryPointer func(display, window uintptr, root, child *uintptr, rootX, rootY, x, y *int32, mask *uint32) int32
	x11SendEvent    func(display, window uintptr, propagate int32, mask int, event *x11ClientMessage) int32
	x11LevelsReady  bool
	bindX11Levels   = sync.OnceFunc(func() {
		if bindX11Panels(); !x11PanelsReady {
			return
		}
		lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			if lib, err = purego.Dlopen("libX11.so", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
				return
			}
		}
		purego.RegisterLibFunc(&x11InternAtom, lib, "XInternAtom")
		purego.RegisterLibFunc(&x11LevelRoot, lib, "XDefaultRootWindow")
		purego.RegisterLibFunc(&x11QueryPointer, lib, "XQueryPointer")
		purego.RegisterLibFunc(&x11SendEvent, lib, "XSendEvent")
		x11LevelsReady = true
	})
)

// x11ClientMessage is Xlib's XClientMessageEvent padded to the size of the XEvent union XSendEvent reads, with Go laying out its fields as C does.
type x11ClientMessage struct {
	typ         int32
	serial      uintptr
	sendEvent   int32
	display     uintptr
	window      uintptr
	messageType uintptr
	format      int32
	data        [5]int
	_           [12]int
}

const (
	x11ClientMessageType = 33 // ClientMessage
	// The window manager hears a _NET_WM_STATE request through these masks on the root window.
	x11SubstructureNotifyMask   = 1 << 19
	x11SubstructureRedirectMask = 1 << 20
	netWMStateRemove            = 0
	netWMStateAdd               = 1
	// netWMSourceApplication says a normal application asks, not a pager.
	netWMSourceApplication = 1
)

// drawsOnX11 reports whether GDK draws the app's windows on X11, Xwayland included, where a window manager stacks them by _NET_WM_STATE.
func drawsOnX11() bool {
	if bindX11Levels(); !x11LevelsReady {
		return false
	}
	gdk := gdkDisplayGetDefault()
	return gdk != 0 && gTypeIsA(gdk, gdkX11DisplayGetType()) != 0
}

// canSetLevel reports whether setLevel reaches a window manager for w: a window of its own on X11 that one manages, not the app's own surface (Options.Hidden) or a tray icon's, which show in none, as on macOS.
func (w *window) canSetLevel() bool {
	return w.win != 0 && !w.embedded && !w.unmanaged && !(w.primary && w.opts.Hidden) && w.kind != driver.SurfaceTrayIcon && drawsOnX11()
}

// setLevel puts the window at level, now where it is mapped and again whenever it maps.
func (w *window) setLevel(level driver.WindowLevel) {
	w.level = level
	if level == driver.WindowLevelNormal && !w.levelWatched || !w.canSetLevel() {
		return
	}
	if !w.levelWatched {
		// The class handler maps the X window before the handlers connected here run, so the window manager hears of the window before the request.
		w.levelWatched = true
		gSignalConnect(w.win, "map", cbLevelMapped, w.handle)
	}
	if gtkWidgetGetMapped(w.win) != 0 {
		w.applyLevel()
	}
}

// applyLevel asks the window manager for w.level's _NET_WM_STATE: neither ABOVE nor BELOW for the normal level.
func (w *window) applyLevel() {
	display, xid, ok := w.x11Window()
	if !ok {
		return
	}
	atom := func(name string) uintptr { return x11InternAtom(display, cstr(name), 0) }
	state, above, below := atom("_NET_WM_STATE"), atom("_NET_WM_STATE_ABOVE"), atom("_NET_WM_STATE_BELOW")
	root := x11LevelRoot(display)
	send := func(action int, first, second uintptr) {
		ev := x11ClientMessage{typ: x11ClientMessageType, sendEvent: 1, display: display, window: xid, messageType: state, format: 32,
			data: [5]int{action, int(first), int(second), netWMSourceApplication}}
		x11SendEvent(display, root, 0, x11SubstructureRedirectMask|x11SubstructureNotifyMask, &ev)
	}
	switch w.level {
	case driver.WindowLevelFloating:
		send(netWMStateRemove, below, 0)
		send(netWMStateAdd, above, 0)
	case driver.WindowLevelDesktop:
		send(netWMStateRemove, above, 0)
		send(netWMStateAdd, below, 0)
	default:
		send(netWMStateRemove, above, below)
	}
	x11Flush(display)
}

// levelMappedCallback asks for the window's level again as it maps, the window manager having dropped it while the window was withdrawn.
func levelMappedCallback(widget, data uintptr) {
	if w := windowOf(data); w != nil && !w.closing {
		w.applyLevel()
	}
}
