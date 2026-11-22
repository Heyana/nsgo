//go:build linux && !android

package linux

import (
	"fmt"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// frameXClient observes native geometry, focus and stacking through a separate
// X connection. It never substitutes a window manager's replies.
type frameXClient struct {
	display, root uintptr
	translate     func(uintptr, uintptr, uintptr, int32, int32, *int32, *int32, *uintptr) int32
	queryTree     func(uintptr, uintptr, *uintptr, *uintptr, *uintptr, *uint32) int32
	move          func(uintptr, uintptr, int32, int32) int32
	motion        func(uintptr, int32, int32, int32, uintptr) int32
	button        func(uintptr, uint32, int32, uintptr) int32
}

func openFrameXClient(t *testing.T) *frameXClient {
	t.Helper()
	if !bindDesktopXlib() {
		t.Skip("no libX11")
	}
	lib, err := purego.Dlopen("libX11.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Fatal(err)
	}
	c := &frameXClient{display: desktopXlib.openDisplay(nil)}
	if c.display == 0 {
		t.Skip("no X display")
	}
	c.root = desktopXlib.rootWindow(c.display)
	purego.RegisterLibFunc(&c.translate, lib, "XTranslateCoordinates")
	purego.RegisterLibFunc(&c.queryTree, lib, "XQueryTree")
	purego.RegisterLibFunc(&c.move, lib, "XMoveWindow")
	t.Cleanup(func() { desktopXlib.closeDisplay(c.display) })
	return c
}

func (c *frameXClient) property(window uintptr, name string, typ uintptr) []uintptr {
	var actual uintptr
	var format int32
	var count, after uint
	var data *byte
	atom := desktopXlib.internAtom(c.display, cstr(name), 0)
	status := desktopXlib.getProperty(c.display, window, atom, 0, 1024, 0, typ, &actual, &format, &count, &after, &data)
	if data != nil {
		defer desktopXlib.free(data)
	}
	if status != 0 || actual != typ || format != 32 || data == nil || count > 1024 {
		return nil
	}
	return slices.Clone(unsafe.Slice((*uintptr)(unsafe.Pointer(data)), count))
}

// outerPosition follows reparenting to the outermost frame window rather than
// deriving its corner from the backend's frame property implementation.
func (c *frameXClient) outerPosition(window uintptr) driver.Point {
	outer := window
	for range 8 {
		var root, parent, children uintptr
		var n uint32
		ok := c.queryTree(c.display, outer, &root, &parent, &children, &n)
		if children != 0 {
			desktopXlib.free((*byte)(cPointer(children)))
		}
		if ok == 0 || parent == c.root || parent == 0 {
			break
		}
		outer = parent
	}
	var x, y int32
	var child uintptr
	c.translate(c.display, outer, c.root, 0, 0, &x, &y, &child)
	position := driver.Point{X: driver.Points(x), Y: driver.Points(y)}
	if shadow := c.property(window, "_GTK_FRAME_EXTENTS", desktopXACardinal); len(shadow) == 4 {
		position.X += driver.Points(shadow[0])
		position.Y += driver.Points(shadow[2])
	}
	return position
}

func waitFrameCondition(t *testing.T, what string, condition func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		if condition() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(what)
		}
	}
}

func TestWindowFrameMovesOnX11AndReportsNativeMoves(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	for _, decorated := range []bool{false, true} {
		t.Run(fmt.Sprintf("decorated=%v", decorated), func(t *testing.T) {
			ready := make(chan *core.Scope, 1)
			w := startTestWindow(t, func(cx *core.Scope) core.Node {
				ready <- cx
				return core.View{Style: core.Style{Background: core.RGB(0xffffff)}}
			}, Options{Width: 200, Height: 120, NoShadow: !decorated})
			cx := <-ready
			c := openFrameXClient(t)
			var xid uintptr
			var scale driver.Points
			w.onLoop(t, func() {
				if !w.canPlace() {
					return
				}
				gtkWindowSetDecorated(w.win, boolInt(decorated))
				_, xid, _ = w.x11Window()
				scale = driver.Points(max(gtkWidgetGetScaleFactor(w.win), 1))
			})
			if xid == 0 {
				t.Skip("GDK is not using X11")
			}
			waitAt := func(x, y driver.Points) {
				t.Helper()
				want := driver.Point{X: x * scale, Y: y * scale}
				for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
					got := c.outerPosition(xid)
					if got == want {
						break
					}
					if time.Now().After(deadline) {
						var detail string
						w.onLoop(t, func() {
							position, _ := w.framePosition()
							detail = fmt.Sprintf("native size=%dx%d, pending=%+v, reported=%v", gtkWidgetGetWidth(w.win), gtkWidgetGetHeight(w.win), w.frameState.resize, position)
						})
						t.Fatalf("native X position = %v, want %v; %s", got, want, detail)
					}
				}
				waitFrameCondition(t, "the app did not observe the native window position", func() bool {
					answer := make(chan bool, 1)
					cx.Post(func() {
						f := cx.Env().WindowFrame()
						answer <- cx.Env().CanPlaceWindows() && f.Move && f.X == x && f.Y == y
					})
					return <-answer
				})
			}
			for _, point := range []driver.Point{{X: 137, Y: 91}, {X: 231, Y: 137}, {}} {
				cx.Post(func() { cx.SetWindowFrame(core.WindowFrame{Move: true, X: point.X, Y: point.Y}) })
				waitAt(point.X, point.Y)
			}
			// X and Y without Move leave the position alone while sizing still works.
			cx.Post(func() { cx.SetWindowFrame(core.WindowFrame{X: 400, Y: 300, Width: 260}) })
			waitFrameCondition(t, "width-only frame did not resize", func() bool {
				var width int32
				w.onLoop(t, func() { width = gtkWidgetGetWidth(w.win) })
				return width == 260
			})
			waitAt(0, 0)
			// An external native move must be reported without another app command or
			// focus change. No window-manager placement restriction is bypassed.
			c.move(c.display, xid, int32(90*scale), int32(70*scale))
			desktopXlib.sync(c.display, 0)
			waitAt(90, 70)
			// A frame requested while hidden is reapplied after the window maps.
			w.onLoop(t, func() {
				gtkWidgetSetVisible(w.win, 0)
				w.setFrame(driver.WindowFrame{Move: true, X: 170, Y: 110})
				gtkWidgetSetVisible(w.win, 1)
			})
			waitAt(170, 110)
			// A request below the native minimum must not leave a stale resize
			// restoration that later takes back a user's move.
			w.onLoop(t, func() { w.setConstraints(driver.WindowConstraints{MinWidth: 300}) })
			waitFrameCondition(t, "native minimum width did not apply", func() bool {
				var width int32
				w.onLoop(t, func() { width = gtkWidgetGetWidth(w.win) })
				return width >= 300
			})
			cx.Post(func() { cx.SetWindowFrame(core.WindowFrame{Width: 100, Move: true, X: 125, Y: 85}) })
			waitAt(125, 85)
			w.onLoop(t, func() {
				if w.frameState.resize != nil {
					t.Error("a clamped size left a pending position restoration")
				}
			})
			c.move(c.display, xid, int32(90*scale), int32(70*scale))
			desktopXlib.sync(c.display, 0)
			waitAt(90, 70)
		})
	}
}

