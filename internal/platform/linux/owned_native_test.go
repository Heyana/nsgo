//go:build linux && !android

package linux

import (
	"os"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TestOwnedWindowsFollowVisibility exercises native transience and the app's
// separate visibility intent through nested windows, explicit hides and closes.
func TestOwnedWindowsFollowVisibility(t *testing.T) {
	if testing.Short() {
		t.Skip("opens windows")
	}
	type handles struct {
		cx                                       *core.Scope
		parent, child, grandchild, hidden, other *core.Window
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
		other := cx.OpenWindow(opts("nsgo-owned-other", nil), view)
		opened <- handles{cx, parent, child, grandchild, hidden, other}
	})
	w := startTestWindow(t, app, Options{Title: "nsgo-owned-app", Width: 120, Height: 80, Hidden: true})
	defer w.onLoop(t, w.close)
	h := <-opened
	native := map[string]*window{}
	wait := func(what string, ready func() bool) {
		t.Helper()
		for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			ok := false
			w.onLoop(t, func() { ok = ready() })
			if ok {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal(what)
			}
		}
	}
	wait("all windows did not attach", func() bool {
		windowHandles.Range(func(_, value any) bool {
			o := value.(*window)
			if o.opener == w && o.win != 0 {
				native[goString(gtkWindowGetTitle(o.win))] = o
			}
			return true
		})
		return len(native) == 7
	})
	parent, child, grandchild, hidden, other := native["nsgo-owned-parent"], native["nsgo-owned-child"], native["nsgo-owned-grandchild"], native["nsgo-owned-hidden"], native["nsgo-owned-other"]
	shown := func(o *window) bool { return o.win != 0 && gtkWidgetGetVisible(o.win) != 0 }
	w.onLoop(t, func() {
		if shown(native["nsgo-owned-initial-child"]) {
			t.Error("a child opened through its initially hidden owner")
		}
		gtkWidgetSetVisible(native["nsgo-owned-initial-parent"].win, 1)
	})
	wait("initially suppressed child did not return", func() bool { return shown(native["nsgo-owned-initial-child"]) })
	w.onLoop(t, func() {
		var transient func(uintptr) uintptr
		purego.RegisterLibFunc(&transient, gtkLib, "gtk_window_get_transient_for")
		if transient(child.win) != parent.win || transient(grandchild.win) != child.win || transient(hidden.win) != parent.win || transient(other.win) != 0 {
			t.Error("WindowOptions.Parent did not become GTK transience")
		}
	})
	wait("initial visibility differs", func() bool {
		return shown(parent) && shown(child) && shown(grandchild) && !shown(hidden) && shown(other)
	})
	h.cx.Post(h.parent.Hide)
	wait("children stayed visible with a hidden parent", func() bool {
		return !shown(parent) && !shown(child) && !shown(grandchild) && !shown(hidden) && shown(other)
	})
	h.cx.Post(h.parent.Show)
	wait("visible children did not return", func() bool { return shown(parent) && shown(child) && shown(grandchild) && !shown(hidden) })
	h.cx.Post(func() { h.child.Hide(); h.parent.Hide() })
	wait("explicit hide did not reach native windows", func() bool { return !shown(parent) && !shown(child) && !shown(grandchild) })
	h.cx.Post(h.parent.Show)
	wait("parent did not return", func() bool { return shown(parent) })
	w.onLoop(t, func() {
		if shown(child) || shown(grandchild) || shown(hidden) {
			t.Error("parent restore resurrected an explicitly hidden child")
		}
	})
	h.cx.Post(func() { h.parent.Hide(); h.child.Show(); h.hidden.Show() })
	wait("parent did not hide again", func() bool { return !shown(parent) })
	w.onLoop(t, func() {
		if shown(child) || shown(grandchild) || shown(hidden) {
			t.Error("show escaped a hidden owner")
		}
	})
	h.cx.Post(h.parent.Show)
	wait("pending child shows did not follow parent restore", func() bool { return shown(parent) && shown(child) && shown(grandchild) && shown(hidden) })
	// Native owner visibility can change without a Window command.
	w.onLoop(t, func() { gtkWidgetSetVisible(parent.win, 0) })
	wait("native hide did not hide descendants", func() bool { return !shown(child) && !shown(grandchild) && !shown(hidden) && shown(other) })
	w.onLoop(t, func() { gtkWidgetSetVisible(parent.win, 1) })
	wait("native restore did not restore descendants", func() bool { return shown(child) && shown(grandchild) && shown(hidden) })
	h.cx.Post(h.parent.Close)
	wait("closing the owner left native descendants", func() bool {
		return parent.win == 0 && child.win == 0 && grandchild.win == 0 && hidden.win == 0 && shown(other)
	})
}

