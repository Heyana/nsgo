//go:build linux && !android

package linux

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// nativeSizeHints is Xlib's XSizeHints, read independently of the backend.
type nativeSizeHints struct {
	Flags                                          int
	X, Y, Width, Height                            int32
	MinWidth, MinHeight, MaxWidth, MaxHeight       int32
	WidthInc, HeightInc                            int32
	MinAspectX, MinAspectY, MaxAspectX, MaxAspectY int32
	BaseWidth, BaseHeight, WinGravity              int32
}

func TestWindowConstraintsPublishMaximumAndAllowNativeResize(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	w := startTestWindow(t, func(*core.Scope) core.Node { return core.View{} }, Options{Title: "nsgo-maximum-size", Width: 300, Height: 200})
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Skip(err)
	}
	var readHints func(uintptr, uintptr, *nativeSizeHints, *int) int32
	var resize func(uintptr, uintptr, uint32, uint32) int32
	purego.RegisterLibFunc(&readHints, lib, "XGetWMNormalHints")
	purego.RegisterLibFunc(&resize, lib, "XResizeWindow")
	var display, xid uintptr
	w.onLoop(t, func() { display, xid, _ = w.x11Window() })
	if xid == 0 {
		t.Skip("X11 native size hints")
	}
	set := func(c driver.WindowConstraints) { w.onLoop(t, func() { w.setConstraints(c) }) }
	requestSize := func(width, height int32) {
		w.onLoop(t, func() {
			display, xid, _ = w.x11Window()
			surface := gtkNativeGetSurface(w.win)
			scale := max(gtkWidgetGetScaleFactor(w.win), 1)
			extraWidth := gdkSurfaceGetWidth(surface) - gtkWidgetGetWidth(w.surface)
			extraHeight := gdkSurfaceGetHeight(surface) - gtkWidgetGetHeight(w.surface)
			resize(display, xid, uint32((width+extraWidth)*scale), uint32((height+extraHeight)*scale))
			x11Flush(display)
		})
	}
	awaitSize := func(width, height driver.Points) {
		t.Helper()
		var got driver.Size
		for end := time.Now().Add(3 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			w.onLoop(t, func() {
				got = driver.Size{Width: driver.Points(gtkWidgetGetWidth(w.surface)), Height: driver.Points(gtkWidgetGetHeight(w.surface))}
			})
			if got.Width == width && got.Height == height {
				return
			}
			if time.Now().After(end) {
				t.Fatalf("content size=%v, want %vx%v", got, width, height)
			}
		}
	}
	const maximum = 1 << 5 // PMaxSize
	// awaitHints waits for the hints a constraint change publishes as the window is next allocated, which setConstraints does not wait for.
	awaitHints := func(what string, ok func(nativeSizeHints) bool) {
		t.Helper()
		var hints nativeSizeHints
		var supplied int
		for end := time.Now().Add(3 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			w.onLoop(t, func() { display, xid, _ = w.x11Window(); readHints(display, xid, &hints, &supplied) })
			if ok(hints) {
				return
			}
			if time.Now().After(end) {
				t.Fatalf("WM_NORMAL_HINTS flags=%#x max=%dx%d have no %s", hints.Flags, hints.MaxWidth, hints.MaxHeight, what)
			}
		}
	}
	set(driver.WindowConstraints{MinWidth: 180, MinHeight: 100, MaxWidth: 400, MaxHeight: 300, NoMinimize: true})
	awaitHints("maximum", func(h nativeSizeHints) bool { return h.Flags&maximum != 0 })
	if bindDesktopXlib() {
		x := &desktopXlib
		var actual uintptr
		var format int32
		var n, after uint
		var data *byte
		w.onLoop(t, func() {
			check := x.internAtom(display, cstr("_NET_SUPPORTING_WM_CHECK"), 0)
			x.getProperty(display, x.rootWindow(display), check, 0, 1, 0, desktopXAWindow, &actual, &format, &n, &after, &data)
			if data != nil {
				x.free(data)
			}
		})
		if format != 32 || n == 0 {
			if os.Getenv("NSGO_REQUIRE_WINDOW_MANAGER") == "1" {
				t.Fatal("the native window-manager gate has no X11 window manager")
			}
			t.Skip("maximum hint published; native resizing needs an X11 window manager")
		}
	}
	requestSize(900, 800)
	awaitSize(400, 300)
	requestSize(260, 160)
	awaitSize(260, 160)
	requestSize(60, 40)
	awaitSize(180, 100)
	set(driver.WindowConstraints{MaxWidth: 240})
	awaitHints("maximum width alone", func(h nativeSizeHints) bool { return h.Flags&maximum != 0 && h.MaxHeight == math.MaxInt32 })
	requestSize(800, 400)
	awaitSize(240, 400)
	set(driver.WindowConstraints{})
	awaitHints("cleared maximum", func(h nativeSizeHints) bool { return h.Flags&maximum == 0 })
	requestSize(600, 450)
	awaitSize(600, 450)
}

