//go:build windows

package windows

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestWindowsLastWindowAsksBeforeQuitting(t *testing.T) {
	if testing.Short() {
		t.Skip("opens windows")
	}
	var closing, requests atomic.Int32
	answer := make(chan func(bool), 1)
	const title = "nsgo-last-window-quit"
	app := core.AppRoot(func(cx *core.Scope) {
		win := cx.OpenWindow(core.WindowOptions{Title: title + "-main"}, func(*core.Scope) core.Node { return core.Text{Content: "Last window"} })
		win.OnClosing(cx, func(c *core.WindowClosing) {
			if closing.Add(1) == 1 {
				c.Cancel()
			}
		})
		cx.OnQuitRequest(func(q *core.QuitRequest) {
			if requests.Add(1) == 1 {
				q.Cancel()
			} else {
				answer <- q.Defer()
			}
		})
	})
	root, done := runQuitTestApp(t, title, app)
	hwnd := waitForWindow(t, title+"-main", done)
	syncEngine := func() {
		onMain(t, root, func(w *window) { w.engine.Sync() })
	}
	for i := int32(1); i <= 3; i++ {
		// Alt+F4 and the caption's close button send this system command.
		sendMessage.Call(hwnd, wmSysCommand, scClose, 0)
		syncEngine()
		if got := closing.Load(); got != i {
			t.Fatalf("close %d ran OnClosing %d times", i, got)
		}
		if got := requests.Load(); got != i-1 {
			t.Fatalf("close %d ran OnQuitRequest %d times", i, got)
		}
		if hwndVisible, _, _ := isWindowVisible.Call(hwnd); hwndVisible == 0 {
			t.Fatalf("close %d took the window away before approval", i)
		}
	}
	(<-answer)(true)
	waitQuitTestApp(t, done)
}

func TestWindowsSessionEndForcesQuit(t *testing.T) {
	if testing.Short() {
		t.Skip("opens windows")
	}
	userRequest := make(chan struct{}, 1)
	var forced, cleaned atomic.Bool
	app := core.AppRoot(func(cx *core.Scope) {
		cx.OnQuitRequest(func(q *core.QuitRequest) {
			if !q.Forced() {
				q.Defer()
				userRequest <- struct{}{}
				return
			}
			q.Cancel()
			q.Defer()(false)
			forced.Store(!q.Canceled())
		})
		cx.OnQuit(func(context.Context) { cleaned.Store(forced.Load()) })
	})
	root, done := runQuitTestApp(t, "nsgo-end-session", app)
	postMessageW.Call(root, wmClose, 0, 0)
	select {
	case <-userRequest:
	case <-time.After(5 * time.Second):
		t.Fatal("the user quit did not reach the app")
	}
	if result, _, _ := sendMessage.Call(root, wmQueryEndSession, 0, 0); result != 1 {
		t.Fatalf("WM_QUERYENDSESSION returned %d", result)
	}
	sendMessage.Call(root, wmEndSession, 0, 0)
	if forced.Load() || cleaned.Load() {
		t.Fatal("a canceled session end quit the app")
	}
	sendMessage.Call(root, wmEndSession, 1, 0)
	if !cleaned.Load() {
		t.Fatal("WM_ENDSESSION returned before forced quit cleanup")
	}
	waitQuitTestApp(t, done)
}

func runQuitTestApp(t *testing.T, title string, app func(*core.Scope) core.Node) (uintptr, <-chan struct{}) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(app, Options{Title: title, Width: 200, Height: 100, Hidden: true})
	}()
	root := waitForWindow(t, title, done)
	t.Cleanup(func() {
		select {
		case <-done:
			return
		default:
		}
		onMain(t, root, func(w *window) { w.finishQuit() })
		waitQuitTestApp(t, done)
	})
	return root, done
}

func waitQuitTestApp(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the app did not quit")
	}
}
