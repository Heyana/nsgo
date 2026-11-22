//go:build linux && !android

package linux

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Trusted web view input on Linux: WebKitGTK 6.0 runs on GTK 4, which has no
// synthetic GdkEvent constructor, so trusted clicks and key events go through
// X11's XTest extension into the window's X11 surface (on Xorg, Xvfb and
// Xwayland), producing real XInput2 events that GTK 4 delivers to the
// WebKitWebView as trusted DOM events with user activation. Characters absent
// from the active XKB keymap fall back to WebKit's InsertText editing command.
var xWebInput = struct {
	ready           bool
	errorTrapPush   func(display uintptr)
	errorTrapPop    func(display uintptr) int32
	mapKeyval       func(display uintptr, keyval uint32, keys **gdkKeymapKey, nKeys *int32) int32
	defaultRoot     func(display uintptr) uintptr
	translateCoords func(display, from, to uintptr, x, y int32, dstX, dstY *int32, child *uintptr) int32
	warpPointer     func(display, src, dst uintptr, srcX, srcY int32, srcW, srcH uint32, dstX, dstY int32) int32
	getInputFocus   func(display uintptr, focus *uintptr, revert *int32) int32
	setInputFocus   func(display, focus uintptr, revert int32, time uintptr) int32
	keysymToKeycode func(display, keysym uintptr) uint8
	sync            func(display uintptr, discard int32) int32
	queryExtension  func(display uintptr, eventBase, errorBase, major, minor *int32) int32
	fakeMotion      func(display uintptr, screen, x, y int32, delay uintptr) int32
	fakeButton      func(display uintptr, button uint32, press int32, delay uintptr) int32
	fakeKey         func(display uintptr, keycode uint32, press int32, delay uintptr) int32
}{}

var bindX11WebInput = sync.OnceFunc(func() {
	if bindX11Panels(); !x11PanelsReady {
		return
	}
	for _, sym := range []string{"gdk_x11_display_error_trap_push", "gdk_x11_display_error_trap_pop", "gdk_display_map_keyval"} {
		if _, err := purego.Dlsym(gtkLib, sym); err != nil {
			return
		}
	}
	x11, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		if x11, err = purego.Dlopen("libX11.so", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			return
		}
	}
	xtst, err := purego.Dlopen("libXtst.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		if xtst, err = purego.Dlopen("libXtst.so", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			return
		}
	}
	for _, sym := range []string{"XDefaultRootWindow", "XTranslateCoordinates", "XWarpPointer", "XGetInputFocus", "XSetInputFocus", "XKeysymToKeycode", "XSync"} {
		if _, err := purego.Dlsym(x11, sym); err != nil {
			return
		}
	}
	for _, sym := range []string{"XTestQueryExtension", "XTestFakeMotionEvent", "XTestFakeButtonEvent", "XTestFakeKeyEvent"} {
		if _, err := purego.Dlsym(xtst, sym); err != nil {
			return
		}
	}
	purego.RegisterLibFunc(&xWebInput.errorTrapPush, gtkLib, "gdk_x11_display_error_trap_push")
	purego.RegisterLibFunc(&xWebInput.errorTrapPop, gtkLib, "gdk_x11_display_error_trap_pop")
	purego.RegisterLibFunc(&xWebInput.mapKeyval, gtkLib, "gdk_display_map_keyval")
	purego.RegisterLibFunc(&xWebInput.defaultRoot, x11, "XDefaultRootWindow")
	purego.RegisterLibFunc(&xWebInput.translateCoords, x11, "XTranslateCoordinates")
	purego.RegisterLibFunc(&xWebInput.warpPointer, x11, "XWarpPointer")
	purego.RegisterLibFunc(&xWebInput.getInputFocus, x11, "XGetInputFocus")
	purego.RegisterLibFunc(&xWebInput.setInputFocus, x11, "XSetInputFocus")
	purego.RegisterLibFunc(&xWebInput.keysymToKeycode, x11, "XKeysymToKeycode")
	purego.RegisterLibFunc(&xWebInput.sync, x11, "XSync")
	purego.RegisterLibFunc(&xWebInput.queryExtension, xtst, "XTestQueryExtension")
	purego.RegisterLibFunc(&xWebInput.fakeMotion, xtst, "XTestFakeMotionEvent")
	purego.RegisterLibFunc(&xWebInput.fakeButton, xtst, "XTestFakeButtonEvent")
	purego.RegisterLibFunc(&xWebInput.fakeKey, xtst, "XTestFakeKeyEvent")
	xWebInput.ready = true
})

// canSendWebInput reports whether GDK draws on X11 with the XTest extension,
// where sendWebInput can deliver trusted pointer and key input to WebKitGTK.
func (w *window) canSendWebInput() bool {
	if bindX11WebInput(); !xWebInput.ready {
		return false
	}
	gdk := gdkDisplayGetDefault()
	if gdk == 0 || gTypeIsA(gdk, gdkX11DisplayGetType()) == 0 {
		return false
	}
	display := gdkX11PanelXDisplay(gdk)
	if display == 0 {
		return false
	}
	var ev, errBase, major, minor int32
	return xWebInput.queryExtension(display, &ev, &errBase, &major, &minor) != 0
}