// TestOwnedWindowsFollowMinimization needs a window manager: Xvfb alone cannot
// apply GTK's asynchronous minimize request or emit its GDK state change.
func TestOwnedWindowsFollowMinimization(t *testing.T) {
	if testing.Short() {
		t.Skip("opens windows")
	}
	opened := make(chan *core.Scope, 1)
	app := core.AppRoot(func(cx *core.Scope) {
		cx.SetShutdownMode(core.ShutdownOnExplicit)
		view := func(*core.Scope) core.Node { return core.View{} }
		parent := cx.OpenWindow(core.WindowOptions{Title: "nsgo-min-parent", Width: 160, Height: 120}, view)
		cx.OpenWindow(core.WindowOptions{Title: "nsgo-min-child", Width: 160, Height: 120, Parent: parent}, view)
		opened <- cx
	})
	w := startTestWindow(t, app, Options{Title: "nsgo-min-app", Width: 120, Height: 80, Hidden: true})
	defer w.onLoop(t, w.close)
	<-opened
	var parent, child *window
	wait := func(what string, ready func() bool) {
		t.Helper()
		for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			ok := false
			w.onLoop(t, func() { ok = ready() })
			if ok {
				return
			}
			if time.Now().After(deadline) {
				t.Fatal(what)
			}
		}
	}
	wait("windows did not attach", func() bool {
		windowHandles.Range(func(_, value any) bool {
			o := value.(*window)
			if o.opener == w && o.win != 0 {
				switch goString(gtkWindowGetTitle(o.win)) {
				case "nsgo-min-parent":
					parent = o
				case "nsgo-min-child":
					child = o
				}
			}
			return true
		})
		return parent != nil && child != nil && gtkWidgetGetMapped(parent.win) != 0 && gtkWidgetGetMapped(child.win) != 0
	})
	var wm bool
	w.onLoop(t, func() { wm = desktopScreen() != 0 })
	if !wm {
		if os.Getenv("NSGO_OWNED_WINDOWS_REQUIRE_WM") == "1" {
			t.Fatal("required X11 window manager is unavailable")
		}
		t.Skip("needs an X11 window manager supporting EWMH")
	}
	w.onLoop(t, func() { gtkWindowMinimize(parent.win) })
	wait("minimization did not hide owned window", func() bool {
		return gdkToplevelGetState(gtkNativeGetSurface(parent.win))&toplevelMinimized != 0 && gtkWidgetGetVisible(child.win) == 0
	})
	w.onLoop(t, func() { gtkWindowPresent(parent.win) })
	wait("restore did not show owned window", func() bool {
		return gdkToplevelGetState(gtkNativeGetSurface(parent.win))&toplevelMinimized == 0 && gtkWidgetGetVisible(child.win) != 0
	})
	w.onLoop(t, func() {
		child.windowCommand(&driver.WindowCommand{Action: driver.WindowHide})
		gtkWindowMinimize(parent.win)
	})
	wait("owner did not minimize again", func() bool { return gdkToplevelGetState(gtkNativeGetSurface(parent.win))&toplevelMinimized != 0 })
	w.onLoop(t, func() { gtkWindowPresent(parent.win) })
	wait("owner did not restore again", func() bool { return gdkToplevelGetState(gtkNativeGetSurface(parent.win))&toplevelMinimized == 0 })
	w.onLoop(t, func() {
		if gtkWidgetGetVisible(child.win) != 0 {
			t.Error("restore resurrected an explicitly hidden child")
		}
	})
}
