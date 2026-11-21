//go:build linux && !android

package linux

import (
	"slices"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	xGenericEvent      = 35
	xiRawMotion        = 17
	xiAllMasterDevices = 1
)

var xPointer = struct {
	root                                             func(display uintptr) uintptr
	queryExtension                                   func(display uintptr, name *byte, opcode, firstEvent, firstError *int32) int32
	queryPointer                                     func(display, window uintptr, root, child *uintptr, rootX, rootY, x, y *int32, mask *uint32) int32
	translate                                        func(display, from, to uintptr, x, y int32, intoX, intoY *int32, child *uintptr) int32
	createWindow                                     func(display, parent uintptr, x, y int32, width, height, border uint32, depth int32, class uint32, visual, mask uintptr, attrs *x11WindowAttributes) uintptr
	mapWindow, destroyWindow, freePixmap, freeCursor func(display, resource uintptr) int32
	blankPixmap                                      func(display, drawable uintptr, bits *byte, width, height uint32) uintptr
	newCursor                                        func(display, src, mask uintptr, fg, bg *xPointerColor, x, y uint32) uintptr
	grab                                             func(display, window uintptr, owner int32, mask uint32, pointerMode, keyboardMode int32, confine, cursor, time uintptr) int32
	ungrab                                           func(display, time uintptr) int32
	warp                                             func(display, from, to uintptr, fromX, fromY int32, fromWidth, fromHeight uint32, x, y int32) int32
	selected                                         func(display, window uintptr, count *int32) uintptr
	selectEvents                                     func(display, window uintptr, masks *xiEventMask, count int32) int32
	getEventData                                     func(display uintptr, event *xGenericCookie) int32
	freeEventData                                    func(display uintptr, event *xGenericCookie)
	free                                             func(ptr uintptr) int32
	cb                                               uintptr
}{}

var bindXPointer = sync.OnceValue(func() bool {
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return false
	}
	xi, err := purego.Dlopen("libXi.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		purego.Dlclose(lib)
		return false
	}
	for name, fn := range map[string]any{
		"XDefaultRootWindow":    &xPointer.root,
		"XQueryExtension":       &xPointer.queryExtension,
		"XQueryPointer":         &xPointer.queryPointer,
		"XTranslateCoordinates": &xPointer.translate,
		"XCreateWindow":         &xPointer.createWindow,
		"XMapWindow":            &xPointer.mapWindow,
		"XDestroyWindow":        &xPointer.destroyWindow,
		"XCreateBitmapFromData": &xPointer.blankPixmap,
		"XCreatePixmapCursor":   &xPointer.newCursor,
		"XFreePixmap":           &xPointer.freePixmap,
		"XFreeCursor":           &xPointer.freeCursor,
		"XGrabPointer":          &xPointer.grab,
		"XUngrabPointer":        &xPointer.ungrab,
		"XWarpPointer":          &xPointer.warp,
		"XGetEventData":         &xPointer.getEventData,
		"XFreeEventData":        &xPointer.freeEventData,
		"XFree":                 &xPointer.free,
	} {
		purego.RegisterLibFunc(fn, lib, name)
	}
	purego.RegisterLibFunc(&xPointer.selected, xi, "XIGetSelectedEvents")
	purego.RegisterLibFunc(&xPointer.selectEvents, xi, "XISelectEvents")
	xPointer.cb = newCallback(xPointerEvent)
	return true
})

type xPointerColor struct {
	pixel            uintptr
	red, green, blue uint16
	flags, pad       uint8
}

type xiEventMask struct {
	device, len int32
	mask        *byte
}

type xGenericCookie struct {
	typ              int32
	serial           uintptr
	sendEvent        int32
	display          uintptr
	extension, event int32
	cookie           uint32
	data             uintptr
}

type xiRawEvent struct {
	typ                           int32
	serial                        uintptr
	sendEvent                     int32
	display                       uintptr
	extension, event              int32
	time                          uintptr
	device, source, detail, flags int32
	maskLen                       int32
	mask                          *byte
	values, rawValues             *float64
}

type xPointerServer struct {
	gdk, display, root uintptr
	opcode             int32
}

var xPointerServers = map[uintptr]*xPointerServer{}

func xPointerDisplay() *xPointerServer {
	bindX11Panels()
	gdk := gdkDisplayGetDefault()
	if !x11PanelsReady || gdk == 0 || gTypeIsA(gdk, gdkX11DisplayGetType()) == 0 || !bindXPointer() {
		return nil
	}
	if server, ok := xPointerServers[gdk]; ok {
		return server
	}
	xPointerServers[gdk] = nil
	display := gdkX11PanelXDisplay(gdk)
	if display == 0 {
		return nil
	}
	var opcode, firstEvent, firstError int32
	// GTK has already negotiated XInput2 on its display.
	if xPointer.queryExtension(display, cstr("XInputExtension"), &opcode, &firstEvent, &firstError) == 0 {
		return nil
	}
	server := &xPointerServer{gdk: gdk, display: display, root: xPointer.root(display), opcode: opcode}
	xPointerServers[gdk] = server
	return server
}

