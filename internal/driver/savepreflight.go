package driver

import (
	"sync"
	"time"
)

// SavePreflights holds a desktop surface's pending save decisions. The
// background worker waits for the engine, leaving the UI thread free.
type SavePreflights struct {
	pending sync.Map // request number -> chan MountItem
}

// Save finishes a chosen save on a background goroutine. The destination
// is opened only after an approval if the request needs a preflight check.
func (p *SavePreflights) Save(r SaveRequest, id uint32, path string, dispatch func(Event)) {
	if !r.Preflight {
		dispatch(r.SaveResult(id, path))
		return
	}
	reply := make(chan MountItem, 1)
	p.pending.Store(id, reply)
	defer p.pending.Delete(id)
	deadline := time.Now().Add(SavePreflightTimeout)
	timer := time.NewTimer(SavePreflightTimeout)
	defer timer.Stop()
	dispatch(Event{Kind: EventSavePreflight, Count: id, Text: path})
	failed := Event{Kind: EventFiles, Count: id, Data: path, Text: SavePreflightTimedOut}
	select {
	case answer := <-reply:
		if time.Now().Before(deadline) {
			if answer.On {
				dispatch(r.SaveResult(id, path))
				return
			}
			failed.Text = answer.Value
		}
	case <-timer.C:
	}
	dispatch(failed)
}

// Answer passes the engine's decision to the waiting save once.
func (p *SavePreflights) Answer(it MountItem) {
	if reply, ok := p.pending.LoadAndDelete(it.Count); ok {
		reply.(chan MountItem) <- it
	}
}

// SaveResult writes to path and returns the answer to the app's request.
func (r SaveRequest) SaveResult(id uint32, path string) Event {
	ev := Event{Kind: EventFiles, Count: id}
	if err := r.SaveTo(path); err != nil {
		ev.Text = err.Error()
	} else {
		ev.Finished, ev.Data = true, path
	}
	return ev
}