func TestWindowLevelKeepsFloatingAboveAnActivatedWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("opens windows")
	}
	ready := make(chan *core.Scope, 1)
	w := startTestWindow(t, func(cx *core.Scope) core.Node {
		ready <- cx
		return core.View{Style: core.Style{Background: core.RGB(0xff0000)}}
	}, Options{Width: 180, Height: 120, NoShadow: true})
	cx := <-ready
	c := openFrameXClient(t)
	if len(c.property(c.root, "_NET_SUPPORTING_WM_CHECK", desktopXAWindow)) == 0 {
		t.Skip("requires a real EWMH window manager")
	}
	xtst, err := purego.Dlopen("libXtst.so.6", purego.RTLD_NOW|purego.RTLD_LOCAL)
	if err != nil {
		t.Skip("requires XTest for native clicks")
	}
	purego.RegisterLibFunc(&c.motion, xtst, "XTestFakeMotionEvent")
	purego.RegisterLibFunc(&c.button, xtst, "XTestFakeButtonEvent")
	var first, second, other uintptr
	var scale driver.Points
	w.onLoop(t, func() {
		if !w.canSetLevel() {
			return
		}
		gtkWindowSetDecorated(w.win, 0)
		w.setFrame(driver.WindowFrame{Move: true, X: 40, Y: 80})
		_, first, _ = w.x11Window()
		scale = driver.Points(max(gtkWidgetGetScaleFactor(w.win), 1))
		other = gtkWindowNew()
		gtkWindowSetTitle(other, cstr("nsgo-level-other"))
		gtkWindowSetDecorated(other, 0)
		gtkWindowSetDefaultSize(other, 180, 120)
		gtkWindowPresent(other)
		second = gdkX11SurfaceGetXID(gtkNativeGetSurface(other))
	})
	if first == 0 {
		t.Skip("GDK is not using managed X11 windows")
	}
	t.Cleanup(func() { w.onLoop(t, func() { gtkWindowDestroy(other) }) })
	c.move(c.display, second, int32(300*scale), int32(80*scale))
	desktopXlib.sync(c.display, 0)
	waitFrameCondition(t, "the second window did not map", func() bool {
		return slices.Contains(c.property(c.root, "_NET_CLIENT_LIST", desktopXAWindow), second)
	})
	click := func(x, y driver.Points, window uintptr) {
		t.Helper()
		c.motion(c.display, -1, int32(x*scale), int32(y*scale), 0)
		c.button(c.display, 1, 1, 0)
		c.button(c.display, 1, 0, 0)
		desktopXlib.sync(c.display, 0)
		waitFrameCondition(t, "a native click did not activate its window", func() bool {
			return slices.Contains(c.property(c.root, "_NET_ACTIVE_WINDOW", desktopXAWindow), window)
		})
	}
	above := desktopXlib.internAtom(c.display, cstr("_NET_WM_STATE_ABOVE"), 0)
	cx.Post(func() { cx.SetWindowLevel(core.WindowLevelFloating) })
	waitFrameCondition(t, "the window manager did not set ABOVE", func() bool {
		return slices.Contains(c.property(first, "_NET_WM_STATE", desktopXAAtom), above)
	})
	click(350, 130, second)
	stack := c.property(c.root, "_NET_CLIENT_LIST_STACKING", desktopXAWindow)
	if slices.Index(stack, first) <= slices.Index(stack, second) {
		t.Error("activating the other window raised it above the floating window")
	}
	cx.Post(func() { cx.SetWindowLevel(core.WindowLevelNormal) })
	waitFrameCondition(t, "normal level did not remove ABOVE", func() bool {
		return !slices.Contains(c.property(first, "_NET_WM_STATE", desktopXAAtom), above)
	})
	click(90, 130, first)
	click(350, 130, second)
	waitFrameCondition(t, "normal window stayed above the activated window", func() bool {
		stack := c.property(c.root, "_NET_CLIENT_LIST_STACKING", desktopXAWindow)
		return slices.Index(stack, second) > slices.Index(stack, first)
	})
}
