//go:build linux && !android

package linux

import (
	"math"
	"sync"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// x11FrameState keeps a position requested while a window is hidden until its
// map: a window manager may choose its own position when it first manages it.
// Later maps keep the user's position unless the app asks for another one.
type x11FrameState struct {
	pending *driver.Point
	mapped  bool
	refresh bool
	// resize keeps the visual corner through GTK's next native resize.
	resize *frameResize
}

type frameResize struct {
	position driver.Point
	size     driver.Size
}

var x11Frames = struct {
	translate func(display, from, to uintptr, x, y int32, intoX, intoY *int32, child *uintptr) int32
	property  func(display, window, property uintptr, offset, length int, delete int32, requested uintptr, actual *uintptr, format *int32, count, after *uint, data *uintptr) int32
	free      func(uintptr) int32
	mapped    uintptr
}{}

var bindX11Frames = sync.OnceValue(func() bool {
	if bindX11Panels(); !x11PanelsReady {
		return false
	}
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return false
	}
	purego.RegisterLibFunc(&x11Frames.translate, lib, "XTranslateCoordinates")
	purego.RegisterLibFunc(&x11Frames.property, lib, "XGetWindowProperty")
	purego.RegisterLibFunc(&x11Frames.free, lib, "XFree")
	x11Frames.mapped = newCallback(frameMappedCallback)
	return true
})

var frameWatch sync.Once

func watchX11Frames() {
	frameWatch.Do(func() {
		gSignalConnect(gdkDisplayGetDefault(), "xevent", newCallback(frameChangedCallback), 0)
	})
}

// canPlace reports whether the app can position this surface's own window.
// X11 manages a regular window and positions an unmanaged panel directly;
// an embedded view, the app's hidden surface and a tray have no window to move.
func (w *window) canPlace() bool {
	return w.win != 0 && !w.embedded && !(w.primary && w.opts.Hidden) &&
		w.kind != driver.SurfaceTrayIcon && drawsOnX11() && bindX11Frames()
}

// framePosition reads the window's visual top left in the same logical screen
// points as Env.Displays. XTranslateCoordinates accounts for reparenting by a
// window manager. Server decorations extend beyond the client, while GTK's
// invisible CSD shadows are inside it and do not belong to the visible frame.
func (w *window) framePosition() (driver.Point, bool) {
	if !w.canPlace() {
		return driver.Point{}, false
	}
	surface := gtkNativeGetSurface(w.win)
	if surface == 0 {
		return driver.Point{}, false
	}
	watchX11Frames()
	display := gdkX11PanelXDisplay(gdkDisplayGetDefault())
	xid := gdkX11SurfaceGetXID(surface)
	var x, y int32
	var child uintptr
	if x11Frames.translate(display, xid, x11LevelRoot(display), 0, 0, &x, &y, &child) == 0 {
		return driver.Point{}, false
	}
	gtk := x11FrameExtents(display, xid, "_GTK_FRAME_EXTENTS")
	wm := x11FrameExtents(display, xid, "_NET_FRAME_EXTENTS")
	scale := driver.Points(max(gtkWidgetGetScaleFactor(w.win), 1))
	return driver.Point{X: driver.Points(int64(x)+int64(gtk[0])-int64(wm[0])) / scale,
		Y: driver.Points(int64(y)+int64(gtk[2])-int64(wm[2])) / scale}, true
}

// x11FrameExtents reads EWMH's four CARDINAL extents: left, right, top, bottom.
// Xlib expands each 32-bit property item into a native unsigned long.
func x11FrameExtents(display, xid uintptr, name string) [4]uint32 {
	var actual, data uintptr
	var format int32
	var count, after uint
	atom := x11InternAtom(display, cstr(name), 0)
	status := x11Frames.property(display, xid, atom, 0, 4, 0, x11AtomCardinal, &actual, &format, &count, &after, &data)
	if data != 0 {
		defer x11Frames.free(data)
	}
	if status != 0 || actual != x11AtomCardinal || format != 32 || count != 4 || data == 0 {
		return [4]uint32{}
	}
	values := (*[4]uintptr)(cPointer(data))
	return [4]uint32{uint32(values[0]), uint32(values[1]), uint32(values[2]), uint32(values[3])}
}

