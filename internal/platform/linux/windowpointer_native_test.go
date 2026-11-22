//go:build linux && !android

package linux

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

type windowPointerCounts struct{ press, wheel, enter, context atomic.Int32 }

type nativeWindowPointer struct {
	position func(*window) driver.Point
	move     func(float64, float64)
	button   func(uint32, bool)
	wheel    func()
	raise    func(*window)
}

// TestWindowPointerModesRouteNativeEvents overlays two actual 600x400 windows
// and injects through the display server, never through NSGo's hit test. X11
// needs XTest and a window manager; Wayland opts into a private Sway with
// NSGO_TEST_SWAY_POINTER naming its swaymsg executable.
func TestWindowPointerModesRouteNativeEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("opens windows and injects native pointer input")
	}
	const lowerTitle, upperTitle = "nsgo-pointer1380-lower", "nsgo-pointer1380-upper"
	var lower, upper windowPointerCounts
	ready := make(chan *core.Scope, 1)
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		cx.OpenWindow(core.WindowOptions{Title: lowerTitle, Width: 600, Height: 400, NoShadow: true}, func(*core.Scope) core.Node {
			return core.View{Style: core.Style{FlexGrow: 1, Background: core.RGB(0x44aa44)},
				OnPress: func(core.PressEvent) { lower.press.Add(1) }, OnWheel: func(core.WheelEvent) { lower.wheel.Add(1) },
				OnMouseEnter: func(core.MouseEvent) { lower.enter.Add(1) }}
		})
		cx.OpenWindow(core.WindowOptions{Title: upperTitle, Width: 600, Height: 400, NoShadow: true}, func(cx *core.Scope) core.Node {
			ready <- cx
			return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
				core.View{Style: abs(100, 100, 200, 200, core.RGB(0x2244aa)),
					OnPress: func(core.PressEvent) { upper.press.Add(1) }, OnWheel: func(core.WheelEvent) { upper.wheel.Add(1) },
					OnMouseEnter: func(core.MouseEvent) { upper.enter.Add(1) }, OnContextMenu: func(core.ContextMenuEvent) { upper.context.Add(1) },
					Children: []core.Node{core.View{Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn},
						Style: abs(0, 0, 200, 40, 0), OnPress: func(core.PressEvent) { upper.press.Add(1) }, Children: []core.Node{
							core.View{Style: abs(0, 0, 50, 40, 0), OnPress: func(core.PressEvent) { upper.press.Add(1) }},
						}}}},
			}}
		})
	})
	host := startTestWindow(t, app, Options{Title: "nsgo-pointer1380-host", Width: 600, Height: 400, Hidden: true})
	defer host.onLoop(t, host.close)
	cx := <-ready
	var bottom, top *window
	pointerWait(t, "both native windows", func() bool {
		host.onLoop(t, func() {
			windowHandles.Range(func(_, value any) bool {
				w := value.(*window)
				if w.win != 0 && !w.closing {
					switch goString(gtkWindowGetTitle(w.win)) {
					case lowerTitle:
						bottom = w
					case upperTitle:
						top = w
					}
				}
				return true
			})
		})
		var mapped bool
		if bottom != nil && top != nil {
			host.onLoop(t, func() {
				mapped = bottom.root != 0 && top.root != 0 && gtkWidgetGetWidth(bottom.surface) == 600 &&
					gtkWidgetGetWidth(top.surface) == 600 && gtkWidgetGetHeight(top.surface) == 400
			})
		}
		return mapped
	})
	mouse := windowPointerNativeInput(t, host, bottom, top)
	var nativeBottom, nativeTop uintptr
	host.onLoop(t, func() { nativeBottom, nativeTop = bottom.win, top.win })
	setMode := func(mode core.WindowPointerEvents) {
		t.Helper()
		cx.Post(func() { cx.SetWindowPointerEvents(mode) })
		mouse.raise(top)
		var detail string
		pointerWait(t, "the native pointer mode to be applied", func() bool {
			var applied bool
			host.onLoop(t, func() {
				detail = fmt.Sprintf("want=%v mode=%v set=%v stale=%v mapped=%v rects=%v", mode, top.input.mode, top.input.set, top.input.stale, gtkWidgetGetMapped(top.surface), top.input.rects)
				applied = top.input.mode == mode && (mode == core.WindowPointerAuto && !top.input.set ||
					mode != core.WindowPointerAuto && top.input.set && !top.input.stale)
			})
			return applied
		}, func() string { return detail })
	}
	moveTo := func(x, y float64) {
		t.Helper()
		// Leave both windows first so every route observes a native enter.
		mouse.move(780, 580)
		at := mouse.position(top)
		mouse.move(float64(at.X)+x, float64(at.Y)+y)
	}
	click := func(x, y float64, count *atomic.Int32, other *atomic.Int32) {
		t.Helper()
		before, otherBefore := count.Load(), other.Load()
		mouse.raise(top)
		moveTo(x, y)
		mouse.button(1, true)
		mouse.button(1, false)
		pointerWait(t, "the native press to reach its window", func() bool { return count.Load() > before })
		if other.Load() != otherBefore {
			t.Error("the native press also reached the other window")
		}
	}
	setMode(core.WindowPointerNone)
	click(200, 220, &lower.press, &upper.press)
	entered, scrolled := lower.enter.Load(), lower.wheel.Load()
	moveTo(500, 350)
	mouse.wheel()
	pointerWait(t, "native hover and wheel through a window taking no pointer", func() bool {
		return lower.enter.Load() > entered && lower.wheel.Load() > scrolled
	}, func() string {
		return fmt.Sprintf("enter=%d want>%d wheel=%d want>%d", lower.enter.Load(), entered, lower.wheel.Load(), scrolled)
	})
	setMode(core.WindowPointerPainted)
	click(200, 220, &upper.press, &lower.press)
	entered, scrolled = upper.enter.Load(), upper.wheel.Load()
	moveTo(200, 220)
	mouse.wheel()
	pointerWait(t, "native hover and wheel on the painted island", func() bool {
		return upper.enter.Load() > entered && upper.wheel.Load() > scrolled
	})
	click(500, 350, &lower.press, &upper.press)
	entered, scrolled = lower.enter.Load(), lower.wheel.Load()
	mouse.raise(top)
	moveTo(500, 350)
	mouse.wheel()
	pointerWait(t, "native hover and wheel beside the painted island", func() bool {
		return lower.enter.Load() > entered && lower.wheel.Load() > scrolled
	}, func() string {
		return fmt.Sprintf("enter=%d want>%d wheel=%d want>%d", lower.enter.Load(), entered, lower.wheel.Load(), scrolled)
	})
	mouse.raise(top)
	moveTo(200, 220)
	context := upper.context.Load()
	mouse.button(3, true)
	mouse.button(3, false)
	pointerWait(t, "the painted island's context menu handler", func() bool { return upper.context.Load() > context })
	// A control inside the drag region still takes its own native press.
	beforeControl := mouse.position(top)
	click(120, 120, &upper.press, &lower.press)
	if mouse.position(top) != beforeControl {
		t.Error("a nearer control dragged its window")
	}
	// Its own press handler does not override an explicit WindowDragOn.
	// A real window-manager drag starts in the island's dedicated drag strip.
	mouse.raise(top)
	before := mouse.position(top)
	moveTo(200, 120)
	mouse.button(1, true)
	time.Sleep(20 * time.Millisecond)
	mouse.move(float64(before.X)+220, float64(before.Y)+140)
	time.Sleep(30 * time.Millisecond)
	// The first motion crosses GTK's threshold and hands the gesture to
	// the window manager; the next motion moves its native grab.
	mouse.move(float64(before.X)+240, float64(before.Y)+160)
	time.Sleep(30 * time.Millisecond)
	mouse.button(1, false)
	pointerWait(t, "the native window drag to move the painted island", func() bool { return mouse.position(top) != before })
	setMode(core.WindowPointerAuto)
	// Auto restores input even on the formerly empty part of the window.
	pointerWait(t, "Auto to restore the whole native input region", func() bool {
		moveTo(500, 350)
		var inside bool
		host.onLoop(t, func() {
			inside = top.pointerIn && top.lastPointer.X >= 499 && top.lastPointer.X <= 501 && top.lastPointer.Y >= 349 && top.lastPointer.Y <= 351
		})
		return inside
	}, func() string {
		var detail string
		host.onLoop(t, func() {
			detail = fmt.Sprintf("inside=%v at=%v mode=%v set=%v", top.pointerIn, top.lastPointer, top.input.mode, top.input.set)
		})
		return detail
	})
	click(200, 220, &upper.press, &lower.press)
	// No native window is replaced while modes change.
	host.onLoop(t, func() {
		if top.win != nativeTop || bottom.win != nativeBottom || windowOf(top.handle) != top || windowOf(bottom.handle) != bottom {
			t.Error("changing pointer modes replaced or closed a window")
		}
	})
}

