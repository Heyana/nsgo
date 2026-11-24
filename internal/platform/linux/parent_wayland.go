//go:build linux && !android

package linux

import (
	"sync"
	"time"

	"github.com/ebitengine/purego"
)

// waylandParentTimeout bounds the wait for a compositor that never answers an export, after which a request goes unparented.
const waylandParentTimeout = time.Second

// gdkWaylandExportHandle is gdk_wayland_toplevel_export_handle, which asks the compositor's xdg-foreign exporter for a handle other processes, such as portals and Sushi, parent their windows by.
var (
	gdkWaylandExportHandle func(toplevel, callback, data, destroy uintptr) int32
	bindWaylandExport      = sync.OnceValue(func() bool {
		if !bindWaylandClient() {
			return false
		}
		symbol, err := purego.Dlsym(gtkLib, "gdk_wayland_toplevel_export_handle")
		if err != nil {
			return false
		}
		purego.RegisterFunc(&gdkWaylandExportHandle, symbol)
		return true
	})
)

// waylandToplevel is the GdkToplevel of the window's surface on a Wayland display, 0 elsewhere or before it is realized.
func (w *window) waylandToplevel() uintptr {
	if w.win == 0 && w.surface == 0 {
		return 0
	}
	display := gdkDisplayGetDefault()
	if display == 0 || !bindWaylandExport() || gTypeIsA(display, wlClient.getDisplayType()) == 0 {
		return 0
	}
	toplevel := w.toplevel()
	if toplevel == 0 {
		return 0
	}
	return gtkNativeGetSurface(toplevel)
}

// waylandParent is the exported handle of the window's toplevel, "wayland:" and the compositor's handle, once an export has finished for its current surface.
func (w *window) waylandParent() string {
	if surface := w.waylandToplevel(); surface != 0 && surface == w.wlParentSurface {
		return w.wlParent
	}
	return ""
}

// withPortalParent runs fn on GTK's thread with the window's portal parent: at once on X11, or when it has no surface to export, and on Wayland once the compositor has exported the toplevel, which GTK keeps for the surface's life, so later requests have it at once.
func (w *window) withPortalParent(fn func(parent string)) {
	if parent := w.portalParentWindow(); parent != "" {
		fn(parent)
		return
	}
	surface := w.waylandToplevel()
	if surface == 0 {
		fn("")
		return
	}
	w.wlParentWaiters = append(w.wlParentWaiters, fn)
	if len(w.wlParentWaiters) > 1 {
		return
	}
	if gdkWaylandExportHandle(surface, cbWaylandExported, w.handle, 0) == 0 {
		w.finishWaylandParent("")
		return
	}
	time.AfterFunc(waylandParentTimeout, func() { w.post(func() { w.finishWaylandParent("") }) })
}

// finishWaylandParent answers the requests waiting for the export, once.
func (w *window) finishWaylandParent(parent string) {
	waiters := w.wlParentWaiters
	w.wlParentWaiters = nil
	for _, fn := range waiters {
		fn(parent)
	}
}

func waylandExportedCallback(toplevel, handle, data uintptr) {
	w := windowOf(data)
	if w == nil || handle == 0 {
		return
	}
	w.wlParent, w.wlParentSurface = "wayland:"+goString(handle), toplevel
	w.finishWaylandParent(w.wlParent)
}
