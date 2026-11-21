//go:build linux && !android

package linux

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// The compositor owns the Wayland connection GTK already uses. The
// extension proxies have their own queue, so registry discovery never
// dispatches a GTK callback inside an environment read.
type wlInterface struct {
	name                 *byte
	version, methodCount int32
	methods              *wlPointerMessage
	eventCount           int32
	events               *wlPointerMessage
}

type wlPointerMessage struct {
	name, signature *byte
	types           **wlInterface
}

var wlPointer struct {
	getDisplayType                     func() uintptr
	getDisplay, getSurface, getPointer func(uintptr) uintptr
	getSeat, seatPointer               func(uintptr) uintptr
	pointerSurface                     func(uintptr, *float64, *float64) uintptr
	cursorNew                          func(*byte, uintptr) uintptr
	getCursor                          func(uintptr) uintptr
	setCursor                          func(uintptr, uintptr)
	ref                                func(uintptr) uintptr

	createQueue         func(uintptr) uintptr
	destroyQueue        func(uintptr)
	setQueue            func(uintptr, uintptr)
	roundtrip, dispatch func(uintptr, uintptr) int32
	flush               func(uintptr) int32
	destroy             func(uintptr)
	addListener         func(uintptr, *uintptr, uintptr) int32
	marshal             func(uintptr, uint32, *wlInterface, uint32, uint32, *uintptr) uintptr

	registry, relativeManager, constraintsManager, relative, locked *wlInterface
	registryEvents                                                  [2]uintptr
	relativeEvents                                                  [1]uintptr
	lockedEvents                                                    [2]uintptr
	pin                                                             runtime.Pinner
	poll                                                            uintptr
}

var bindWaylandPointer = sync.OnceValue(func() bool {
	lib, err := purego.Dlopen("libwayland-client.so.0", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return false
	}
	for name, fn := range map[string]any{
		"gdk_wayland_display_get_type":       &wlPointer.getDisplayType,
		"gdk_wayland_display_get_wl_display": &wlPointer.getDisplay,
		"gdk_wayland_surface_get_wl_surface": &wlPointer.getSurface,
		"gdk_wayland_device_get_wl_pointer":  &wlPointer.getPointer,
		"gdk_display_get_default_seat":       &wlPointer.getSeat,
		"gdk_seat_get_pointer":               &wlPointer.seatPointer,
		"gdk_device_get_surface_at_position": &wlPointer.pointerSurface,
		"gdk_cursor_new_from_name":           &wlPointer.cursorNew,
		"gdk_surface_get_cursor":             &wlPointer.getCursor,
		"gdk_surface_set_cursor":             &wlPointer.setCursor,
	} {
		symbol, err := purego.Dlsym(gtkLib, name)
		if err != nil {
			return false
		}
		purego.RegisterFunc(fn, symbol)
	}
	for name, fn := range map[string]any{
		"wl_display_create_queue":           &wlPointer.createQueue,
		"wl_event_queue_destroy":            &wlPointer.destroyQueue,
		"wl_proxy_set_queue":                &wlPointer.setQueue,
		"wl_display_roundtrip_queue":        &wlPointer.roundtrip,
		"wl_display_dispatch_queue_pending": &wlPointer.dispatch,
		"wl_display_flush":                  &wlPointer.flush,
		"wl_proxy_destroy":                  &wlPointer.destroy,
		"wl_proxy_add_listener":             &wlPointer.addListener,
		"wl_proxy_marshal_array_flags":      &wlPointer.marshal,
	} {
		symbol, err := purego.Dlsym(lib, name)
		if err != nil {
			return false
		}
		purego.RegisterFunc(fn, symbol)
	}
	var pointer, surface, region *wlInterface
	for name, into := range map[string]**wlInterface{
		"wl_registry_interface": &wlPointer.registry,
		"wl_pointer_interface":  &pointer,
		"wl_surface_interface":  &surface,
		"wl_region_interface":   &region,
	} {
		address, err := purego.Dlsym(lib, name)
		if err != nil {
			return false
		}
		*into = (*wlInterface)(cPointer(address))
	}
	purego.RegisterLibFunc(&wlPointer.ref, gobjectLib, "g_object_ref")
	// Version 1 of the two Wayland pointer extensions. The type table
	// includes every wire argument, with nil entries for scalars.
	destroy := newWLPointerMessage("destroy", "")
	wlPointer.relative = newWLPointerInterface("zwp_relative_pointer_v1",
		[]wlPointerMessage{destroy},
		[]wlPointerMessage{newWLPointerMessage("relative_motion", "uuffff", nil, nil, nil, nil, nil, nil)})
	wlPointer.locked = newWLPointerInterface("zwp_locked_pointer_v1",
		[]wlPointerMessage{destroy},
		[]wlPointerMessage{newWLPointerMessage("locked", ""), newWLPointerMessage("unlocked", "")})
	wlPointer.relativeManager = newWLPointerInterface("zwp_relative_pointer_manager_v1",
		[]wlPointerMessage{destroy, newWLPointerMessage("get_relative_pointer", "no", wlPointer.relative, pointer)}, nil)
	wlPointer.constraintsManager = newWLPointerInterface("zwp_pointer_constraints_v1",
		[]wlPointerMessage{destroy, newWLPointerMessage("lock_pointer", "noo?ou", wlPointer.locked, surface, pointer, region, nil)}, nil)
	wlPointer.registryEvents = [2]uintptr{newCallback(wlPointerGlobal), newCallback(wlPointerGlobalRemoved)}
	wlPointer.relativeEvents = [1]uintptr{newCallback(wlPointerMotion)}
	wlPointer.lockedEvents = [2]uintptr{newCallback(wlPointerLocked), newCallback(wlPointerUnlocked)}
	wlPointer.poll = newCallback(wlPointerPoll)
	return true
})

