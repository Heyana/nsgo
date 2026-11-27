//go:build nsgodev

package dev

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

const devReadyEnv = "NSGO_DEV_READY_ID"

// startDevState loads any state saved by an earlier run in this nsgo dev
// session and starts pushing updates back to the dev server.
func startDevState(server string) {
	if resp, err := devtools.Fetch("GET", server+"/_nsgo/state", "", nil); err == nil {
		var saved map[string]json.RawMessage
		if resp.StatusCode == 200 {
			_ = json.NewDecoder(resp.Body).Decode(&saved)
		}
		resp.Body.Close()
		if len(saved) > 0 {
			devtools.LoadState(saved)
		}
	}
	w := &devStateWriter{
		server: server,
		wake:   make(chan struct{}, 1),
	}
	go w.run()
	devtools.SetStateWriter(w.write, w.flush)
}

type devStateWriter struct {
	server string
	wake   chan struct{}

	sendMu  sync.Mutex
	mu      sync.Mutex
	pending map[string]json.RawMessage
}

func (w *devStateWriter) write(key string, raw json.RawMessage) {
	w.mu.Lock()
	if w.pending == nil {
		w.pending = map[string]json.RawMessage{}
	}
	w.pending[key] = raw
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

func (w *devStateWriter) run() {
	for range w.wake {
		time.Sleep(50 * time.Millisecond)
		w.flush()
	}
}

func (w *devStateWriter) flush() {
	w.sendMu.Lock()
	defer w.sendMu.Unlock()
	w.mu.Lock()
	if len(w.pending) == 0 {
		w.mu.Unlock()
		return
	}
	batch := w.pending
	w.pending = nil
	w.mu.Unlock()
	body, err := json.Marshal(batch)
	if err != nil {
		return
	}
	if resp, err := devtools.Fetch("POST", w.server+"/_nsgo/state", "application/json", bytes.NewReader(body)); err == nil {
		resp.Body.Close()
	}
}

var (
	handoffMu   sync.Mutex
	lastHandoff string
)

// ackHandoff flushes any pending dev state writes and acknowledges a restart
// handoff request once per ID.
func ackHandoff(server, id string) {
	if id == "" {
		return
	}
	handoffMu.Lock()
	if id == lastHandoff {
		handoffMu.Unlock()
		return
	}
	lastHandoff = id
	handoffMu.Unlock()

	devtools.FlushState()
	if resp, err := devtools.Fetch("POST", server+"/_nsgo/handoff?id="+url.QueryEscape(id), "", nil); err == nil {
		resp.Body.Close()
	}
}

// startDevReady notifies the dev server once this process applies its first
// visible view batch, allowing nsgo dev to overlap the old and new windows
// without a gap.
func startDevReady(server string) {
	id := os.Getenv(devReadyEnv)
	os.Unsetenv(devReadyEnv)
	if id == "" {
		return
	}
	var once sync.Once
	prev := driver.Applied
	driver.Applied = func(b *driver.MountBatch, took time.Duration) {
		if prev != nil {
			prev(b, took)
		}
		if batchHasView(b) {
			once.Do(func() {
				go func() {
					if resp, err := devtools.Fetch("POST", server+"/_nsgo/ready?id="+url.QueryEscape(id), "", nil); err == nil {
						resp.Body.Close()
					}
				}()
			})
		}
	}
}

func batchHasView(b *driver.MountBatch) bool {
	if b == nil {
		return false
	}
	for i := range b.Items {
		if b.Items[i].Kind == driver.ItemCreate {
			return true
		}
	}
	return false
}