// moveFrame places a window without activating it. A mapped managed window
// uses EWMH's explicit NorthWestGravity, so x and y describe its outer corner
// regardless of the client's gravity. With no EWMH window manager, or for an
// unmanaged panel, XMoveWindow directly positions the native window.
func (w *window) moveFrame(position driver.Point) {
	display, xid, ok := w.x11Window()
	if !ok {
		return
	}
	gtk := x11FrameExtents(display, xid, "_GTK_FRAME_EXTENTS")
	scale := float64(max(gtkWidgetGetScaleFactor(w.win), 1))
	x := int32(math.Round(float64(position.X)*scale)) - int32(gtk[0])
	y := int32(math.Round(float64(position.Y)*scale)) - int32(gtk[2])
	if screen := desktopScreen(); !w.unmanaged && screen != 0 &&
		gdkX11ScreenSupportsNetWMHint(screen, cstr("_NET_MOVERESIZE_WINDOW")) != 0 {
		const northWestGravity = 1
		const positionFlags = 1<<8 | 1<<9
		ev := x11ClientMessage{typ: x11ClientMessageType, sendEvent: 1, display: display, window: xid,
			messageType: x11InternAtom(display, cstr("_NET_MOVERESIZE_WINDOW"), 0), format: 32,
			data: [5]int{northWestGravity | positionFlags | netWMSourceApplication<<12, int(x), int(y)}}
		x11SendEvent(display, x11LevelRoot(display), 0, x11SubstructureRedirectMask|x11SubstructureNotifyMask, &ev)
	} else {
		x11MoveWindow(display, xid, x, y)
	}
	x11Flush(display)
}

func (w *window) setFramePosition(position driver.Point) {
	if !w.canPlace() {
		return
	}
	if !w.frameState.mapped {
		w.frameState.mapped = true
		gSignalConnect(w.win, "map", x11Frames.mapped, w.handle)
	}
	if gtkWidgetGetMapped(w.win) == 0 {
		w.frameState.pending = &position
	}
	w.moveFrame(position)
}

func frameMappedCallback(widget, data uintptr) {
	if w := windowOf(data); w != nil && !w.closing && w.frameState.pending != nil {
		position := *w.frameState.pending
		w.frameState.pending = nil
		w.moveFrame(position)
	}
}

// x11ConfigureEvent is Xlib's XConfigureEvent, on GTK's thread.
type x11ConfigureEvent struct {
	typ            int32
	serial         uintptr
	sent           int32
	display        uintptr
	event, window  uintptr
	x, y, w, h, bw int32
	above          uintptr
	override       int32
}

// frameChangedCallback refreshes the environment after GDK processes a native
// move or resize. One queued refresh per window coalesces a configure burst.
func frameChangedCallback(display, event, data uintptr) int32 {
	const configureNotify = 22
	ev := (*x11ConfigureEvent)(cPointer(event))
	if ev.typ != configureNotify {
		return 0
	}
	windowHandles.Range(func(_, value any) bool {
		w := value.(*window)
		if w.closing || !w.canPlace() || w.frameState.refresh {
			return true
		}
		surface := gtkNativeGetSurface(w.win)
		if surface == 0 || gdkX11SurfaceGetXID(surface) != ev.window {
			return true
		}
		w.frameState.refresh = true
		w.post(func() {
			w.frameState.refresh = false
			if !w.closing {
				w.updateEnvironment()
			}
		})
		return true
	})
	return 0
}

// finishFrameResize runs after GTK has allocated the resized window. A native
// configure event can precede that allocation, so it must not consume this
// restoration while GTK still holds the old size.
func (w *window) finishFrameResize() {
	if pending := w.frameState.resize; pending != nil &&
		gtkWidgetGetWidth(w.win) == int32(pending.size.Width) && gtkWidgetGetHeight(w.win) == int32(pending.size.Height) {
		w.frameState.resize = nil
		w.moveFrame(pending.position)
	}
}