// sendWebInput gives v's WebKitWebView trusted input, taking keyboard focus
// first as a click does.
func (w *window) sendWebInput(v *view, in driver.WebInput) {
	if v == nil || v.web == nil || v.web.widget == 0 || !w.canSendWebInput() {
		return
	}
	page := v.web.widget
	gtkWidgetGrabFocus(page)
	display, xid, ok := w.x11Window()
	if !ok {
		return
	}
	if w.opts.test != nil && !prepareTestWebInput(display) {
		return
	}
	gdk := gdkDisplayGetDefault()
	switch in.Kind {
	case driver.WebInputClick:
		w.sendWebClick(v, page, gdk, display, xid, in.Point)
	case driver.WebInputText:
		w.focusWebX11(gdk, display, xid)
		shiftCode := uint32(xWebInput.keysymToKeycode(display, uintptr(webKeyShiftL)))
		for _, step := range webInputTextSteps(in.Text) {
			if code, shift, ok := mapWebKeyval(gdk, display, step.keyval, false); ok {
				sendX11Key(gdk, display, code, shift, shiftCode)
				continue
			}
			if step.text != "" {
				webkitWebViewExecuteEditingCommandWithArgument(page, cstr("InsertText"), cstr(step.text))
			}
		}
	case driver.WebInputKey:
		kv, ok := webKeyvalForName(in.Key)
		if !ok {
			return
		}
		w.focusWebX11(gdk, display, xid)
		shiftCode := uint32(xWebInput.keysymToKeycode(display, uintptr(webKeyShiftL)))
		if code, shift, ok := mapWebKeyval(gdk, display, kv, true); ok {
			sendX11Key(gdk, display, code, shift, shiftCode)
		}
	}
}

func (w *window) sendWebClick(v *view, page, gdk, display, xid uintptr, pt driver.Point) {
	zoom := webkitWebViewGetZoomLevel(page)
	cx, cy := webCSSPoint(pt, zoom)
	native := gtkWidgetGetNative(page)
	if native == 0 {
		native = w.win
	}
	var out graphenePoint
	if native == 0 || gtkWidgetComputePoint(page, native, &graphenePoint{x: cx, y: cy}, &out) == 0 {
		at := w.frameAt(v)
		out = graphenePoint{x: float32(at.X) + cx, y: float32(at.Y) + cy}
	}
	var tx, ty float64
	if native != 0 {
		gtkNativeGetSurfaceTransform(native, &tx, &ty)
	}
	scale := 1.0
	if surface := gtkNativeGetSurface(native); surface != 0 {
		scale = gdkSurfaceGetScale(surface)
	} else if f := gtkWidgetGetScaleFactor(page); f > 0 {
		scale = float64(f)
	}
	px, py := webSurfacePixel(float64(out.x), float64(out.y), tx, ty, scale)
	xWebInput.errorTrapPush(gdk)
	var rx, ry int32
	var child uintptr
	if root := xWebInput.defaultRoot(display); root != 0 &&
		xWebInput.translateCoords(display, xid, root, px, py, &rx, &ry, &child) != 0 {
		recordTestWebInput(display)
		xWebInput.fakeMotion(display, -1, rx, ry, 0)
	} else {
		xWebInput.warpPointer(display, 0, xid, 0, 0, 0, 0, px, py)
	}
	recordTestWebInput(display)
	xWebInput.fakeButton(display, 1, 1, 0)
	recordTestWebInput(display)
	xWebInput.fakeButton(display, 1, 0, 0)
	xWebInput.sync(display, 0)
	_ = xWebInput.errorTrapPop(gdk)
}

func (w *window) focusWebX11(gdk, display, xid uintptr) {
	const revertToParent = 2
	xWebInput.errorTrapPush(gdk)
	var cur uintptr
	var revert int32
	xWebInput.getInputFocus(display, &cur, &revert)
	if cur != xid {
		xWebInput.setInputFocus(display, xid, revertToParent, 0)
		xWebInput.sync(display, 0)
	}
	_ = xWebInput.errorTrapPop(gdk)
}

func mapWebKeyval(gdk, display uintptr, keyval uint32, allowDirect bool) (keycode uint32, shift, ok bool) {
	if keyval == 0 {
		return 0, false, false
	}
	var raw *gdkKeymapKey
	var n int32
	if xWebInput.mapKeyval(gdk, keyval, &raw, &n) != 0 && raw != nil && n > 0 {
		defer gFree(uintptr(unsafe.Pointer(raw)))
		if code, s, found := chooseWebKeymapKey(unsafe.Slice(raw, int(n))); found {
			return code, s, true
		}
	}
	if allowDirect {
		if code := uint32(xWebInput.keysymToKeycode(display, uintptr(keyval))); code != 0 {
			return code, false, true
		}
	}
	return 0, false, false
}

func sendX11Key(gdk, display uintptr, keycode uint32, shift bool, shiftCode uint32) {
	xWebInput.errorTrapPush(gdk)
	if shift && shiftCode != 0 {
		recordTestWebInput(display)
		xWebInput.fakeKey(display, shiftCode, 1, 0)
	}
	recordTestWebInput(display)
	xWebInput.fakeKey(display, keycode, 1, 0)
	recordTestWebInput(display)
	xWebInput.fakeKey(display, keycode, 0, 0)
	if shift && shiftCode != 0 {
		recordTestWebInput(display)
		xWebInput.fakeKey(display, shiftCode, 0, 0)
	}
	xWebInput.sync(display, 0)
	_ = xWebInput.errorTrapPop(gdk)
}
