//go:build linux && !android && nsgodev

package linux

import (
	"encoding/json"
	"os"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const devWindowFrameEnv = "NSGO_DEV_WINDOW_FRAME_FILE"

type devWindowState struct {
	Width, Height int32
	FullScreen    bool
}

// devWindowFrame records GTK's default size and the compositor's
// confirmed full-screen mode in the dev tool's session file.
type devWindowFrame struct {
	path    string
	last    devWindowState
	pending chan devWindowState
	done    chan struct{}
}

func (d *devWindowFrame) restore(opts *Options) {
	d.path = os.Getenv(devWindowFrameEnv)
	os.Unsetenv(devWindowFrameEnv)
	if d.path == "" {
		return
	}
	data, err := os.ReadFile(d.path)
	var saved devWindowState
	if err == nil && json.Unmarshal(data, &saved) == nil && saved.Width > 0 && saved.Height > 0 {
		d.last = saved
		opts.Width, opts.Height = driver.Points(saved.Width), driver.Points(saved.Height)
	}
}

func (d *devWindowFrame) start(w *window) {
	if d.path == "" {
		return
	}
	d.pending, d.done = make(chan devWindowState, 1), make(chan struct{})
	go saveDevWindowStates(d.path, d.pending, d.done)
	if d.last.FullScreen {
		gtkWindowFullscreen(w.win)
	}
}

func (d *devWindowFrame) close() {
	if d.pending != nil {
		close(d.pending)
		<-d.done
		d.pending = nil
	}
}

func (d *devWindowFrame) record(w *window, env driver.Environment) {
	if d.pending == nil || w.closing || env.Minimized || gtkWidgetGetMapped(w.win) == 0 {
		return
	}
	// Unlike the actual allocation, GTK's default size stays regular
	// when the window manager gives the window a larger area.
	next := devWindowState{
		Width: int32(env.WindowFrame.Width), Height: int32(env.WindowFrame.Height),
		FullScreen: gtkWindowIsFullscreen(w.win) != 0,
	}
	if next.Width <= 0 || next.Height <= 0 || next == d.last {
		return
	}
	d.last = next
	select {
	case <-d.pending:
	default:
	}
	d.pending <- next
}

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
