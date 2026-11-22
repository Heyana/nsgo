//go:build linux && !android

package linux

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// Each extension uses a private queue on GTK's Wayland connection, so a registry roundtrip cannot dispatch GTK callbacks inside an environment read. These are the public libwayland wire descriptors, built without C stubs.
type wlInterface struct {
	name                 *byte
	version, methodCount int32
	methods              *wlMessage
	eventCount           int32
	events               *wlMessage
}

type wlMessage struct {
	name, signature *byte
	types           **wlInterface
}

var wlClient struct {
	lib                 uintptr
	getDisplayType      func() uintptr
	getDisplay          func(uintptr) uintptr
	createQueue         func(uintptr) uintptr
	destroyQueue        func(uintptr)
	setQueue            func(uintptr, uintptr)
	roundtrip, dispatch func(uintptr, uintptr) int32
	flush               func(uintptr) int32
	destroy             func(uintptr)
	addListener         func(uintptr, *uintptr, uintptr) int32
	marshal             func(uintptr, uint32, *wlInterface, uint32, uint32, *uintptr) uintptr
	registry            *wlInterface
	pin                 runtime.Pinner
}

var bindWaylandClient = sync.OnceValue(func() bool {
	lib, err := purego.Dlopen("libwayland-client.so.0", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		return false
	}
	wlClient.lib = lib
	for name, fn := range map[string]any{
		"gdk_wayland_display_get_type":       &wlClient.getDisplayType,
		"gdk_wayland_display_get_wl_display": &wlClient.getDisplay,
	} {
		symbol, err := purego.Dlsym(gtkLib, name)
		if err != nil {
			return false
		}
		purego.RegisterFunc(fn, symbol)
	}
	for name, fn := range map[string]any{
		"wl_display_create_queue":           &wlClient.createQueue,
		"wl_event_queue_destroy":            &wlClient.destroyQueue,
		"wl_proxy_set_queue":                &wlClient.setQueue,
		"wl_display_roundtrip_queue":        &wlClient.roundtrip,
		"wl_display_dispatch_queue_pending": &wlClient.dispatch,
		"wl_display_flush":                  &wlClient.flush,
		"wl_proxy_destroy":                  &wlClient.destroy,
		"wl_proxy_add_listener":             &wlClient.addListener,
		"wl_proxy_marshal_array_flags":      &wlClient.marshal,
	} {
		symbol, err := purego.Dlsym(lib, name)
		if err != nil {
			return false
		}
		purego.RegisterFunc(fn, symbol)
	}
	address, err := purego.Dlsym(lib, "wl_registry_interface")
	if err != nil {
		return false
	}
	wlClient.registry = (*wlInterface)(cPointer(address))
	return true
})

func pinnedWLString(s string) *byte {
	p := cstr(s)
	wlClient.pin.Pin(p)
	return p
}

func newWLMessage(name, signature string, types ...*wlInterface) wlMessage {
	m := wlMessage{name: pinnedWLString(name), signature: pinnedWLString(signature), types: unsafe.SliceData(types)}
	if m.types != nil {
		wlClient.pin.Pin(m.types)
	}
	return m
}

func newWLInterface(name string, version int32, methods, events []wlMessage) *wlInterface {
	i := &wlInterface{name: pinnedWLString(name), version: version, methodCount: int32(len(methods)), methods: unsafe.SliceData(methods), eventCount: int32(len(events)), events: unsafe.SliceData(events)}
	wlClient.pin.Pin(i)
	if i.methods != nil {
		wlClient.pin.Pin(i.methods)
	}
	if i.events != nil {
		wlClient.pin.Pin(i.events)
	}
	return i
}
