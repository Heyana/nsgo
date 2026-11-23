//go:build windows

package windows

import (
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func openNativeFramePanel(t *testing.T, dock core.PanelDock, content func(*core.Scope) core.Node) (uintptr, *window, *testOpen) {
	t.Helper()
	if testing.Short() {
		t.Skip("opens a native panel")
	}
	test := &testOpen{ready: make(chan *window, 1), options: testhost.Options{Environment: &driver.Environment{}}}
	root := startAppWith(t, core.AppRoot(func(cx *core.Scope) {
		cx.OpenPanel(core.PanelOptions{Width: 160, Height: 100, NoActivate: true, Dock: dock}, content)
	}), Options{Title: "nsgo-native-" + t.Name(), Width: 300, Height: 200, Hidden: true, test: test})
	var panel *window
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
		onMain(t, root, func(*window) {
			for _, w := range test.windows {
				if w.kind == driver.SurfacePanel && w.root != 0 {
					panel = w
				}
			}
		})
		if panel != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the native panel did not mount")
		}
	}
	var shown uintptr
	onMain(t, root, func(*window) {
		panel.windowCommand(&driver.WindowCommand{Action: driver.WindowShow})
		shown, _, _ = isWindowVisible.Call(panel.hwnd)
	})
	t.Cleanup(func() {
		onMain(t, root, func(*window) {
			panel.windowCommand(&driver.WindowCommand{Action: driver.WindowDestroy})
		})
	})
	if shown == 0 {
		if !sessionActive() {
			t.Skip("native panel visibility needs an interactive Windows desktop")
		}
		t.Fatal("the native panel did not show")
	}
	return root, panel, test
}

func nativePanelRect(t *testing.T, panel *window) rect {
	t.Helper()
	var r rect
	if ok, _, err := getWindowRect.Call(panel.hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		t.Errorf("GetWindowRect: %v", err)
	}
	return r
}

const frameOverrideSubclassID = 0x4E534750 // NSGP

var frameOverrideSubclass = syscall.NewCallback(func(hwnd, message, wParam, lParam, _, width uintptr) uintptr {
	result, _, _ := defSubclassProc.Call(hwnd, message, wParam, lParam)
	if w := windows[hwnd]; message == wmSize && w != nil && w.panelMotion.Active() {
		// A newer mount can be applied before the native SetWindowPos returns.
		w.windowCommand(&driver.WindowCommand{Action: driver.WindowSetFrame,
			Frame: driver.WindowFrame{Width: driver.Points(width)}})
	}
	return result
})

func TestPanelFrameAnimationStepsNativeWindow(t *testing.T) {
	root, panel, host := openNativeFramePanel(t, core.PanelDock{}, func(*core.Scope) core.Node { return core.View{} })
	onMain(t, root, func(*window) {
		// Keep the commands and controlled frame samples in one dispatch so a
		// posted clock tick cannot race any of these native frame observations.
		frame := func(width driver.Points, animated bool) {
			panel.windowCommand(&driver.WindowCommand{Action: driver.WindowSetFrame,
				Frame: driver.WindowFrame{Width: width, Animated: animated}})
		}
		initial := nativePanelRect(t, panel)
		check := func(width driver.Points) {
			t.Helper()
			want := initial
			want.Right = want.Left + panel.px(width)
			if got := nativePanelRect(t, panel); got != want {
				t.Errorf("native frame = %+v, want %+v", got, want)
			}
		}
		focus, _, _ := getFocus.Call()
		frame(300, true)
		if got := nativePanelRect(t, panel); got != initial {
			t.Errorf("animated request jumped to %+v from %+v", got, initial)
		}
		start := time.Now()
		panel.stepPanelFrame(start.Add(40 * time.Millisecond))
		mid := nativePanelRect(t, panel)
		if mid.Left != initial.Left || mid.Top != initial.Top || mid.Bottom != initial.Bottom ||
			mid.Right <= initial.Right || mid.Right >= initial.Left+panel.px(300) {
			t.Errorf("expected an intermediate width and fixed origin and height: %+v from %+v", mid, initial)
		}
		panel.stepPanelFrame(start.Add(time.Second))
		check(300)

		frame(160, true)
		if ok, _, err := setWindowSubclass.Call(panel.hwnd, frameOverrideSubclass, frameOverrideSubclassID, 210); ok == 0 {
			t.Errorf("SetWindowSubclass: %v", err)
		} else {
			panel.stepPanelFrame(time.Now().Add(40 * time.Millisecond))
			removeWindowSubclass.Call(panel.hwnd, frameOverrideSubclass, frameOverrideSubclassID)
		}
		panel.stepPanelFrame(time.Now().Add(time.Second))
		check(210)

		frame(280, true)
		before := time.Now()
		host.pin.Set(&driver.Environment{ReduceMotion: true})
		panel.stepPanelFrame(before)
		check(280)
		frame(230, true)
		check(230)

		host.pin.Set(&driver.Environment{})
		frame(310, true)
		panel.windowCommand(&driver.WindowCommand{Action: driver.WindowHide})
		panel.stepPanelFrame(time.Now().Add(time.Second))
		check(230)
		frame(195, true)
		check(195)
		if got, _, _ := getFocus.Call(); got != focus {
			t.Errorf("panel frame commands changed keyboard focus from %#x to %#x", focus, got)
		}
	})
}