func pinnedWLString(s string) *byte {
	p := cstr(s)
	wlPointer.pin.Pin(p)
	return p
}

func newWLPointerMessage(name, signature string, types ...*wlInterface) wlPointerMessage {
	m := wlPointerMessage{name: pinnedWLString(name), signature: pinnedWLString(signature), types: unsafe.SliceData(types)}
	if m.types != nil {
		wlPointer.pin.Pin(m.types)
	}
	return m
}

func newWLPointerInterface(name string, methods, events []wlPointerMessage) *wlInterface {
	i := &wlInterface{name: pinnedWLString(name), version: 1, methodCount: int32(len(methods)), methods: unsafe.SliceData(methods),
		eventCount: int32(len(events)), events: unsafe.SliceData(events)}
	wlPointer.pin.Pin(i)
	if i.methods != nil {
		wlPointer.pin.Pin(i.methods)
	}
	if i.events != nil {
		wlPointer.pin.Pin(i.events)
	}
	return i
}

type wlPointerServer struct {
	display, queue, registry      uintptr
	relative, constraints         uintptr
	relativeName, constraintsName uint32
}

var (
	wlPointerServers    = map[uintptr]*wlPointerServer{}
	wlPointerRegistries = map[uintptr]*wlPointerServer{}
)

func waylandPointerDisplay() *wlPointerServer {
	gdk := gdkDisplayGetDefault()
	if gdk == 0 || !bindWaylandPointer() || gTypeIsA(gdk, wlPointer.getDisplayType()) == 0 {
		return nil
	}
	server, ok := wlPointerServers[gdk]
	if !ok {
		server = discoverWLPointer(gdk)
		wlPointerServers[gdk] = server
	}
	if server == nil || server.relative == 0 || server.constraints == 0 {
		return nil
	}
	return server
}

func discoverWLPointer(gdk uintptr) *wlPointerServer {
	display := wlPointer.getDisplay(gdk)
	if display == 0 {
		return nil
	}
	queue := wlPointer.createQueue(display)
	if queue == 0 {
		return nil
	}
	// wl_display.get_registry(new ID)
	registry := wlPointerRequest(display, 1, wlPointer.registry, 0)
	if registry == 0 {
		wlPointer.destroyQueue(queue)
		return nil
	}
	wlPointer.setQueue(registry, queue)
	server := &wlPointerServer{display: display, queue: queue, registry: registry}
	wlPointerRegistries[registry] = server
	if wlPointer.addListener(registry, &wlPointer.registryEvents[0], 0) != 0 || wlPointer.roundtrip(display, queue) < 0 {
		delete(wlPointerRegistries, registry)
		wlPointer.destroy(registry)
		wlPointer.destroyQueue(queue)
		return nil
	}
	return server
}

func wlPointerGlobal(_ uintptr, registry uintptr, name uint32, iface *byte, _ uint32) {
	s := wlPointerRegistries[registry]
	if s == nil {
		return
	}
	var i *wlInterface
	switch goString(uintptr(unsafe.Pointer(iface))) {
	case "zwp_relative_pointer_manager_v1":
		i = wlPointer.relativeManager
	case "zwp_pointer_constraints_v1":
		i = wlPointer.constraintsManager
	default:
		return
	}
	// wl_registry.bind(name, interface name, version, new ID)
	proxy := wlPointerRequest(registry, 0, i, uintptr(name), uintptr(unsafe.Pointer(i.name)), 1, 0)
	if i == wlPointer.relativeManager {
		s.relative, s.relativeName = proxy, name
	} else {
		s.constraints, s.constraintsName = proxy, name
	}
}

