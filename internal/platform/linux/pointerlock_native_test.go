//go:build linux && !android

package linux

import (
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
)

func TestX11PointerLock(t *testing.T) {
	var ref core.Ref
	ready := make(chan *core.Scope, 1)
	moves := make(chan [2]float64, 8)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		ready <- cx
		return core.View{Ref: &ref, OnPointerDelta: func(dx, dy float64) {
			moves <- [2]float64{dx, dy}
		}}
	}, Options{Width: 240, Height: 180})
	cx := <-ready
	var display, top uintptr
	var server *xPointerServer
	var fake func(uintptr, int32, int32, uintptr) int32
	w.onLoop(t, func() {
		server = xPointerDisplay()
		if server == nil {
			return
		}
		native := gtkNativeGetSurface(gtkWidgetGetNative(w.surface))
		display, top = server.display, gdkX11SurfaceGetXID(native)
		lib, err := purego.Dlopen("libXtst.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&fake, lib, "XTestFakeRelativeMotionEvent")
		var focus func(uintptr, uintptr, int32, uintptr) int32
		libX, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			return
		}
		purego.RegisterLibFunc(&focus, libX, "XSetInputFocus")
		focus(display, top, 1, 0)
		xPointer.warp(display, 0, top, 0, 0, 0, 0, 50, 50)
		x11Flush(display)
	})
	if server == nil || fake == nil {
		t.Skip("requires XInput2 and XTest")
	}
	wait := func(what string, ready func() bool) {
		t.Helper()
		for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			met := false
			w.onLoop(t, func() { met = ready() })
			if met {
				return
			}
		}
		t.Fatalf("timed out waiting for %s", what)
	}
	wait("window focus", w.canHoldPointer)
	var startX, startY int32
	read := func() (int32, int32) {
		var root, child uintptr
		var x, y, localX, localY int32
		var mask uint32
		xPointer.queryPointer(display, server.root, &root, &child, &x, &y, &localX, &localY, &mask)
		return x, y
	}
	w.onLoop(t, func() { startX, startY = read() })
	cx.Post(func() { ref.LockPointer() })
	wait("the X11 pointer grab", func() bool { return lockedPointer.owner == w })
	w.onLoop(t, func() {
		fake(display, 40, -35, 0)
		x11Flush(display)
	})
	select {
	case delta := <-moves:
		if delta != [2]float64{40, -35} {
			t.Errorf("raw delta = %v, want [40 -35]", delta)
		}
	case <-time.After(4 * time.Second):
		t.Error("the grab produced no relative motion")
	}
	w.onLoop(t, func() {
		if x, y := read(); x != startX || y != startY {
			t.Errorf("locked pointer moved to (%d, %d) from (%d, %d)", x, y, startX, startY)
		}
		keyPressedCallback(0, gdkEscape, 0, 0, w.handle)
		if lockedPointer.owner == w {
			t.Error("Escape did not release the X11 pointer")
		}
		if x, y := read(); x != startX || y != startY {
			t.Errorf("unlocked pointer returned to (%d, %d), want (%d, %d)", x, y, startX, startY)
		}
	})
}

// Run this under a nested Weston compositor with an X11 backend.
func TestWaylandPointerLock(t *testing.T) {
	weston := os.Getenv("NSGO_TEST_WESTON_WINDOW")
	if weston == "" {
		t.Skip("requires the X11 window of a nested Weston compositor")
	}
	var ref core.Ref
	ready := make(chan *core.Scope, 1)
	moved := make(chan [2]float64, 32)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		ready <- cx
		return core.View{Ref: &ref, OnPointerDelta: func(dx, dy float64) {
			select {
			case moved <- [2]float64{dx, dy}:
			default:
			}
		}}
	}, Options{Width: 240, Height: 180})
	cx := <-ready
	xdotool := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("xdotool", args...).CombinedOutput(); err != nil {
			t.Fatalf("xdotool %v: %v: %s", args, err, out)
		}
	}
	wait := func(what string, ready func() bool) {
		t.Helper()
		for deadline := time.Now().Add(4 * time.Second); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
			met := false
			w.onLoop(t, func() { met = ready() })
			if met {
				return
			}
		}
		t.Fatalf("timed out waiting for %s", what)
	}
	w.onLoop(t, func() { gtkWindowFullscreen(w.win) })
	wait("the fullscreen Wayland surface", func() bool { return gtkWidgetGetWidth(w.surface) >= 600 })
	xdotool("windowfocus", weston)
	xdotool("mousemove", "--window", weston, "320", "240")
	xdotool("click", "1")
	wait("the focused Wayland window", w.canHoldPointer)
	if !w.env.CanLockPointer {
		t.Fatal("the compositor did not advertise the pointer extensions")
	}
	cx.Post(func() { ref.LockPointer() })
	wait("the compositor's lock", func() bool {
		p, ok := lockedPointer.capture.(*wlLockedPointer)
		return ok && lockedPointer.owner == w && p.active
	})
	xdotool("mousemove_relative", "--", "35", "-24")
	select {
	case delta := <-moved:
		if delta[0] <= 0 || delta[1] >= 0 {
			t.Errorf("raw delta = %v, want movement right and up", delta)
		}
	case <-time.After(4 * time.Second):
		t.Error("the Wayland lock produced no relative motion")
	}
	xdotool("key", "Escape")
	wait("Escape to release Wayland", func() bool { return lockedPointer.owner != w })
}
