//go:build linux && !android && nsgodev

package linux

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestDevWindowFrameRestoresSizeAndFullScreen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frame")
	open := func() (*window, devWindowState) {
		t.Helper()
		t.Setenv(devWindowFrameEnv, path)
		started := make(chan devWindowState, 1)
		w := startTestWindow(t, func(cx *core.Scope) core.Node {
			frame := cx.Env().WindowFrame()
			started <- devWindowState{Width: int32(frame.Width), Height: int32(frame.Height)}
			return core.View{}
		}, Options{Title: "NSGo development window", Width: 280, Height: 220})
		select {
		case first := <-started:
			return w, first
		case <-time.After(5 * time.Second):
			t.Fatal("the window did not render")
			return nil, devWindowState{}
		}
	}
	waitFor := func(what string, check func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !check() {
			if time.Now().After(deadline) {
				t.Fatalf("%s did not happen", what)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	waitSaved := func(want devWindowState) {
		t.Helper()
		waitFor("saving window state", func() bool {
			data, err := os.ReadFile(path)
			var state devWindowState
			return err == nil && json.Unmarshal(data, &state) == nil && state == want
		})
	}
	closeWindow := func(w *window) { w.onLoop(t, w.close) }
	waitWidth := func(w *window, want int32) {
		t.Helper()
		waitFor("allocating the window", func() bool {
			var width int32
			w.onLoop(t, func() { width = gtkWidgetGetWidth(w.surface) })
			return width == want
		})
	}

	w, _ := open()
	moved := devWindowState{Width: 390, Height: 290}
	waitWidth(w, 280)
	w.onLoop(t, func() { gtkWindowSetDefaultSize(w.win, moved.Width, moved.Height) })
	waitWidth(w, moved.Width)
	waitSaved(moved)
	closeWindow(w)

	w, initial := open()
	if initial != moved {
		t.Fatalf("initial size after reopening = %dx%d, want %dx%d", initial.Width, initial.Height, moved.Width, moved.Height)
	}
	w.onLoop(t, func() { gtkWindowFullscreen(w.win) })
	full := moved
	full.FullScreen = true
	waitSaved(full)
	closeWindow(w)

	w, _ = open()
	waitFor("restoring native full screen", func() bool {
		var full bool
		w.onLoop(t, func() { full = gtkWindowIsFullscreen(w.win) != 0 })
		return full
	})
	w.onLoop(t, func() { gtkWindowUnfullscreen(w.win) })
	waitSaved(moved)
	closeWindow(w)
}
