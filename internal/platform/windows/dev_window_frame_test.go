//go:build windows && nsgodev

package windows

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestDevWindowFrameRestoresPlacementAndFullScreen(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	path := filepath.Join(t.TempDir(), "frame")
	const title = "nsgo-dev-window-frame"
	type initial struct {
		scope *core.Scope
		state devWindowState
	}
	start := func() (uintptr, devWindowState, func()) {
		t.Helper()
		t.Setenv(devWindowFrameEnv, path)
		ready := make(chan initial, 1)
		done := make(chan struct{})
		go func() {
			runtime.LockOSThread()
			defer close(done)
			Run(func(cx *core.Scope) core.Node {
				env := cx.Env()
				ready <- initial{cx, devWindowState{Frame: env.WindowFrame(), FullScreen: env.FullScreen()}}
				return core.View{}
			}, Options{Title: title, Width: 350, Height: 220})
		}()
		hwnd := waitForWindow(t, title, done)
		select {
		case opened := <-ready:
			stop := sync.OnceFunc(func() {
				opened.scope.Post(opened.scope.Quit)
				select {
				case <-done:
				case <-time.After(5 * time.Second):
					t.Error("Quit did not close the test window")
				}
			})
			t.Cleanup(stop)
			return hwnd, opened.state, stop
		case <-time.After(5 * time.Second):
			t.Fatal("the app did not render")
			return 0, devWindowState{}, func() {}
		}
	}
	waitForState := func(want devWindowState) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			data, err := os.ReadFile(path)
			var saved devWindowState
			if err == nil && json.Unmarshal(data, &saved) == nil && saved == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("saved window state = %+v, want %+v", saved, want)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	hwnd, first, stop := start()
	moved := first
	onMain(t, hwnd, func(w *window) {
		frame := first.Frame
		frame.X, frame.Y = frame.X+30, frame.Y+25
		frame.Width, frame.Height = frame.Width+40, frame.Height+35
		w.placeAt(frame)
		moved.Frame = w.windowFrame()
	})
	if moved == first {
		t.Fatal("the first window did not move")
	}
	waitForState(moved)
	stop()

	hwnd, second, stop := start()
	if second != moved {
		t.Fatalf("reopened state = %+v, want %+v", second, moved)
	}
	onMain(t, hwnd, func(w *window) { w.toggleFullScreen() })
	full := moved
	full.FullScreen = true
	waitForState(full)
	stop()

	hwnd, third, stop := start()
	if !third.FullScreen {
		t.Fatal("the first render after reopening was not full screen")
	}
	onMain(t, hwnd, func(w *window) { w.toggleFullScreen() })
	waitForState(moved)
	stop()
}
