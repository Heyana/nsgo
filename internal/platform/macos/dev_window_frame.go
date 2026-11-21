//go:build darwin && !ios && nsgodev

package macos

import (
	"encoding/json"
	"os"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

const devWindowFrameEnv = "NSGO_DEV_WINDOW_FRAME_FILE"

// devWindowFrame keeps the last normal main window frame in the dev
// command's temporary session directory. The main thread offers updates
// without waiting for the file writer.
type devWindowFrame struct {
	path    string
	pending chan driver.WindowFrame
	last    driver.WindowFrame
}

func (d *devWindowFrame) restore(f driver.WindowFrame) driver.WindowFrame {
	d.path = os.Getenv(devWindowFrameEnv)
	os.Unsetenv(devWindowFrameEnv)
	if d.path == "" {
		return f
	}
	data, err := os.ReadFile(d.path)
	if err != nil {
		return f
	}
	var saved driver.WindowFrame
	if json.Unmarshal(data, &saved) == nil && saved.Move && saved.Width > 0 && saved.Height > 0 {
		return saved
	}
	return f
}

func (d *devWindowFrame) start(s *surface) {
	if d.path == "" {
		return
	}
	d.pending = make(chan driver.WindowFrame, 1)
	s.stops = append(s.stops, func() {
		close(d.pending)
		d.pending = nil
	})
	go saveDevWindowFrames(d.path, d.pending)
	d.record(s.env)
}

func (d *devWindowFrame) record(env driver.Environment) {
	f := env.WindowFrame
	if d.pending == nil || env.FullScreen || env.Minimized || env.Maximized ||
		!f.Move || f.Width <= 0 || f.Height <= 0 || f == d.last {
		return
	}
	d.last = f
	// Only the main thread sends. Removing an older queued frame always
	// leaves room for the most recent one.
	select {
	case <-d.pending:
	default:
	}
	d.pending <- f
}

func saveDevWindowFrames(path string, frames <-chan driver.WindowFrame) {
	for f := range frames {
		data, err := json.Marshal(f)
		if err != nil {
			continue
		}
		pending := path + ".pending"
		if err := os.WriteFile(pending, data, 0o600); err == nil {
			_ = os.Rename(pending, path)
		}
	}
}
