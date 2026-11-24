//go:build linux && !android

package linux

import (
	"testing"
	"unsafe"
)

// The compositor's export answers every request that waited for it and stays for the surface's later ones; the timeout that follows changes nothing.
func TestWaylandExportAnswersWaitersOnce(t *testing.T) {
	w := &window{}
	w.handle = registerWindow(w)
	defer windowHandles.Delete(w.handle)
	var got []string
	for range 2 {
		w.wlParentWaiters = append(w.wlParentWaiters, func(parent string) { got = append(got, parent) })
	}
	const surface = 0x1234
	handle := cstr("abc-123")
	waylandExportedCallback(surface, uintptr(unsafe.Pointer(handle)), w.handle)
	w.finishWaylandParent("")
	if len(got) != 2 || got[0] != "wayland:abc-123" || got[1] != "wayland:abc-123" {
		t.Fatalf("waiters got %q, want the exported handle twice", got)
	}
	if w.wlParent != "wayland:abc-123" || w.wlParentSurface != surface || w.wlParentWaiters != nil {
		t.Fatalf("cached %q for %#x with %d waiting", w.wlParent, w.wlParentSurface, len(w.wlParentWaiters))
	}
}

// A window gone before the compositor answers, or an export with no handle, answers nothing.
func TestWaylandExportIgnoresGoneWindows(t *testing.T) {
	w := &window{}
	w.handle = registerWindow(w)
	called := false
	w.wlParentWaiters = []func(string){func(string) { called = true }}
	waylandExportedCallback(1, 0, w.handle)
	windowHandles.Delete(w.handle)
	waylandExportedCallback(1, uintptr(unsafe.Pointer(cstr("late"))), w.handle)
	if called || w.wlParent != "" {
		t.Fatalf("called %v, cached %q", called, w.wlParent)
	}
}