type xLockedPointer struct {
	server      *xPointerServer
	x, y        int32
	box, cursor uintptr
	handler     uint64
	previous    []byte
	grabbed     bool
}

func (w *window) lockXPointer() pointerCapture {
	s := xPointerDisplay()
	top := gtkNativeGetSurface(gtkWidgetGetNative(w.surface))
	if s == nil || top == 0 {
		return nil
	}
	xid := gdkX11SurfaceGetXID(top)
	if xid == 0 {
		return nil
	}
	p := &xLockedPointer{server: s}
	var root, child uintptr
	var x, y int32
	var mask uint32
	if xPointer.queryPointer(s.display, s.root, &root, &child, &p.x, &p.y, &x, &y, &mask) == 0 ||
		xPointer.translate(s.display, s.root, xid, p.x, p.y, &x, &y, &child) == 0 {
		return nil
	}
	// An invisible child gives XGrabPointer a one-pixel confine window.
	const inputOnly = 2
	p.box = xPointer.createWindow(s.display, xid, x, y, 1, 1, 0, 0, inputOnly, 0, 0, nil)
	if p.box == 0 {
		return nil
	}
	xPointer.mapWindow(s.display, p.box)
	var blank byte
	pixmap := xPointer.blankPixmap(s.display, xid, &blank, 1, 1)
	var color xPointerColor
	if pixmap != 0 {
		p.cursor = xPointer.newCursor(s.display, pixmap, pixmap, &color, &color, 0, 0)
		xPointer.freePixmap(s.display, pixmap)
	}
	if p.cursor == 0 {
		p.close()
		return nil
	}
	const buttonEvents = 1<<2 | 1<<3
	const async = 1
	if xPointer.grab(s.display, xid, 0, buttonEvents, async, async, p.box, p.cursor, 0) != 0 {
		p.close()
		return nil
	}
	p.grabbed = true
	p.previous = s.rawMask()
	maskBytes := make([]byte, max(len(p.previous), xiRawMotion/8+1))
	copy(maskBytes, p.previous)
	maskBytes[xiRawMotion/8] |= 1 << (xiRawMotion % 8)
	if !s.setRawMask(maskBytes) {
		p.close()
		return nil
	}
	p.handler = gSignalConnect(s.gdk, "xevent", xPointer.cb, w.handle)
	if p.handler == 0 {
		s.setRawMask(p.previous)
		p.close()
		return nil
	}
	x11Flush(s.display)
	return p
}

func (s *xPointerServer) rawMask() []byte {
	var count int32
	masks := xPointer.selected(s.display, s.root, &count)
	if masks == 0 {
		return nil
	}
	defer xPointer.free(masks)
	for _, m := range unsafe.Slice((*xiEventMask)(cPointer(masks)), count) {
		if m.device == xiAllMasterDevices {
			return slices.Clone(unsafe.Slice(m.mask, m.len))
		}
	}
	return nil
}

func (s *xPointerServer) setRawMask(mask []byte) bool {
	m := xiEventMask{device: xiAllMasterDevices, len: int32(len(mask)), mask: unsafe.SliceData(mask)}
	return xPointer.selectEvents(s.display, s.root, &m, 1) == 0
}

func (p *xLockedPointer) close() {
	s := p.server
	if p.handler != 0 {
		gSignalHandlerDisconnect(s.gdk, p.handler)
		s.setRawMask(p.previous)
	}
	if p.grabbed {
		xPointer.ungrab(s.display, 0)
		xPointer.warp(s.display, 0, s.root, 0, 0, 0, 0, p.x, p.y)
	}
	if p.cursor != 0 {
		xPointer.freeCursor(s.display, p.cursor)
	}
	xPointer.destroyWindow(s.display, p.box)
	x11Flush(s.display)
}

func xPointerEvent(_ uintptr, event, data uintptr) int32 {
	w := windowOf(data)
	p, ok := lockedPointer.capture.(*xLockedPointer)
	if w == nil || lockedPointer.owner != w || !ok || event == 0 {
		return 0
	}
	cookie := (*xGenericCookie)(cPointer(event))
	if cookie.typ != xGenericEvent || cookie.extension != p.server.opcode || cookie.event != xiRawMotion {
		return 0
	}
	if cookie.data == 0 {
		if xPointer.getEventData(p.server.display, cookie) == 0 {
			return 0
		}
		defer xPointer.freeEventData(p.server.display, cookie)
	}
	raw := (*xiRawEvent)(cPointer(cookie.data))
	if raw.maskLen > 0 && raw.mask != nil && raw.rawValues != nil {
		var dx, dy float64
		switch *raw.mask & 3 {
		case 1:
			dx = *raw.rawValues
		case 2:
			dy = *raw.rawValues
		case 3:
			values := unsafe.Slice(raw.rawValues, 2)
			dx, dy = values[0], values[1]
		}
		w.pointerMotion(dx, dy)
	}
	return 1
}
