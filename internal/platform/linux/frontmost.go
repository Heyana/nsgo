//go:build linux && !android

package linux

import (
	"bytes"
	"encoding/binary"
	"os"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The application in front on X11: the window manager names the window in front in the root window's _NET_ACTIVE_WINDOW (EWMH), and that window says whose it is with WM_CLASS, _GTK_APPLICATION_ID and _NET_WM_PID, and whether it is in full screen with _NET_WM_STATE. The app hears both change as PropertyNotify events through GdkX11Display's xevent signal. Wayland uses its optional compositor protocol (frontmost_wayland.go), never Xwayland's partial window list. The GTK main thread alone uses these.
var (
	x11GetWindowProperty   func(display, window, property uintptr, offset, length int, delete int32, reqType uintptr, actualType *uintptr, actualFormat *int32, items, bytesAfter *uint, prop *uintptr) int32
	x11Free                func(data uintptr) int32
	x11SelectInput         func(display, window uintptr, mask int) int32
	x11GetWindowAttributes func(display, window uintptr, attributes *x11Attributes) int32

	frontmost        driver.FrontmostAppInfo
	frontmostX11     *x11Frontmost
	frontmostWayland *wlFrontmostServer
	frontmostWatch   sync.Once
)

// x11Attributes is Xlib's XWindowAttributes on LP64, of which the app reads the event mask it selected.
type x11Attributes struct {
	x, y, width, height, borderWidth, depth     int32
	visual, root                                uintptr
	class, bitGravity, winGravity, backingStore int32
	backingPlanes, backingPixel                 uintptr
	saveUnder                                   int32
	colormap                                    uintptr
	mapInstalled, mapState                      int32
	allEventMasks, yourEventMask, noPropagate   int
	overrideRedirect                            int32
	screen                                      uintptr
}

const (
	x11PropertyNotify      = 28
	x11PropertyChangeMask  = 1 << 22
	x11AtomAtom            = 4
	x11AtomCardinal        = 6
	x11AtomString          = 31
	x11AtomWindow          = 33
	x11AtomWMClass         = 67
	x11PropertyMaxLongs    = 1024
	x11SizeOfLong          = 8
	x11PropertyEventWindow = 32 // XPropertyEvent's window, on LP64
	x11PropertyEventAtom   = 40 // and its atom
)

// x11Frontmost follows the window in front on one X display.
type x11Frontmost struct {
	gdk, display, root uintptr
	// active is the window in front, whose properties the app selected; hadMask says it selected them before, as for its own windows.
	active  uintptr
	hadMask bool
	atoms   struct{ activeWindow, pid, state, fullScreen, applicationID, utf8 uintptr }
}

// frontmostApp follows X11 or an available compositor-owned Wayland protocol.
func frontmostApp() (driver.FrontmostAppInfo, bool) {
	frontmostWatch.Do(func() {
		if frontmostX11 = watchX11Frontmost(); frontmostX11 != nil {
			frontmost = frontmostX11.read()
		} else {
			frontmostWayland = watchWaylandFrontmost()
		}
	})
	return frontmost, frontmostX11 != nil || frontmostWayland != nil
}

func watchX11Frontmost() *x11Frontmost {
	// Xwayland cannot describe the native clients of a Wayland desktop.
	if os.Getenv("WAYLAND_DISPLAY") != "" || strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland") {
		return nil
	}
	if bindX11HotKeys(); !x11HotKeysReady {
		return nil
	}
	if bindX11Panels(); !x11PanelsReady {
		return nil
	}
	gdk := gdkDisplayGetDefault()
	if gdk == 0 || gTypeIsA(gdk, gdkX11DisplayGetType()) == 0 {
		return nil
	}
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		if lib, err = purego.Dlopen("libX11.so", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			return nil
		}
	}
	for _, symbol := range []string{"XInternAtom", "XGetWindowProperty", "XFree", "XSelectInput", "XGetWindowAttributes"} {
		if _, err := purego.Dlsym(lib, symbol); err != nil {
			return nil
		}
	}
	purego.RegisterLibFunc(&x11InternAtom, lib, "XInternAtom")
	purego.RegisterLibFunc(&x11GetWindowProperty, lib, "XGetWindowProperty")
	purego.RegisterLibFunc(&x11Free, lib, "XFree")
	purego.RegisterLibFunc(&x11SelectInput, lib, "XSelectInput")
	purego.RegisterLibFunc(&x11GetWindowAttributes, lib, "XGetWindowAttributes")
	display := gdkX11DisplayGetXDisplay(gdk)
	if display == 0 {
		return nil
	}
	f := &x11Frontmost{gdk: gdk, display: display, root: x11DefaultRootWindow(display)}
	atom := func(name string) uintptr { return x11InternAtom(display, cstr(name), 0) }
	f.atoms.activeWindow, f.atoms.pid = atom("_NET_ACTIVE_WINDOW"), atom("_NET_WM_PID")
	f.atoms.state, f.atoms.fullScreen = atom("_NET_WM_STATE"), atom("_NET_WM_STATE_FULLSCREEN")
	f.atoms.applicationID, f.atoms.utf8 = atom("_GTK_APPLICATION_ID"), atom("UTF8_STRING")
	gdkX11DisplayErrorTrapPush(gdk)
	f.selectProperties(f.root)
	_ = gdkX11DisplayErrorTrapPop(gdk)
	gSignalConnect(gdk, "xevent", cbFrontmostEvent, 0)
	return f
}