func TestPanelFrameAnimationRepicksStationaryHover(t *testing.T) {
	events := make(chan string, 16)
	root, panel, _ := openNativeFramePanel(t, core.PanelDock{}, func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(160), Height: core.Pt(100)},
			OnMouseEnter: func(core.MouseEvent) { events <- "enter" },
			OnMouseLeave: func(core.MouseEvent) { events <- "leave" }}
	})
	onMain(t, root, func(*window) {
		// The native test input gate admits this input only; it ignores physical
		// mouse messages. The device cursor is left where the user has it.
		x, y := panel.px(20), panel.px(40)
		panel.inputGate.enter()
		sendMessage.Call(panel.hwnd, wmMouseMove, 0, uintptr(uint32(uint16(x))|uint32(uint16(y))<<16))
		panel.inputGate.leave()

		at := panel.windowFrame()
		away := at
		away.X += 100
		away.Animated = true
		panel.setWindowFrame(away)
		panel.stepPanelFrame(time.Now().Add(70 * time.Millisecond))

		// Retarget from the still-intermediate native frame. The same screen
		// point comes back inside without a second mouse message.
		old := nativePanelRect(t, panel)
		at.Animated = true
		panel.setWindowFrame(at)
		if got := nativePanelRect(t, panel); got != old {
			t.Errorf("retarget jumped from %+v to %+v", old, got)
		}
		panel.stepPanelFrame(time.Now().Add(time.Second))

		// Geometry changes in a pass-through panel cannot keep its hover.
		panel.setPointerEvents(driver.WindowPointerNone)
		at.X += 2
		panel.setWindowFrame(at)
		panel.stepPanelFrame(time.Now().Add(time.Second))
	})
	for _, want := range []string{"enter", "leave", "enter", "leave"} {
		select {
		case got := <-events:
			if got != want {
				t.Fatalf("hover = %q, want %q", got, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("no hover event, want %q", want)
		}
	}
}

func TestDockedPanelFrameAnimationKeepsItsNativeEdge(t *testing.T) {
	root, panel, _ := openNativeFramePanel(t, core.PanelDock{Edge: core.PlaceRight, Margin: 10}, func(*core.Scope) core.Node { return core.View{} })
	onMain(t, root, func(*window) {
		initial := nativePanelRect(t, panel)
		panel.windowCommand(&driver.WindowCommand{Action: driver.WindowFitSize,
			Frame: driver.WindowFrame{Width: 280, Height: 150, Animated: true}})
		if got := nativePanelRect(t, panel); got != initial {
			t.Errorf("animated dock fit jumped from %+v to %+v", initial, got)
		}
		start := time.Now()
		panel.stepPanelFrame(start.Add(40 * time.Millisecond))
		mid := nativePanelRect(t, panel)
		if mid.Right != initial.Right || mid.Left >= initial.Left || mid.Left <= initial.Right-panel.px(280) ||
			mid.Top >= initial.Top || mid.Bottom <= initial.Bottom {
			t.Errorf("dock fit did not expand at its native edge and center: %+v from %+v", mid, initial)
		}
		panel.stepPanelFrame(start.Add(time.Second))
		final := nativePanelRect(t, panel)
		if final.Right != initial.Right || final.Right-final.Left != panel.px(280) || final.Bottom-final.Top != panel.px(150) ||
			final.Top+final.Bottom < initial.Top+initial.Bottom-2 || final.Top+final.Bottom > initial.Top+initial.Bottom+2 {
			t.Errorf("docked frame = %+v, want 280 by 150 points at the edge and center of %+v", final, initial)
		}
	})
}
