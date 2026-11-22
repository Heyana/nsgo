//go:build linux && !android

package linux

import (
	"sync"

	"github.com/ebitengine/purego"
)

// gtk4-layer-shell interposes libwayland calls. Load it before GTK loads libwayland, as its linking.md requires; an embedding host that already loaded libwayland without the shim cannot enable this optional feature.
type layerShellAPI struct {
	supported        func() int32
	initWindow       func(uintptr)
	isLayerWindow    func(uintptr) int32
	setNamespace     func(uintptr, *byte)
	setLayer         func(uintptr, int32)
	setMonitor       func(uintptr, uintptr)
	setAnchor        func(uintptr, int32, int32)
	setMargin        func(uintptr, int32, int32)
	setExclusiveZone func(uintptr, int32)
	setKeyboardMode  func(uintptr, int32)
	setRespectClose  func(uintptr, int32)
}

type layerShellLoad struct {
	api   *layerShellAPI
	issue string
}

var loadLayerShell = sync.OnceValue(func() layerShellLoad {
	// RTLD_NOLOAD is 4 in Linux's dynamic loader. Unlike a plain dlopen, this only observes whether the library is already loaded.
	const noLoad = 4
	var lib uintptr
	for _, name := range []string{"libgtk4-layer-shell.so.0", "libgtk4-layer-shell.so"} {
		if h, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL|noLoad); err == nil {
			lib = h
			break
		}
	}
	if lib == 0 {
		if wayland, err := purego.Dlopen("libwayland-client.so.0", purego.RTLD_NOW|noLoad); err == nil {
			purego.Dlclose(wayland)
			return layerShellLoad{issue: "gtk4-layer-shell must be loaded before the embedding host loads Wayland"}
		}
		for _, name := range []string{"libgtk4-layer-shell.so.0", "libgtk4-layer-shell.so"} {
			if h, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
				lib = h
				break
			}
		}
	}
	if lib == 0 {
		return layerShellLoad{issue: "Wayland panel docking needs the optional gtk4-layer-shell library"}
	}
	shim, shimErr := purego.Dlsym(lib, "wl_proxy_marshal_flags")
	global, globalErr := purego.Dlsym(0, "wl_proxy_marshal_flags")
	if shimErr != nil || globalErr != nil || shim != global {
		return layerShellLoad{issue: "gtk4-layer-shell was loaded after Wayland; its required symbol interposition is unavailable"}
	}
	api := new(layerShellAPI)
	for _, f := range []struct {
		fn   any
		name string
	}{
		{&api.supported, "gtk_layer_is_supported"}, {&api.initWindow, "gtk_layer_init_for_window"},
		{&api.isLayerWindow, "gtk_layer_is_layer_window"}, {&api.setNamespace, "gtk_layer_set_namespace"},
		{&api.setLayer, "gtk_layer_set_layer"}, {&api.setMonitor, "gtk_layer_set_monitor"},
		{&api.setAnchor, "gtk_layer_set_anchor"}, {&api.setMargin, "gtk_layer_set_margin"},
		{&api.setExclusiveZone, "gtk_layer_set_exclusive_zone"}, {&api.setKeyboardMode, "gtk_layer_set_keyboard_mode"},
	} {
		if _, err := purego.Dlsym(lib, f.name); err != nil {
			return layerShellLoad{issue: "gtk4-layer-shell is missing " + f.name}
		}
		purego.RegisterLibFunc(f.fn, lib, f.name)
	}
	if _, err := purego.Dlsym(lib, "gtk_layer_set_respect_close"); err == nil {
		purego.RegisterLibFunc(&api.setRespectClose, lib, "gtk_layer_set_respect_close")
	}
	return layerShellLoad{api: api}
})

func (w *window) canDockPanels() bool {
	if w.embedded || gdkDisplayGetDefault() == 0 {
		return false
	}
	if drawsOnX11() {
		return true
	}
	loaded := loadLayerShell()
	return loaded.api != nil && loaded.api.supported() != 0
}