func TestWindowConstraintsFilterNativeHeaderButtons(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	var titlebar func(uintptr) uintptr
	var layout func(uintptr) uintptr
	var hasClass func(uintptr, *byte) int32
	var setString func(uintptr, *byte, *byte, uintptr)
	purego.RegisterLibFunc(&titlebar, gtkLib, "gtk_window_get_titlebar")
	purego.RegisterLibFunc(&layout, gtkLib, "gtk_header_bar_get_decoration_layout")
	purego.RegisterLibFunc(&hasClass, gtkLib, "gtk_widget_has_css_class")
	purego.RegisterLibFunc(&setString, gobjectLib, "g_object_set")
	w := startTestWindow(t, func(*core.Scope) core.Node { return core.View{} }, Options{Title: "nsgo-header-limits", Width: 320, Height: 180})
	var previous string
	w.onLoop(t, func() {
		var text uintptr
		gObjectGet(gtkSettingsGetDefault(), cstr("gtk-decoration-layout"), &text, uintptr(0))
		if text != 0 {
			previous = goString(text)
			gFree(text)
		}
	})
	t.Cleanup(func() {
		w.onLoop(t, func() { setString(gtkSettingsGetDefault(), cstr("gtk-decoration-layout"), cstr(previous), 0) })
	})
	w.onLoop(t, func() {
		setString(gtkSettingsGetDefault(), cstr("gtk-decoration-layout"), cstr("icon:minimize,maximize,close"), 0)
	})
	for _, tc := range []struct {
		c                  driver.WindowConstraints
		want               string
		minimize, maximize int
	}{
		{driver.WindowConstraints{NoMinimize: true}, "icon:maximize,close", 0, 1},
		{driver.WindowConstraints{NoMaximize: true}, "icon:minimize,close", 1, 0},
		{driver.WindowConstraints{NoResize: true}, "icon:minimize,close", 1, 0},
		{driver.WindowConstraints{NoMinimize: true, NoMaximize: true}, "icon:close", 0, 0},
		{driver.WindowConstraints{}, "icon:minimize,maximize,close", 1, 1},
	} {
		w.onLoop(t, func() {
			w.setConstraints(tc.c)
			if tc.c.NoMinimize {
				gtkWidgetActivateAction(w.win, cstr("window.minimize"), 0)
			}
			if tc.c.NoMaximize || tc.c.NoResize {
				gtkWidgetActivateAction(w.win, cstr("window.toggle-maximized"), 0)
			}
		})
		time.Sleep(50 * time.Millisecond)
		w.onLoop(t, func() {
			if tc.c.NoMinimize && gdkToplevelGetState(gtkNativeGetSurface(w.win))&toplevelMinimized != 0 {
				t.Error("disabled native minimize action ran")
			}
			if (tc.c.NoMaximize || tc.c.NoResize) && gtkWindowIsMaximized(w.win) != 0 {
				t.Error("disabled native maximize action ran")
			}
			bar := titlebar(w.win)
			if bar == 0 {
				t.Error("constraints installed no native header")
				return
			}
			if got := goString(layout(bar)); got != tc.want {
				t.Errorf("header layout=%q, want %q", got, tc.want)
			}
			count := func(class string) int {
				var walk func(uintptr) int
				walk = func(widget uintptr) int {
					if gtkWidgetGetVisible(widget) == 0 {
						return 0
					}
					n := 0
					if hasClass(widget, cstr(class)) != 0 {
						n++
					}
					for child := gtkWidgetGetFirstChild(widget); child != 0; child = gtkWidgetGetNextSibling(child) {
						n += walk(child)
					}
					return n
				}
				return walk(bar)
			}
			if min, max, close := count("minimize"), count("maximize"), count("close"); min > tc.minimize || max > tc.maximize || close != 1 || drawsOnX11() && (min != tc.minimize || max != tc.maximize) {
				t.Errorf("native buttons minimize=%d maximize=%d close=%d, want %d,%d,1", min, max, close, tc.minimize, tc.maximize)
			}
		})
	}
	w.onLoop(t, func() {
		w.setConstraints(driver.WindowConstraints{NoMaximize: true})
		setString(gtkSettingsGetDefault(), cstr("gtk-decoration-layout"), cstr("close,maximize:minimize"), 0)
		if got := goString(layout(titlebar(w.win))); got != "close:minimize" {
			t.Errorf("changed session layout=%q, want close:minimize", got)
		}
		w.windowCommand(&driver.WindowCommand{Action: driver.WindowSetTitleBar, TitleBar: driver.TitleBarHidden})
	})
	time.Sleep(50 * time.Millisecond)
	w.onLoop(t, func() {
		if gtkWidgetGetMapped(titlebar(w.win)) != 0 {
			t.Error("hidden titlebar still maps its native header")
		}
		w.windowCommand(&driver.WindowCommand{Action: driver.WindowSetTitleBar, TitleBar: driver.TitleBarDefault})
	})
}
