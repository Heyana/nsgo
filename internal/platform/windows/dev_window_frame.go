//go:build windows && nsgodev

package windows

import (
	"encoding/json"
	"os"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const devWindowFrameEnv = "NSGO_DEV_WINDOW_FRAME_FILE"

type devWindowState struct {
	Frame      driver.WindowFrame
	FullScreen bool
}

// devWindowFrame keeps the last regular frame and the full-screen mode
// for the main view. Its worker writes the dev tool's session file.
type devWindowFrame struct {
	path    string
	last    devWindowState
	pending chan devWindowState
	done    chan struct{}
	queued  bool
}

func (d *devWindowFrame) restore(f driver.WindowFrame) (driver.WindowFrame, bool) {
	d.path = os.Getenv(devWindowFrameEnv)
	os.Unsetenv(devWindowFrameEnv)
	if d.path != "" {
		data, err := os.ReadFile(d.path)
		var saved devWindowState
		if err == nil && json.Unmarshal(data, &saved) == nil && validDevWindowFrame(saved.Frame) {
			d.last = saved
			return saved.Frame, saved.FullScreen
		}
	}
	return f, false
}

func (d *devWindowFrame) start(w *window) {
	if d.path == "" {
		return
	}
	d.pending, d.done = make(chan devWindowState, 1), make(chan struct{})
	go saveDevWindowStates(d.path, d.pending, d.done)
	d.record(w)
}

func (d *devWindowFrame) close() {
	if d.pending != nil {
		close(d.pending)
		<-d.done
		d.pending = nil
	}
}

// Wait until the window procedure finishes its move or mode transition.
// Full-screen toggles can send size messages before setting their final frame.
func (d *devWindowFrame) changed(w *window) {
	if d.pending == nil || d.queued {
		return
	}
	d.queued = true
	w.post(func() {
		d.queued = false
		d.record(w)
	})
}

func (d *devWindowFrame) record(w *window) {
	if w.hwnd == 0 || isIconicWindow(w.hwnd) {
		return
	}
	next := d.last
	next.FullScreen = w.chrome.fullScreen
	if !next.FullScreen && !w.zoomed() {
		next.Frame = w.env.WindowFrame
	}
	if !validDevWindowFrame(next.Frame) || next == d.last {
		return
	}
	d.last = next
	select {
	case <-d.pending:
	default:
	}
	d.pending <- next
}

func validDevWindowFrame(f driver.WindowFrame) bool { return f.Move && f.Width > 0 && f.Height > 0 }

func saveDevWindowStates(path string, states <-chan devWindowState, done chan<- struct{}) {
	defer close(done)
	for state := range states {
		data, err := json.Marshal(state)
		if err != nil {
			continue
		}
		pending := path + ".pending"
		if err := os.WriteFile(pending, data, 0o600); err == nil {
			_ = os.Rename(pending, path)
		}
	}
}
