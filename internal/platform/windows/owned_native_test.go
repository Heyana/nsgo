//go:build windows

package windows

import (
	"runtime"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestOwnedWindowsFollowVisibility exercises Win32 ownership, stacking and
// visibility through nested owners, minimization and independent child hides.
func TestOwnedWindowsFollowVisibility(t *testing.T) {
	if testing.Short() {
		t.Skip("opens windows")
	}
	type handles struct {
		cx                                *core.Scope
		parent, child, grandchild, hidden *core.Window
	}
	opened := make(chan handles, 1)
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		view := func(*core.Scope) core.Node { return core.View{} }
		opts := func(title string, parent *core.Window) core.WindowOptions {
			return core.WindowOptions{Title: title, Width: 160, Height: 120, Parent: parent}
		}
		initialOpts := opts("nsgo-owned-initial-parent", nil)
		initialOpts.Hidden = true
		initialParent := cx.OpenWindow(initialOpts, view)
		cx.OpenWindow(opts("nsgo-owned-initial-child", initialParent), view)
		parent := cx.OpenWindow(opts("nsgo-owned-parent", nil), view)
		child := cx.OpenWindow(opts("nsgo-owned-child", parent), view)
		grandchild := cx.OpenWindow(opts("nsgo-owned-grandchild", child), view)
		hiddenOpts := opts("nsgo-owned-hidden", parent)
		hiddenOpts.Hidden = true
		hidden := cx.OpenWindow(hiddenOpts, view)
		cx.OpenWindow(opts("nsgo-owned-other", nil), view)
		opened <- handles{cx, parent, child, grandchild, hidden}
	})
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: "nsgo-owned-app", Width: 120, Height: 80, Hidden: true})
	}()
	root := waitForWindow(t, "nsgo-owned-app", done)
	t.Cleanup(func() {
		postMessageW.Call(root, wmClose, 0, 0)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("app did not close")
		}
	})
	parent := waitForWindow(t, "nsgo-owned-parent", done)
	child := waitForWindow(t, "nsgo-owned-child", done)
	grandchild := waitForWindow(t, "nsgo-owned-grandchild", done)
	hidden := waitForWindow(t, "nsgo-owned-hidden", done)
	other := waitForWindow(t, "nsgo-owned-other", done)
	initialParent := waitForWindow(t, "nsgo-owned-initial-parent", done)
	initialChild := waitForWindow(t, "nsgo-owned-initial-child", done)
	h := <-opened
	shown := func(hwnd uintptr) bool { r, _, _ := isWindowVisible.Call(hwnd); return r != 0 }
	wait := func(what string, ready func() bool) {
		t.Helper()
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			ok := false
			onMain(t, root, func(*window) { ok = ready() })
			if ok {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal(what)
			}
		}
	}
	onMain(t, root, func(*window) {
		if shown(initialChild) {
			t.Error("a child opened through its initially hidden owner")
		}
		showWindow.Call(initialParent, swShowNA)
	})
	wait("initially suppressed child did not return", func() bool { return shown(initialChild) })
	onMain(t, root, func(*window) {
		const gwOwner = 4
		for hwnd, want := range map[uintptr]uintptr{parent: 0, child: parent, grandchild: child, hidden: parent, other: 0} {
			owner, _, _ := getWindowProc.Call(hwnd, gwOwner)
			if owner != want {
				t.Errorf("owner of %x = %x, want %x", hwnd, owner, want)
			}
			style, _, _ := getWindowLongPtrW.Call(hwnd, gwlStyle)
			if style&wsChild != 0 {
				t.Error("owned window became an embedded child")
			}
		}
	})
	wait("initial visibility differs", func() bool {
		return shown(parent) && shown(child) && shown(grandchild) && !shown(hidden) && shown(other)
	})
	// Raising an owner must leave its child above it in the native Z order.
	onMain(t, parent, func(w *window) { w.orderFront() })
	onMain(t, root, func(*window) {
		const gwHwndPrev = 3
		found := false
		for at, _, _ := getWindowProc.Call(parent, gwHwndPrev); at != 0; at, _, _ = getWindowProc.Call(at, gwHwndPrev) {
			if at == child {
				found = true
				break
			}
		}
		if !found {
			t.Error("owned child fell behind its owner")
		}
	})
	h.cx.Post(h.parent.Hide)
	wait("children stayed visible with hidden owner", func() bool {
		return !shown(parent) && !shown(child) && !shown(grandchild) && !shown(hidden) && shown(other)
	})
	h.cx.Post(h.parent.Show)
	wait("visible descendants did not return", func() bool { return shown(parent) && shown(child) && shown(grandchild) && !shown(hidden) })
	onMain(t, parent, func(w *window) { w.minimize() })
	wait("minimization did not hide descendants", func() bool { return isIconicWindow(parent) && !shown(child) && !shown(grandchild) && !shown(hidden) })
	onMain(t, parent, func(*window) { showWindow.Call(parent, swRestore) })
	wait("restore did not return descendants", func() bool { return !isIconicWindow(parent) && shown(child) && shown(grandchild) && !shown(hidden) })
	h.cx.Post(func() { h.child.Hide(); h.parent.Hide() })
	wait("explicit hides did not reach native windows", func() bool { return !shown(parent) && !shown(child) && !shown(grandchild) })
	h.cx.Post(h.parent.Show)
	wait("owner did not return", func() bool { return shown(parent) })
	onMain(t, root, func(*window) {
		if shown(child) || shown(grandchild) || shown(hidden) {
			t.Error("restore resurrected an explicitly hidden child")
		}
	})
	h.cx.Post(func() { h.parent.Hide(); h.child.Show(); h.hidden.Show() })
	wait("owner did not hide again", func() bool { return !shown(parent) })
	onMain(t, root, func(*window) {
		if shown(child) || shown(grandchild) || shown(hidden) {
			t.Error("show escaped a hidden owner")
		}
	})
	h.cx.Post(h.parent.Show)
	wait("pending child shows did not follow restore", func() bool { return shown(parent) && shown(child) && shown(grandchild) && shown(hidden) })
	onMain(t, parent, func(*window) { showWindow.Call(parent, swHide) })
	wait("native hide did not hide descendants", func() bool { return !shown(child) && !shown(grandchild) && !shown(hidden) && shown(other) })
	onMain(t, parent, func(*window) { showWindow.Call(parent, swShowNA) })
	wait("native restore did not restore descendants", func() bool { return shown(child) && shown(grandchild) && shown(hidden) })
	h.cx.Post(h.parent.Close)
	wait("close left owned native windows", func() bool {
		return windows[parent] == nil && windows[child] == nil && windows[grandchild] == nil && windows[hidden] == nil && shown(other)
	})
	// A later destroy batch for an already natively destroyed owned HWND is safe.
	onMain(t, other, func(w *window) { w.windowCommand(&driver.WindowCommand{Action: driver.WindowHide}) })
}
