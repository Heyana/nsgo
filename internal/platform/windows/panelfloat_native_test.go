//go:build windows

package windows

import (
	"os"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestPanelsFloatAboveTheOrdinaryWindows opens a panel that takes the focus
// and one that never does, as desktop.OpenPanel opens them, shows both, and
// reads their extended style from the system: each floats above the ordinary
// windows, and only the second says it takes no focus. A panel without the
// topmost style sank under a maximized window in front, which left the app
// running with no window in sight. It then moves the first panel among the
// ordinary windows, as SetWindowLevel(WindowLevelNormal) asks, and fronting it
// leaves it there; back at the floating level, fronting it floats it again.
func TestPanelsFloatAboveTheOrdinaryWindows(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	// The process names the root, so a window another run left behind with the
	// same title is not taken for this one.
	title := "nsgo-native-panel-float-" + strconv.Itoa(os.Getpid())
	appScopes := make(chan *core.Scope, 1)
	opened := make(chan []*core.Window, 1)
	blank := func(cx *core.Scope) core.Node { return core.View{Style: core.Style{Background: driver.RGB(0xffffff)}} }
	app := core.AppRoot(func(cx *core.Scope) {
		appScopes <- cx
		opened <- []*core.Window{
			cx.OpenPanel(core.PanelOptions{Width: 200, Height: 160}, blank),
			cx.OpenPanel(core.PanelOptions{Width: 200, Height: 160, NoActivate: true}, blank),
		}
	})
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: title, Width: 240, Height: 160, Hidden: true})
	}()
	root := waitForWindow(t, title, done)
	if root == 0 {
		return
	}
	// The shell lists the window as soon as it is created, which is a moment
	// before the platform registers it for the loop.
	for deadline := time.Now().Add(3 * time.Second); windows[root] == nil; time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the root did not reach the loop")
		}
	}
	var appCx *core.Scope
	select {
	case appCx = <-appScopes:
	case <-time.After(5 * time.Second):
		t.Fatal("the app did not start")
	}
	t.Cleanup(func() {
		appCx.Post(appCx.Quit)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the app did not close")
		}
	})
	var panels []*core.Window
	select {
	case panels = <-opened:
	case <-time.After(5 * time.Second):
		t.Fatal("the app did not open the panels")
	}
	for i, p := range panels {
		if p == nil {
			t.Fatalf("panel %d did not open", i)
		}
	}

	// The panels are the root's opened surfaces, which the platform creates on
	// its thread: pick them out of the window list there, by the focus one
	// takes.
	type pair struct{ focus, quiet uintptr }
	found := func() pair {
		var p pair
		onMain(t, root, func(*window) {
			for h, w := range windows {
				if w.kind != driver.SurfacePanel {
					continue
				}
				if w.noActivate {
					p.quiet = h
				} else {
					p.focus = h
				}
			}
		})
		return p
	}
	var got pair
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		if got = found(); got.focus != 0 && got.quiet != 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("panels showed as %+v; want one that takes the focus and one that never does", got)
		}
	}

	const gwlExStyle = ^uintptr(19) // -20
	exStyle := func(h uintptr) uintptr {
		ex, _, _ := getWindowLongProc.Call(h, gwlExStyle)
		return ex
	}
	if ex := exStyle(got.focus); ex&wsExTopmost == 0 {
		t.Errorf("the panel that takes the focus has ex style %#x; want it above the ordinary windows", ex)
	} else if ex&wsExNoActivate != 0 {
		t.Errorf("the panel that takes the focus has ex style %#x; want it taking the focus", ex)
	}
	if ex := exStyle(got.quiet); ex&(wsExTopmost|wsExNoActivate) != wsExTopmost|wsExNoActivate {
		t.Errorf("the panel that never takes the focus has ex style %#x; want it above the ordinary windows and saying so", ex)
	}

	// Shown, both float there still.
	appCx.Post(func() {
		panels[0].Show()
		panels[1].Show()
	})
	shown := func(h uintptr) bool {
		v := false
		onMain(t, root, func(*window) {
			if p := windows[h]; p != nil {
				v = p.visible
			}
		})
		return v
	}
	for deadline := time.Now().Add(5 * time.Second); !shown(got.focus) || !shown(got.quiet); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the panels did not show")
		}
	}
	for _, h := range []uintptr{got.focus, got.quiet} {
		if ex := exStyle(h); ex&wsExTopmost == 0 {
			t.Errorf("panel %#x shows with ex style %#x; want it above the ordinary windows", h, ex)
		}
	}

	// SetWindowLevel(WindowLevelNormal) leaves a panel among the ordinary
	// windows, and fronting it does not float it again; the floating level
	// floats it.
	on := func(action func(w *window)) {
		onMain(t, root, func(*window) {
			if p := windows[got.focus]; p != nil {
				action(p)
			}
		})
	}
	topmost := func() bool {
		v := false
		on(func(w *window) { v = exStyle(got.focus)&wsExTopmost != 0 })
		return v
	}
	on(func(w *window) { w.setLevel(driver.WindowLevelNormal) })
	if topmost() {
		t.Fatal("a panel the app moved among the ordinary windows kept the topmost style")
	}
	on(func(w *window) { w.orderFront() })
	if topmost() {
		t.Fatal("fronting a panel the app moved among the ordinary windows floated it above them again")
	}
	on(func(w *window) { w.setLevel(driver.WindowLevelFloating) })
	if !topmost() {
		t.Fatal("a panel the app floated again stayed among the ordinary windows")
	}
	on(func(w *window) { w.orderFront() })
}
