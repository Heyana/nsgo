//go:build windows

package windows

import (
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

type livePointerCounts struct{ press, wheel, enter, context atomic.Int32 }

// TestWindowPointerModesRouteTheSystemPointer lays a 600 by 400 window
// over another and moves the system's own pointer over them, pressing,
// turning the wheel and pressing the right button, so Windows routes each
// message, never NSGo's hit test (#1380). With WindowPointerNone all of it
// reaches the window below; with WindowPointerPainted a 200 by 200 island
// takes its own, its context menu and a WindowDrag of its strip, while
// what is around it reaches the window below; Auto takes it all again,
// all without reopening the window or losing what it shows, plain and
// with a material, which draws through DirectComposition. It moves the pointer of an interactive
// desktop session, so it runs only with NSGO_LIVE_POINTER=1.
func TestWindowPointerModesRouteTheSystemPointer(t *testing.T) {
	if os.Getenv("NSGO_LIVE_POINTER") != "1" {
		t.Skip("set NSGO_LIVE_POINTER=1 in an interactive desktop session")
	}
	for _, c := range []struct {
		name     string
		material core.WindowMaterial
	}{{"plain", core.WindowMaterialNone}, {"mica", core.WindowMaterialMica}} {
		t.Run(c.name, func(t *testing.T) { livePointerModes(t, c.material) })
	}
}

func livePointerModes(t *testing.T, material core.WindowMaterial) {
	lowerTitle, upperTitle := "nsgo-pointer1380-lower-"+t.Name(), "nsgo-pointer1380-upper-"+t.Name()
	var lower, upper livePointerCounts
	at := func(x, y, w, h driver.Points, rgb uint32) core.Style {
		s := core.Style{Position: core.Absolute, Left: driver.Pt(x), Top: driver.Pt(y), Width: driver.Pt(w), Height: driver.Pt(h)}
		if rgb != 0 {
			s.Background = driver.RGB(rgb)
		}
		return s
	}
	ready := make(chan *core.Scope, 1)
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		cx.OpenWindow(core.WindowOptions{Title: lowerTitle, Width: 600, Height: 400}, func(*core.Scope) core.Node {
			return core.View{Style: core.Style{FlexGrow: 1, Background: driver.RGB(0x44aa44)},
				OnPress: func(core.PressEvent) { lower.press.Add(1) }, OnWheel: func(core.WheelEvent) { lower.wheel.Add(1) },
				OnMouseEnter: func(core.MouseEvent) { lower.enter.Add(1) }}
		})
		cx.OpenWindow(core.WindowOptions{Title: upperTitle, Width: 600, Height: 400, Material: material}, func(cx *core.Scope) core.Node {
			ready <- cx
			return core.View{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
				core.View{Style: at(100, 100, 200, 200, 0x2244aa),
					OnPress: func(core.PressEvent) { upper.press.Add(1) }, OnWheel: func(core.WheelEvent) { upper.wheel.Add(1) },
					OnMouseEnter: func(core.MouseEvent) { upper.enter.Add(1) }, OnContextMenu: func(core.ContextMenuEvent) { upper.context.Add(1) },
					Children: []core.Node{core.View{Desktop: core.DesktopViewProps{WindowDrag: core.WindowDragOn},
						Style: at(0, 0, 200, 40, 0), OnPress: func(core.PressEvent) { upper.press.Add(1) }, Children: []core.Node{
							core.View{Style: at(0, 0, 50, 40, 0), OnPress: func(core.PressEvent) { upper.press.Add(1) }},
						}}}},
			}}
		})
	})
	root := startAppWith(t, app, Options{Title: "nsgo-pointer1380-host-" + t.Name(), Width: 200, Height: 100, Hidden: true})
	var cx *core.Scope
	select {
	case cx = <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("the upper window did not render")
	}
	find := func(title string) uintptr {
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
			if hwnd, _, _ := findWindowW.Call(0, uintptr(unsafe.Pointer(wide(title)))); hwnd != 0 {
				return hwnd
			}
			if time.Now().After(deadline) {
				t.Fatalf("no window %q", title)
			}
		}
	}
	bottom, top := find(lowerTitle), find(upperTitle)
	var scale float64
	onMain(t, root, func(*window) {
		for _, hwnd := range []uintptr{bottom, top} {
			setWindowPos.Call(hwnd, 0, 100, 100, 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
		}
		scale = windows[top].scale
	})
	raise := func() { setForegroundWindow.Call(top); time.Sleep(50 * time.Millisecond) }
	origin := func() point {
		p := point{}
		clientToScreen.Call(top, uintptr(unsafe.Pointer(&p)))
		return p
	}
	move := func(x, y float64) {
		o := origin()
		setCursorPos.Call(uintptr(o.X+int32(x*scale)), uintptr(o.Y+int32(y*scale)))
		time.Sleep(40 * time.Millisecond)
	}
	// leave moves the pointer off both windows, so every route sees an
	// enter.
	leave := func() {
		o := origin()
		setCursorPos.Call(uintptr(o.X+int32(700*scale)), uintptr(o.Y+int32(500*scale)))
		time.Sleep(60 * time.Millisecond)
	}
	const (
		leftDown, leftUp, rightDown, rightUp, wheel = 0x2, 0x4, 0x8, 0x10, 0x800
	)
	input := func(flags uintptr, data int32) {
		mouseEvent.Call(flags, 0, 0, uintptr(uint32(data)), 0)
		time.Sleep(20 * time.Millisecond)
	}
	wait := func(what string, ok func() bool, detail func() string) {
		t.Helper()
		for deadline := time.Now().Add(3 * time.Second); !ok(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s: %s", what, detail())
			}
		}
	}
	counts := func() string {
		return fmt.Sprintf("lower press %d wheel %d enter %d; upper press %d wheel %d enter %d context %d",
			lower.press.Load(), lower.wheel.Load(), lower.enter.Load(),
			upper.press.Load(), upper.wheel.Load(), upper.enter.Load(), upper.context.Load())
	}
	setMode := func(mode core.WindowPointerEvents) {
		t.Helper()
		cx.Post(func() { cx.SetWindowPointerEvents(mode) })
		wait("the pointer mode", func() bool {
			var applied bool
			onMain(t, top, func(w *window) { applied = w.pointerMode == mode })
			return applied
		}, func() string { return fmt.Sprint(mode) })
		raise()
		// The window still shows its content, layered or not.
		if got, ok := waitForPixel(t, top, int32(200*scale), int32(220*scale), 0x2244aa, 2*time.Second); !ok {
			t.Errorf("with %v the island shows #%06x, want #2244aa", mode, got)
		}
	}
	// click presses at x, y, where to must hear it and other must not.
	click := func(x, y float64, to, other *atomic.Int32) {
		t.Helper()
		before, otherBefore := to.Load(), other.Load()
		leave()
		move(x, y)
		input(leftDown, 0)
		input(leftUp, 0)
		wait(fmt.Sprintf("a press at %v, %v", x, y), func() bool { return to.Load() > before }, counts)
		time.Sleep(50 * time.Millisecond)
		if other.Load() != otherBefore {
			t.Errorf("the press at %v, %v also reached the other window: %s", x, y, counts())
		}
	}
	// taken presses at x, y, where the upper window, which has nothing
	// there, takes the press from the lower one.
	taken := func(x, y float64) {
		t.Helper()
		before := lower.press.Load()
		leave()
		move(x, y)
		input(leftDown, 0)
		input(leftUp, 0)
		time.Sleep(200 * time.Millisecond)
		if lower.press.Load() != before {
			t.Errorf("the press at %v, %v went through the window: %s", x, y, counts())
		}
	}
	// hoverAndWheel enters at x, y and turns the wheel, which win must hear.
	hoverAndWheel := func(x, y float64, win, other *livePointerCounts) {
		t.Helper()
		entered, scrolled, otherScrolled := win.enter.Load(), win.wheel.Load(), other.wheel.Load()
		leave()
		move(x, y)
		input(wheel, -120)
		wait(fmt.Sprintf("hover and wheel at %v, %v", x, y), func() bool {
			return win.enter.Load() > entered && win.wheel.Load() > scrolled
		}, counts)
		if other.wheel.Load() != otherScrolled {
			t.Errorf("the wheel at %v, %v also reached the other window: %s", x, y, counts())
		}
	}
	raise()
	taken(500, 350)
	setMode(core.WindowPointerNone)
	click(200, 220, &lower.press, &upper.press)
	click(500, 350, &lower.press, &upper.press)
	hoverAndWheel(200, 220, &lower, &upper)

	setMode(core.WindowPointerPainted)
	click(200, 220, &upper.press, &lower.press)
	hoverAndWheel(200, 220, &upper, &lower)
	click(500, 350, &lower.press, &upper.press)
	raise()
	hoverAndWheel(500, 350, &lower, &upper)
	raise()
	leave()
	move(200, 220)
	context := upper.context.Load()
	input(rightDown, 0)
	input(rightUp, 0)
	wait("the island's context menu", func() bool { return upper.context.Load() > context }, counts)
	var before, after rect
	getWindowRect.Call(top, uintptr(unsafe.Pointer(&before)))
	// A control in the drag strip takes its own press.
	click(120, 120, &upper.press, &lower.press)
	getWindowRect.Call(top, uintptr(unsafe.Pointer(&after)))
	if after != before {
		t.Error("a press on a control in the drag strip moved the window")
	}
	// The strip itself drags the window.
	raise()
	leave()
	move(200, 120)
	input(leftDown, 0)
	o := origin()
	for i := 1; i <= 8; i++ {
		setCursorPos.Call(uintptr(o.X+int32((200+float64(i)*5)*scale)), uintptr(o.Y+int32((120+float64(i)*5)*scale)))
		time.Sleep(20 * time.Millisecond)
	}
	input(leftUp, 0)
	wait("a WindowDrag of the island's strip", func() bool {
		getWindowRect.Call(top, uintptr(unsafe.Pointer(&after)))
		return after.Left != before.Left && after.Top != before.Top
	}, func() string { return fmt.Sprint(before, after) })
	// The lower window stays where the upper one was, so put them back.
	setWindowPos.Call(top, 0, uintptr(before.Left), uintptr(before.Top), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
	time.Sleep(100 * time.Millisecond)

	setMode(core.WindowPointerAuto)
	taken(500, 350)
	click(200, 220, &upper.press, &lower.press)
	if find(upperTitle) != top || find(lowerTitle) != bottom {
		t.Error("changing pointer modes replaced a window")
	}
}