func pointerWait(t *testing.T, what string, predicate func() bool, details ...func() string) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if predicate() {
			return
		}
		if time.Now().After(deadline) {
			if len(details) > 0 {
				t.Fatal("timed out waiting for ", what, ": ", details[0]())
			}
			t.Fatal("timed out waiting for ", what)
		}
	}
}

func windowPointerNativeInput(t *testing.T, host, bottom, top *window) nativeWindowPointer {
	t.Helper()
	if sway := os.Getenv("NSGO_TEST_SWAY_POINTER"); sway != "" {
		return windowPointerSwayInput(t, host, sway)
	}
	var x11 bool
	host.onLoop(t, func() { x11 = drawsOnX11() })
	if !x11 {
		t.Skip("Wayland native input requires NSGO_TEST_SWAY_POINTER in a private Sway instance")
	}
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Skip(err)
	}
	xtst, err := purego.Dlopen("libXtst.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Skip(err)
	}
	var open func(*byte) uintptr
	var closeDisplay func(uintptr) int32
	var flush func(uintptr, int32) int32
	var translate func(uintptr, uintptr, uintptr, int32, int32, *int32, *int32, *uintptr) int32
	var motion func(uintptr, int32, int32, int32, uintptr) int32
	var button func(uintptr, uint32, int32, uintptr) int32
	purego.RegisterLibFunc(&open, lib, "XOpenDisplay")
	purego.RegisterLibFunc(&closeDisplay, lib, "XCloseDisplay")
	purego.RegisterLibFunc(&flush, lib, "XSync")
	purego.RegisterLibFunc(&translate, lib, "XTranslateCoordinates")
	purego.RegisterLibFunc(&motion, xtst, "XTestFakeMotionEvent")
	purego.RegisterLibFunc(&button, xtst, "XTestFakeButtonEvent")
	display := open(nil)
	if display == 0 {
		t.Skip("cannot open X display")
	}
	t.Cleanup(func() { closeDisplay(display) })
	if !bindDesktopXlib() {
		t.Skip("cannot inspect the X11 window manager")
	}
	x := &desktopXlib
	var typ uintptr
	var format int32
	var n, after uint
	var data *byte
	check := x.internAtom(display, cstr("_NET_SUPPORTING_WM_CHECK"), 0)
	x.getProperty(display, x.rootWindow(display), check, 0, 1, 0, desktopXAWindow, &typ, &format, &n, &after, &data)
	if data != nil {
		x.free(data)
	}
	if n == 0 {
		t.Skip("native window dragging needs an X11 window manager")
	}
	var scale float64
	host.onLoop(t, func() {
		for _, w := range []*window{bottom, top} {
			gtkWindowSetDecorated(w.win, 0)
			d, xid, _ := w.x11Window()
			x11MoveWindow(d, xid, 50, 50)
			x11Flush(d)
		}
		top.setLevel(driver.WindowLevelFloating)
		scale = float64(max(gtkWidgetGetScaleFactor(top.win), 1))
	})
	press := func(b uint32, down bool) { button(display, b, boolInt(down), 0); flush(display, 0) }
	return nativeWindowPointer{
		position: func(w *window) driver.Point {
			var xid uintptr
			host.onLoop(t, func() { xid = gdkX11SurfaceGetXID(gtkNativeGetSurface(w.win)) })
			var x, y int32
			var child uintptr
			translate(display, xid, x11LevelRoot(display), 0, 0, &x, &y, &child)
			return driver.Point{X: driver.Points(float64(x) / scale), Y: driver.Points(float64(y) / scale)}
		},
		move:   func(x, y float64) { motion(display, -1, int32(x*scale), int32(y*scale), 0); flush(display, 0) },
		button: press,
		wheel:  func() { press(5, true); press(5, false) },
		raise:  func(w *window) { host.onLoop(t, func() { gtkWindowPresent(w.win) }) },
	}
}