func wlPointerGlobalRemoved(_ uintptr, registry uintptr, name uint32) {
	s := wlPointerRegistries[registry]
	if s == nil {
		return
	}
	if name == s.relativeName && s.relative != 0 {
		wlPointerDestroy(s.relative)
		s.relative = 0
	}
	if name == s.constraintsName && s.constraints != 0 {
		wlPointerDestroy(s.constraints)
		s.constraints = 0
	}
}

func wlPointerRequest(proxy uintptr, opcode uint32, iface *wlInterface, args ...uintptr) uintptr {
	return wlPointer.marshal(proxy, opcode, iface, 1, 0, unsafe.SliceData(args))
}

func wlPointerDestroy(proxy uintptr) {
	if proxy != 0 {
		const destroy = 1 // WL_MARSHAL_FLAG_DESTROY
		wlPointer.marshal(proxy, 0, nil, 1, destroy, nil)
	}
}

type wlLockedPointer struct {
	server                          *wlPointerServer
	surface, relative, lock, cursor uintptr
	timer                           uint32
	active                          bool
}

func (w *window) lockWaylandPointer() pointerCapture {
	server := waylandPointerDisplay()
	top := gtkNativeGetSurface(gtkWidgetGetNative(w.surface))
	if server == nil || top == 0 {
		return nil
	}
	seat := wlPointer.getSeat(gdkDisplayGetDefault())
	if seat == 0 {
		return nil
	}
	device := wlPointer.seatPointer(seat)
	var x, y float64
	if device == 0 || wlPointer.pointerSurface(device, &x, &y) != top {
		return nil
	}
	pointer, surface := wlPointer.getPointer(device), wlPointer.getSurface(top)
	if pointer == 0 || surface == 0 {
		return nil
	}

	p := &wlLockedPointer{server: server, surface: top}
	p.relative = wlPointerRequest(server.relative, 1, wlPointer.relative, 0, pointer)
	const oneshot = 1
	p.lock = wlPointerRequest(server.constraints, 1, wlPointer.locked, 0, surface, pointer, 0, oneshot)
	if p.relative == 0 || p.lock == 0 ||
		wlPointer.addListener(p.relative, &wlPointer.relativeEvents[0], w.handle) != 0 ||
		wlPointer.addListener(p.lock, &wlPointer.lockedEvents[0], w.handle) != 0 {
		p.close()
		return nil
	}
	// GTK reads this display's socket. Drain our queue even when a
	// relative motion has no corresponding GTK event.
	p.timer = gTimeoutAdd(4, wlPointer.poll, w.handle)
	wlPointer.flush(server.display)
	return p
}

func (p *wlLockedPointer) close() {
	if p.timer != 0 {
		gSourceRemove(p.timer)
	}
	if p.active {
		wlPointer.setCursor(p.surface, p.cursor)
		if p.cursor != 0 {
			gObjectUnref(p.cursor)
		}
	}
	wlPointerDestroy(p.relative)
	wlPointerDestroy(p.lock)
	wlPointer.flush(p.server.display)
}

func wlCurrentPointer(data uintptr) (*window, *wlLockedPointer) {
	w := windowOf(data)
	if w == nil || lockedPointer.owner != w {
		return nil, nil
	}
	p, _ := lockedPointer.capture.(*wlLockedPointer)
	return w, p
}

func wlPointerPoll(data uintptr) int32 {
	w, p := wlCurrentPointer(data)
	if p == nil {
		return 0
	}
	if wlPointer.dispatch(p.server.display, p.server.queue) < 0 {
		w.releasePointer(true)
		return 0
	}
	if lockedPointer.capture != p {
		return 0
	}
	return 1
}

func wlPointerLocked(data, proxy uintptr) {
	_, p := wlCurrentPointer(data)
	if p == nil || p.lock != proxy {
		return
	}
	if cursor := wlPointer.getCursor(p.surface); cursor != 0 {
		p.cursor = wlPointer.ref(cursor)
	}
	hidden := wlPointer.cursorNew(cstr("none"), 0)
	wlPointer.setCursor(p.surface, hidden)
	if hidden != 0 {
		gObjectUnref(hidden)
	}
	p.active = true
}

func wlPointerUnlocked(data, proxy uintptr) {
	w, p := wlCurrentPointer(data)
	if p != nil && p.lock == proxy {
		w.releasePointer(true)
	}
}

func wlPointerMotion(data, proxy uintptr, _, _ uint32, _, _, dx, dy int32) {
	w, p := wlCurrentPointer(data)
	if p != nil && p.relative == proxy && p.active {
		// wl_fixed_t is a signed 24.8 fixed-point number.
		w.pointerMotion(float64(dx)/256, float64(dy)/256)
	}
}
