package driver

import (
	"sync"
	"time"
)

// SavePreflights holds a desktop surface's pending save decisions. The
// background worker waits for the engine, leaving the UI thread free.
type SavePreflights struct {
	pending sync.Map // savePreflightKey -> chan MountItem
}

type savePreflightKey struct{ id, attempt uint32 }

// Save finishes a chosen save on a background goroutine. The destination
// is opened only after an approval if the request needs a preflight check.
func (p *SavePreflights) Save(r SaveRequest, id uint32, path string, dispatch func(Event)) {
	if r.Preflight {
		if approved, reason := p.Check(id, 0, path, dispatch); !approved {
			dispatch(Event{Kind: EventFiles, Count: id, Data: path, Text: reason})
			return
		}
	}
	dispatch(SaveResult(r, id, path))
}

// Check asks the engine about one candidate on a background goroutine. A
// dialog that can retry numbers its attempts from one. A one-shot chooser
// uses zero.
func (p *SavePreflights) Check(id, attempt uint32, path string, dispatch func(Event)) (approved bool, reason string) {
	key := savePreflightKey{id, attempt}
	reply := make(chan MountItem, 1)
	p.pending.Store(key, reply)
	defer p.pending.Delete(key)
	deadline := time.Now().Add(SavePreflightTimeout)
	timer := time.NewTimer(SavePreflightTimeout)
	defer timer.Stop()
	event := Event{Kind: EventSavePreflight, Count: id, Text: path}
	if attempt != 0 {
		event.Data = attempt
	}
	dispatch(event)
	select {
	case answer := <-reply:
		if time.Now().Before(deadline) {
			return answer.On, answer.Value
		}
	case <-timer.C:
	}
	return false, SavePreflightTimedOut
}

// Answer passes the engine's decision to the matching attempt once.
func (p *SavePreflights) Answer(it MountItem) {
	attempt, _ := it.Data.(uint32)
	if reply, ok := p.pending.LoadAndDelete(savePreflightKey{it.Count, attempt}); ok {
		reply.(chan MountItem) <- it
	}
}

// SaveResult writes what r saves to path and returns the answer to the app's request. It is a function, not a method, so core's SaveRequest alias does not reach driver.Event.
func SaveResult(r SaveRequest, id uint32, path string) Event {
	ev := Event{Kind: EventFiles, Count: id}
	if err := r.SaveTo(path); err != nil {
		ev.Text = err.Error()
	} else {
		ev.Finished, ev.Data = true, path
	}
	return ev
}
