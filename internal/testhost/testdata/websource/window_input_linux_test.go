//go:build linux && !android

package websource_test

import (
	"os"
	"testing"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The Linux test surface dispatches presses through NSGo's engine. WebKit
// is a native child widget, so it needs X11 input from the test display.
func init() {
	clickWindowPage = func(_ *nsgotest.App, t *testing.T, y int) { clickWebKit(t, y) }
}

func clickWebKit(t *testing.T, y int) {
	t.Helper()
	x11, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Skip("WebKit click test needs libX11")
	}
	defer purego.Dlclose(x11)
	xtst, err := purego.Dlopen("libXtst.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Skip("WebKit click test needs libXtst")
	}
	defer purego.Dlclose(xtst)
	var (
		open   func(uintptr) uintptr
		close  func(uintptr) int32
		root   func(uintptr) uintptr
		tree   func(uintptr, uintptr, *uintptr, *uintptr, **uintptr, *uint32) int32
		free   func(unsafe.Pointer) int32
		atom   func(uintptr, string, int32) uintptr
		prop   func(uintptr, uintptr, uintptr, int64, int64, int32, uintptr, *uintptr, *int32, *uintptr, *uintptr, **uintptr) int32
		warp   func(uintptr, uintptr, uintptr, int32, int32, uint32, uint32, int32, int32) int32
		button func(uintptr, uint32, int32, uintptr) int32
		xsync  func(uintptr, int32) int32
	)
	purego.RegisterLibFunc(&open, x11, "XOpenDisplay")
	purego.RegisterLibFunc(&close, x11, "XCloseDisplay")
	purego.RegisterLibFunc(&root, x11, "XDefaultRootWindow")
	purego.RegisterLibFunc(&tree, x11, "XQueryTree")
	purego.RegisterLibFunc(&free, x11, "XFree")
	purego.RegisterLibFunc(&atom, x11, "XInternAtom")
	purego.RegisterLibFunc(&prop, x11, "XGetWindowProperty")
	purego.RegisterLibFunc(&warp, x11, "XWarpPointer")
	purego.RegisterLibFunc(&button, xtst, "XTestFakeButtonEvent")
	purego.RegisterLibFunc(&xsync, x11, "XSync")
	display := open(0)
	if display == 0 {
		t.Skip("WebKit click test needs an X11 display")
	}
	defer close(display)
	var rootID, parentID uintptr
	var children *uintptr
	var n uint32
	if tree(display, root(display), &rootID, &parentID, &children, &n) == 0 {
		t.Fatal("cannot list the X11 test windows")
	}
	if children != nil {
		defer free(unsafe.Pointer(children))
	}
	pidProperty := atom(display, "_NET_WM_PID", 0)
	for _, window := range unsafe.Slice(children, n) {
		var kind, length, after uintptr
		var format int32
		var pid *uintptr
		const cardinal = 6 // XA_CARDINAL
		prop(display, window, pidProperty, 0, 1, 0, cardinal, &kind, &format, &length, &after, &pid)
		if pid == nil {
			continue
		}
		ours := kind == cardinal && format == 32 && length == 1 && *pid == uintptr(os.Getpid())
		free(unsafe.Pointer(pid))
		if !ours {
			continue
		}
		warp(display, 0, window, 0, 0, 0, 0, 12, int32(y))
		button(display, 1, 1, 0)
		button(display, 1, 0, 0)
		xsync(display, 0)
		return
	}
	t.Fatal("the X11 test window was not found")
}