// selectProperties adds PropertyChangeMask to what the app selects on window, keeping the rest, as GDK selects on its own windows and on the root; it reports whether the app selected it already.
func (f *x11Frontmost) selectProperties(window uintptr) (had bool) {
	var attrs x11Attributes
	if x11GetWindowAttributes(f.display, window, &attrs) == 0 {
		return true
	}
	if attrs.yourEventMask&x11PropertyChangeMask != 0 {
		return true
	}
	x11SelectInput(f.display, window, attrs.yourEventMask|x11PropertyChangeMask)
	return false
}

// unselectProperties takes PropertyChangeMask back from window, leaving what else the app selects on it.
func (f *x11Frontmost) unselectProperties(window uintptr) {
	var attrs x11Attributes
	if x11GetWindowAttributes(f.display, window, &attrs) != 0 {
		x11SelectInput(f.display, window, attrs.yourEventMask&^x11PropertyChangeMask)
	}
}

// read reads the application in front, following the properties of the window in front from now on.
func (f *x11Frontmost) read() (app driver.FrontmostAppInfo) {
	gdkX11DisplayErrorTrapPush(f.gdk)
	defer func() { _ = gdkX11DisplayErrorTrapPop(f.gdk) }()
	var active uintptr
	if longs := f.longs(f.root, f.atoms.activeWindow, x11AtomWindow); len(longs) > 0 {
		active = longs[0]
	}
	if active != f.active {
		if f.active != 0 && !f.hadMask {
			f.unselectProperties(f.active)
		}
		f.active, f.hadMask = active, true
		if active != 0 {
			f.hadMask = f.selectProperties(active)
		}
	}
	if active == 0 {
		return app
	}
	instance, class := wmClass(f.property(active, x11AtomWMClass, x11AtomString))
	app.Name = class
	if app.Name == "" {
		app.Name = instance
	}
	app.ID = string(f.property(active, f.atoms.applicationID, f.atoms.utf8))
	if app.ID == "" {
		app.ID = app.Name
	}
	if longs := f.longs(active, f.atoms.pid, x11AtomCardinal); len(longs) > 0 {
		app.PID = int(int32(longs[0]))
	}
	for _, state := range f.longs(active, f.atoms.state, x11AtomAtom) {
		if state == f.atoms.fullScreen {
			app.FullScreen = true
		}
	}
	return app
}

// property returns the bytes of window's property of type kind, of format 8, nil without one.
func (f *x11Frontmost) property(window, property, kind uintptr) []byte {
	data, format := f.get(window, property, kind)
	if format != 8 {
		return nil
	}
	return data
}

// longs returns window's property of type kind, of format 32, which Xlib hands out as C longs.
func (f *x11Frontmost) longs(window, property, kind uintptr) []uintptr {
	data, format := f.get(window, property, kind)
	if format != 32 {
		return nil
	}
	out := make([]uintptr, len(data)/x11SizeOfLong)
	for i := range out {
		out[i] = uintptr(binary.NativeEndian.Uint64(data[i*x11SizeOfLong:]))
	}
	return out
}

// get reads up to 4 KB of window's property of type kind, with its format.
func (f *x11Frontmost) get(window, property, kind uintptr) ([]byte, int32) {
	if window == 0 || property == 0 {
		return nil, 0
	}
	var actualType, prop uintptr
	var format int32
	var items, after uint
	if x11GetWindowProperty(f.display, window, property, 0, x11PropertyMaxLongs, 0, kind, &actualType, &format, &items, &after, &prop) != 0 || prop == 0 {
		return nil, 0
	}
	defer x11Free(prop)
	if actualType != kind {
		return nil, 0
	}
	size := int(items)
	switch format {
	case 16:
		size *= 2
	case 32:
		size *= x11SizeOfLong
	}
	return bytes.Clone(unsafe.Slice((*byte)(cPointer(prop)), size)), format
}

// wmClass splits WM_CLASS into its instance and class names, two strings each ending in NUL.
func wmClass(b []byte) (instance, class string) {
	parts := bytes.SplitN(b, []byte{0}, 3)
	if len(parts) > 0 {
		instance = string(parts[0])
	}
	if len(parts) > 1 {
		class = string(parts[1])
	}
	return instance, class
}

// frontmostEvent is GdkX11Display's xevent handler: a change to the root window's active window or to the active window's state reads the app in front again. It never takes the event from GTK.
func frontmostEvent(_, xevent, _ uintptr) int32 {
	f := frontmostX11
	if f == nil || xevent == 0 {
		return 0
	}
	ev := cPointer(xevent)
	if *(*int32)(ev) != x11PropertyNotify {
		return 0
	}
	xid := *(*uintptr)(unsafe.Add(ev, x11PropertyEventWindow))
	atom := *(*uintptr)(unsafe.Add(ev, x11PropertyEventAtom))
	if !(xid == f.root && atom == f.atoms.activeWindow || xid == f.active && atom == f.atoms.state) {
		return 0
	}
	reportFrontmost(f.read())
	return 0
}

func reportFrontmost(app driver.FrontmostAppInfo) {
	if app == frontmost {
		return
	}
	frontmost = app
	refreshFrontmostWindows()
}

func refreshFrontmostWindows() {
	windowHandles.Range(func(_, value any) bool {
		if w := value.(*window); !w.closing {
			w.updateEnvironment()
		}
		return true
	})
}
