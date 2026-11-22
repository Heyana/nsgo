//go:build linux && !android

package linux

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var wlPointer struct {
	getSurface, getPointer func(uintptr) uintptr
	getSeat, seatPointer   func(uintptr) uintptr
	pointerSurface         func(uintptr, *float64, *float64) uintptr
	cursorNew              func(*byte, uintptr) uintptr
	getCursor              func(uintptr) uintptr
	setCursor              func(uintptr, uintptr)
	ref                    func(uintptr) uintptr

	relativeManager, constraintsManager, relative, locked *wlInterface
	registryEvents                                        [2]uintptr
	relativeEvents                                        [1]uintptr
	lockedEvents                                          [2]uintptr
	poll                                                  uintptr
}

var bindWaylandPointer = sync.OnceValue(func() bool {
	if !bindWaylandClient() {
		return false
	}
	for name, fn := range map[string]any{
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
	var pointer, surface, region *wlInterface
	for name, into := range map[string]**wlInterface{
		"wl_pointer_interface": &pointer,
		"wl_surface_interface": &surface,
		"wl_region_interface":  &region,
	} {
		address, err := purego.Dlsym(wlClient.lib, name)
		if err != nil {
			return false
		}
		*into = (*wlInterface)(cPointer(address))
	}
	purego.RegisterLibFunc(&wlPointer.ref, gobjectLib, "g_object_ref")
	// Version 1 of the two Wayland pointer extensions. The type table
	// includes every wire argument, with nil entries for scalars.
	destroy := newWLMessage("destroy", "")
	wlPointer.relative = newWLInterface("zwp_relative_pointer_v1", 1,
		[]wlMessage{destroy},
		[]wlMessage{newWLMessage("relative_motion", "uuffff", nil, nil, nil, nil, nil, nil)})
	wlPointer.locked = newWLInterface("zwp_locked_pointer_v1", 1,
		[]wlMessage{destroy},
		[]wlMessage{newWLMessage("locked", ""), newWLMessage("unlocked", "")})
	wlPointer.relativeManager = newWLInterface("zwp_relative_pointer_manager_v1", 1,
		[]wlMessage{destroy, newWLMessage("get_relative_pointer", "no", wlPointer.relative, pointer)}, nil)
	wlPointer.constraintsManager = newWLInterface("zwp_pointer_constraints_v1", 1,
		[]wlMessage{destroy, newWLMessage("lock_pointer", "noo?ou", wlPointer.locked, surface, pointer, region, nil)}, nil)
	wlPointer.registryEvents = [2]uintptr{newCallback(wlPointerGlobal), newCallback(wlPointerGlobalRemoved)}
	wlPointer.relativeEvents = [1]uintptr{newCallback(wlPointerMotion)}
	wlPointer.lockedEvents = [2]uintptr{newCallback(wlPointerLocked), newCallback(wlPointerUnlocked)}
	wlPointer.poll = newCallback(wlPointerPoll)
	return true
})

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
	if gdk == 0 || !bindWaylandPointer() || gTypeIsA(gdk, wlClient.getDisplayType()) == 0 {
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
	display := wlClient.getDisplay(gdk)
	if display == 0 {
		return nil
	}
	queue := wlClient.createQueue(display)
	if queue == 0 {
		return nil
	}
	// wl_display.get_registry(new ID)
	registry := wlPointerRequest(display, 1, wlClient.registry, 0)
	if registry == 0 {
		wlClient.destroyQueue(queue)
		return nil
	}
	wlClient.setQueue(registry, queue)
	server := &wlPointerServer{display: display, queue: queue, registry: registry}
	wlPointerRegistries[registry] = server
	if wlClient.addListener(registry, &wlPointer.registryEvents[0], 0) != 0 || wlClient.roundtrip(display, queue) < 0 {
		delete(wlPointerRegistries, registry)
		wlClient.destroy(registry)
		wlClient.destroyQueue(queue)
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
	return wlClient.marshal(proxy, opcode, iface, 1, 0, unsafe.SliceData(args))
}

func wlPointerDestroy(proxy uintptr) {
	if proxy != 0 {
		const destroy = 1 // WL_MARSHAL_FLAG_DESTROY
		wlClient.marshal(proxy, 0, nil, 1, destroy, nil)
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
		wlClient.addListener(p.relative, &wlPointer.relativeEvents[0], w.handle) != 0 ||
		wlClient.addListener(p.lock, &wlPointer.lockedEvents[0], w.handle) != 0 {
		p.close()
		return nil
	}
	// GTK reads this display's socket. Drain our queue even when a
	// relative motion has no corresponding GTK event.
	p.timer = gTimeoutAdd(4, wlPointer.poll, w.handle)
	wlClient.flush(server.display)
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
	wlClient.flush(p.server.display)
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
	if wlClient.dispatch(p.server.display, p.server.queue) < 0 {
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
