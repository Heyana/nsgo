//go:build windows

package windows

import (
	"runtime"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
)

func TestExplicitFullScreenRestoresWindowFrame(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	const title = "nsgo-explicit-full-screen"
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		Run(func(*core.Scope) core.Node { return core.View{} }, Options{Title: title, Width: 320, Height: 220})
	}()
	hwnd := waitForWindow(t, title, done)
	t.Cleanup(func() {
		postMessageW.Call(hwnd, wmClose, 0, 0)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the app did not close")
		}
	})
	onMain(t, hwnd, func(w *window) {
		placements := monitorLayout()
		if len(placements) == 0 {
			t.Error("no monitor for the test window")
			return
		}
		frame := func() rect {
			var r rect
			getWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
			return r
		}
		before := frame()
		target := placements[len(placements)-1]
		w.setFullScreen(true, target.display.ID)
		if got := frame(); got != target.info.Monitor || !w.chrome.fullScreen {
			t.Errorf("full screen = %v, want monitor %v", got, target.info.Monitor)
		}
		saved := w.chrome.savedPlace
		w.setFullScreen(true, target.display.ID)
		if w.chrome.savedPlace != saved || frame() != target.info.Monitor {
			t.Error("repeated entry changed the saved placement or the window")
		}
		w.setFullScreen(false, "")
		if got := frame(); got != before || w.chrome.fullScreen {
			t.Errorf("restored frame = %v, want %v", got, before)
		}
	})
}
