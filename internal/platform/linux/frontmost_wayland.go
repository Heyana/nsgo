//go:build linux && !android

package linux

import (
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// wlr-foreign-toplevel-management exposes compositor-owned application IDs and activated/fullscreen states. It does not expose PIDs. Compositors without this optional protocol remain unavailable; Xwayland's partial window list is never used as a substitute for a Wayland desktop.
var wlFrontmost struct {
	manager, handle *wlInterface
	registryEvents  [2]uintptr
	managerEvents   [2]uintptr
	handleEvents    [7]uintptr
	poll            uintptr
	desktopInfo     func(*byte) uintptr
	displayName     func(uintptr) uintptr
	names           map[string]string
}

var waylandFrontmostOnce sync.Once
var waylandFrontmostReady bool

func bindWaylandFrontmost() bool {
	waylandFrontmostOnce.Do(func() { waylandFrontmostReady = loadWaylandFrontmost() })
	return waylandFrontmostReady
}

func loadWaylandFrontmost() bool {
	if !bindWaylandClient() {
		return false
	}
	interfaces := map[string]*wlInterface{}
	for _, name := range []string{"wl_output_interface", "wl_seat_interface", "wl_surface_interface"} {
		address, err := purego.Dlsym(wlClient.lib, name)
		if err != nil {
			return false
		}
		interfaces[name] = (*wlInterface)(cPointer(address))
	}
	output, seat, surface := interfaces["wl_output_interface"], interfaces["wl_seat_interface"], interfaces["wl_surface_interface"]
	wlFrontmost.handle = newWLInterface("zwlr_foreign_toplevel_handle_v1", 2,
		[]wlMessage{
			newWLMessage("set_maximized", ""), newWLMessage("unset_maximized", ""),
			newWLMessage("set_minimized", ""), newWLMessage("unset_minimized", ""),
			newWLMessage("activate", "o", seat), newWLMessage("close", ""),
			newWLMessage("set_rectangle", "oiiii", surface, nil, nil, nil, nil), newWLMessage("destroy", ""),
			newWLMessage("set_fullscreen", "2?o", output), newWLMessage("unset_fullscreen", "2"),
		}, []wlMessage{
			newWLMessage("title", "s", nil), newWLMessage("app_id", "s", nil),
			newWLMessage("output_enter", "o", output), newWLMessage("output_leave", "o", output),
			newWLMessage("state", "a", nil), newWLMessage("done", ""), newWLMessage("closed", ""),
		})
	wlFrontmost.manager = newWLInterface("zwlr_foreign_toplevel_manager_v1", 2,
		[]wlMessage{newWLMessage("stop", "")}, []wlMessage{newWLMessage("toplevel", "n", wlFrontmost.handle), newWLMessage("finished", "")})
	wlFrontmost.registryEvents = [2]uintptr{newCallback(wlFrontmostGlobal), newCallback(wlFrontmostGlobalRemoved)}
	wlFrontmost.managerEvents = [2]uintptr{newCallback(wlFrontmostToplevel), newCallback(wlFrontmostFinished)}
	wlFrontmost.handleEvents = [7]uintptr{newCallback(wlFrontmostTitle), newCallback(wlFrontmostAppID), newCallback(wlFrontmostOutput), newCallback(wlFrontmostOutput), newCallback(wlFrontmostState), newCallback(wlFrontmostDone), newCallback(wlFrontmostClosed)}
	wlFrontmost.poll = newCallback(wlFrontmostPoll)
	purego.RegisterLibFunc(&wlFrontmost.desktopInfo, gioLib, "g_desktop_app_info_new")
	purego.RegisterLibFunc(&wlFrontmost.displayName, gioLib, "g_app_info_get_display_name")
	wlFrontmost.names = map[string]string{}
	return true
}

type wlFrontmostWindow struct {
	server                *wlFrontmostServer
	pending, app          driver.FrontmostAppInfo
	pendingActive, active bool
	serial                uint64
}

type wlFrontmostServer struct {
	display, queue, registry, manager uintptr
	global, version                   uint32
	timer                             uint32
	managerEnded                      bool
	finished                          bool
	serial                            uint64
	windows                           map[uintptr]*wlFrontmostWindow
}

var (
	wlFrontmostRegistries = map[uintptr]*wlFrontmostServer{}
	wlFrontmostManagers   = map[uintptr]*wlFrontmostServer{}
	wlFrontmostWindows    = map[uintptr]*wlFrontmostWindow{}
)

func watchWaylandFrontmost() *wlFrontmostServer {
	gdk := gdkDisplayGetDefault()
	if gdk == 0 || !bindWaylandFrontmost() || gTypeIsA(gdk, wlClient.getDisplayType()) == 0 {
		return nil
	}
	s := &wlFrontmostServer{display: wlClient.getDisplay(gdk), windows: map[uintptr]*wlFrontmostWindow{}}
	s.queue = wlClient.createQueue(s.display)
	if s.queue == 0 {
		return nil
	}
	s.registry = wlClient.marshal(s.display, 1, wlClient.registry, 1, 0, unsafe.SliceData([]uintptr{0}))
	if s.registry == 0 {
		s.close()
		return nil
	}
	wlClient.setQueue(s.registry, s.queue)
	wlFrontmostRegistries[s.registry] = s
	// The first roundtrip discovers the manager; the second receives its initial toplevels, complete through each handle's atomic done event.
	if wlClient.addListener(s.registry, &wlFrontmost.registryEvents[0], 0) != 0 ||
		wlClient.roundtrip(s.display, s.queue) < 0 || s.manager == 0 ||
		wlClient.roundtrip(s.display, s.queue) < 0 || s.finished {
		s.close()
		return nil
	}
	frontmost = s.current()
	s.timer = gTimeoutAdd(50, wlFrontmost.poll, s.registry)
	return s
}

func wlFrontmostGlobal(_, registry uintptr, name uint32, iface *byte, version uint32) {
	s := wlFrontmostRegistries[registry]
	if s == nil || s.manager != 0 || goString(uintptr(unsafe.Pointer(iface))) != "zwlr_foreign_toplevel_manager_v1" || version < 2 {
		return
	}
	s.version = 2 // Fullscreen state was introduced in version 2.
	args := []uintptr{uintptr(name), uintptr(unsafe.Pointer(wlFrontmost.manager.name)), 2, 0}
	s.manager = wlClient.marshal(registry, 0, wlFrontmost.manager, 2, 0, unsafe.SliceData(args))
	if s.manager == 0 {
		return
	}
	s.global = name
	wlFrontmostManagers[s.manager] = s
	if wlClient.addListener(s.manager, &wlFrontmost.managerEvents[0], 0) != 0 {
		s.finished = true
	}
}

func wlFrontmostGlobalRemoved(_, registry uintptr, name uint32) {
	if s := wlFrontmostRegistries[registry]; s != nil && s.global == name {
		s.finished = true
	}
}

func wlFrontmostToplevel(_, manager, handle uintptr) {
	s := wlFrontmostManagers[manager]
	if s == nil {
		return
	}
	w := &wlFrontmostWindow{server: s}
	s.windows[handle] = w
	wlFrontmostWindows[handle] = w
	if wlClient.addListener(handle, &wlFrontmost.handleEvents[0], 0) != 0 {
		s.finished = true
	}
}
func wlFrontmostFinished(_, manager uintptr) {
	if s := wlFrontmostManagers[manager]; s != nil {
		s.managerEnded = true
		s.finished = true
	}
}
func wlFrontmostTitle(_, _ uintptr, _ *byte) {} // Window titles are not application names.
func wlFrontmostOutput(_, _, _ uintptr)      {}

func wlFrontmostAppID(_, handle uintptr, id *byte) {
	if w := wlFrontmostWindows[handle]; w != nil {
		w.pending.ID = goString(uintptr(unsafe.Pointer(id)))
		w.pending.Name = waylandApplicationName(w.pending.ID)
	}
}

// wlArray is libwayland's wl_array on supported 64-bit Linux targets.
type wlArray struct {
	size, allocated uintptr
	data            unsafe.Pointer
}

func wlFrontmostState(_, handle uintptr, states *wlArray) {
	w := wlFrontmostWindows[handle]
	if w == nil {
		return
	}
	w.pendingActive, w.pending.FullScreen = false, false
	if states == nil || states.size%4 != 0 || states.size > 4096 || states.data == nil {
		return
	}
	for _, state := range unsafe.Slice((*uint32)(states.data), states.size/4) {
		switch state {
		case 2:
			w.pendingActive = true
		case 3:
			w.pending.FullScreen = true
		}
	}
}
func wlFrontmostDone(_, handle uintptr) {
	if w := wlFrontmostWindows[handle]; w != nil {
		w.app, w.active = w.pending, w.pendingActive
		w.server.serial++
		w.serial = w.server.serial
	}
}
func wlFrontmostClosed(_, handle uintptr) {
	if w := wlFrontmostWindows[handle]; w != nil {
		delete(w.server.windows, handle)
		delete(wlFrontmostWindows, handle)
		wlClient.marshal(handle, 7, nil, w.server.version, 1, nil)
	}
}

func (s *wlFrontmostServer) current() driver.FrontmostAppInfo {
	var newest *wlFrontmostWindow
	for _, w := range s.windows {
		if w.active && (newest == nil || w.serial > newest.serial) {
			newest = w
		}
	}
	if newest != nil {
		return newest.app
	}
	return driver.FrontmostAppInfo{}
}

func wlFrontmostPoll(registry uintptr) int32 {
	s := wlFrontmostRegistries[registry]
	if s == nil {
		return 0
	}
	if wlClient.dispatch(s.display, s.queue) < 0 || s.finished {
		s.close()
		frontmostWayland = nil
		frontmost = driver.FrontmostAppInfo{}
		refreshFrontmostWindows()
		return 0
	}
	// Commit all queued done events together, so a focus transfer never publishes half of a compositor's state update.
	reportFrontmost(s.current())
	wlClient.flush(s.display)
	return 1
}

func (s *wlFrontmostServer) close() {
	if s.timer != 0 {
		gSourceRemove(s.timer)
		s.timer = 0
	}
	for handle := range s.windows {
		delete(wlFrontmostWindows, handle)
		wlClient.marshal(handle, 7, nil, s.version, 1, nil)
	}
	if s.manager != 0 {
		delete(wlFrontmostManagers, s.manager)
		if !s.managerEnded {
			wlClient.marshal(s.manager, 0, nil, s.version, 0, nil)
		}
		wlClient.destroy(s.manager)
		s.manager = 0
	}
	if s.registry != 0 {
		delete(wlFrontmostRegistries, s.registry)
		wlClient.destroy(s.registry)
		s.registry = 0
	}
	if s.display != 0 {
		wlClient.flush(s.display)
	}
	if s.queue != 0 {
		wlClient.destroyQueue(s.queue)
		s.queue = 0
	}
}

func waylandApplicationName(id string) string {
	if name, ok := wlFrontmost.names[id]; ok {
		return name
	}
	name := id
	if id != "" && !strings.ContainsAny(id, "/\\") {
		file := id
		if !strings.HasSuffix(file, ".desktop") {
			file += ".desktop"
		}
		if info := wlFrontmost.desktopInfo(cstr(file)); info != 0 {
			if display := goString(wlFrontmost.displayName(info)); display != "" {
				name = display
			}
			gObjectUnref(info)
		}
	}
	wlFrontmost.names[id] = name
	return name
}