func windowPointerSwayInput(t *testing.T, host *window, sway string) nativeWindowPointer {
	t.Helper()
	input := keepWindowPointerDevice(t, host)
	call := func(args ...string) []byte {
		t.Helper()
		out, err := exec.Command(sway, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("private Sway input: %v: %s", err, out)
		}
		return out
	}
	type node struct {
		Name     string
		Rect     struct{ X, Y, Width, Height int }
		Nodes    []json.RawMessage
		Floating []json.RawMessage `json:"floating_nodes"`
	}
	position := func(w *window) driver.Point {
		t.Helper()
		var found *node
		var visit func(json.RawMessage)
		visit = func(raw json.RawMessage) {
			var n node
			if err := json.Unmarshal(raw, &n); err != nil {
				t.Fatal(err)
			}
			if n.Name == w.opts.Title {
				found = &n
			}
			for _, c := range append(n.Nodes, n.Floating...) {
				visit(c)
			}
		}
		visit(call("-r", "-t", "get_tree"))
		if found == nil {
			t.Fatalf("no private Sway window %q", w.opts.Title)
		}
		return driver.Point{X: driver.Points(found.Rect.X), Y: driver.Points(found.Rect.Y)}
	}
	var tree node
	if err := json.Unmarshal(call("-r", "-t", "get_tree"), &tree); err != nil {
		t.Fatal(err)
	}
	if tree.Rect.Width <= 0 || tree.Rect.Height <= 0 {
		t.Fatal("the private compositor has no output extent")
	}
	move := input.move
	input.position = position
	input.move = func(x, y float64) {
		// The virtual pointer uses a normalized 0..65535 range; scale the
		// compositor's actual output extent, rather than a window's size.
		move((x-float64(tree.Rect.X))*65535/float64(tree.Rect.Width), (y-float64(tree.Rect.Y))*65535/float64(tree.Rect.Height))
	}
	input.raise = func(w *window) { call("-r", "[title="+strconv.Quote(w.opts.Title)+"] focus") }
	return input
}

