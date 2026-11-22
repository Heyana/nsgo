//go:build windows && !nsgo_steam

package windows

import (
	"sync"
	"sync/atomic"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// WinSparkle invokes its callbacks off the app's main thread. The query
// waits there while the window and engine continue handling a deferred decision.
type updateQuit struct {
	w              *window
	queries        sync.Mutex
	closed         chan struct{}
	once           sync.Once
	approved       atomic.Bool
	shutdownPosted atomic.Bool
	// id and reply belong to the window thread. The high bit separates
	// updater queries from ordinary native quit requests, whose count is zero.
	id    uint32
	reply chan bool
}

const updateQuitBit uint32 = 1 << 31

func newUpdateQuit(w *window) *updateQuit {
	return &updateQuit{w: w, closed: make(chan struct{})}
}

func (q *updateQuit) ask() bool {
	q.queries.Lock()
	defer q.queries.Unlock()
	select {
	case <-q.closed:
		return false
	default:
	}
	if q.shutdownPosted.Load() {
		return false
	}
	answer := make(chan bool, 1)
	q.w.post(func() {
		select {
		case <-q.closed:
			answer <- false
			return
		default:
		}
		if q.w.destroying || q.w.engine == nil {
			answer <- false
			return
		}
		q.id = updateQuitBit | ((q.id + 1) &^ updateQuitBit)
		q.reply = answer
		q.w.engine.Dispatch(driver.Event{Kind: driver.EventQuitRequest, Count: q.id})
	})
	select {
	case approved := <-answer:
		if approved {
			q.approved.Store(true)
		}
		return approved
	case <-q.closed:
		return false
	}
}

func answerUpdateQuit(w *window, it *driver.MountItem) bool {
	if it.Count&updateQuitBit == 0 {
		return false
	}
	// Stale updater responses must never fall through to an ordinary quit,
	// including after the updater has been cleaned up.
	if q := updateQuitOwner.Load(); q != nil && q.w == w.appWindow() && it.Count == q.id && q.reply != nil {
		q.reply <- it.On
		q.reply = nil
	}
	return true
}

func (q *updateQuit) shutdown() {
	if !q.approved.Load() || q.shutdownPosted.Swap(true) {
		return
	}
	q.w.post(func() {
		select {
		case <-q.closed:
			return
		default:
			// The engine approved this quit already. Asking the window's
			// OnClosing handlers now could strand the running installer.
			q.w.finishQuit()
		}
	})
}

func (q *updateQuit) close() { q.once.Do(func() { close(q.closed) }) }