// keepWindowPointerDevice gives the isolated headless compositor a persistent
// virtual pointer. Without an input device, wl_seat advertises no pointer to
// GTK even when Sway's native cursor commands can move its cursor.
func keepWindowPointerDevice(t *testing.T, host *window) nativeWindowPointer {
	t.Helper()
	if !bindWaylandPointer() {
		t.Fatal("Wayland pointer bindings are unavailable")
	}
	lib, err := purego.Dlopen("libwayland-client.so.0", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Fatal(err)
	}
	var connect func(*byte) uintptr
	var disconnect func(uintptr)
	purego.RegisterLibFunc(&connect, lib, "wl_display_connect")
	purego.RegisterLibFunc(&disconnect, lib, "wl_display_disconnect")
	display := connect(nil)
	if display == 0 {
		t.Fatal("cannot connect to the private Wayland display")
	}
	var queue, registry, manager, device uintptr
	var pin runtime.Pinner
	t.Cleanup(func() {
		if device != 0 {
			wlPointer.marshal(device, 8, nil, 1, 1, nil)
		}
		if manager != 0 {
			wlPointer.marshal(manager, 1, nil, 1, 1, nil)
		}
		if queue != 0 {
			wlPointer.roundtrip(display, queue)
		}
		if registry != 0 {
			wlPointer.destroy(registry)
		}
		if queue != 0 {
			wlPointer.destroyQueue(queue)
		}
		disconnect(display)
		pin.Unpin()
	})
	queue = wlPointer.createQueue(display)
	if queue == 0 {
		t.Fatal("cannot create the private Wayland queue")
	}
	registry = wlPointerRequest(display, 1, wlPointer.registry, 0)
	if registry == 0 {
		t.Fatal("cannot create the private Wayland registry")
	}
	wlPointer.setQueue(registry, queue)
	address, err := purego.Dlsym(lib, "wl_seat_interface")
	if err != nil {
		t.Fatal(err)
	}
	seat := (*wlInterface)(cPointer(address))
	pointer := newWLPointerInterface("zwlr_virtual_pointer_v1", []wlPointerMessage{
		newWLPointerMessage("motion", "uff"), newWLPointerMessage("motion_absolute", "uuuuu"),
		newWLPointerMessage("button", "uuu"), newWLPointerMessage("axis", "uuf"), newWLPointerMessage("frame", ""),
		newWLPointerMessage("axis_source", "u"), newWLPointerMessage("axis_stop", "uu"),
		newWLPointerMessage("axis_discrete", "uufi"), newWLPointerMessage("destroy", ""),
	}, nil)
	managerInterface := newWLPointerInterface("zwlr_virtual_pointer_manager_v1", []wlPointerMessage{
		newWLPointerMessage("create_virtual_pointer", "?on", seat, pointer), newWLPointerMessage("destroy", ""),
	}, nil)
	listeners := [2]uintptr{
		newCallback(func(_ uintptr, r uintptr, name uint32, iface *byte, _ uint32) {
			if goString(uintptr(unsafe.Pointer(iface))) == "zwlr_virtual_pointer_manager_v1" {
				manager = wlPointerRequest(r, 0, managerInterface, uintptr(name), uintptr(unsafe.Pointer(managerInterface.name)), 1, 0)
			}
		}), newCallback(func(uintptr, uintptr, uint32) {}),
	}
	pin.Pin(&listeners[0])
	wlPointer.addListener(registry, &listeners[0], 0)
	if wlPointer.roundtrip(display, queue) < 0 || manager == 0 {
		t.Fatal("the private compositor has no virtual pointer protocol")
	}
	device = wlPointerRequest(manager, 0, pointer, 0, 0)
	if device == 0 || wlPointer.roundtrip(display, queue) < 0 {
		t.Fatal("the private pointer device did not initialize")
	}
	pointerWait(t, "GTK to discover the virtual pointer", func() bool {
		var ready bool
		host.onLoop(t, func() { ready = wlPointer.seatPointer(wlPointer.getSeat(gdkDisplayGetDefault())) != 0 })
		return ready
	})
	start := time.Now()
	stamp := func() uintptr { return uintptr(time.Since(start).Milliseconds()) }
	frame := func() {
		wlPointerRequest(device, 4, nil)
		wlPointer.roundtrip(display, queue)
	}
	return nativeWindowPointer{
		move: func(x, y float64) {
			wlPointerRequest(device, 1, nil, stamp(), uintptr(x), uintptr(y), 65535, 65535)
			frame()
		},
		button: func(button uint32, down bool) {
			code := map[uint32]uintptr{1: 0x110, 2: 0x112, 3: 0x111}[button]
			wlPointerRequest(device, 2, nil, stamp(), code, uintptr(boolInt(down)))
			frame()
		},
		wheel: func() {
			// Axis source wheel, one vertical discrete step, then a frame.
			wlPointerRequest(device, 5, nil, 0)
			wlPointerRequest(device, 7, nil, stamp(), 0, 15*256, 1)
			frame()
		},
	}
}
