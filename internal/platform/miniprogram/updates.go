package miniprogram

import (
	"sync"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A platform may finish its cold-start check before Go starts. The runtime
// retains this snapshot; one subscription feeds every pending app request.
type updateSnapshot struct {
	supported, checked, available, ready bool
	err                                  string
	checkedAt                            time.Time
}

type miniUpdater struct {
	mu           sync.Mutex
	snapshot     updateSnapshot
	app, version string
	dispatch     func(driver.Event)
	pending      map[uint32]*time.Timer
	closed       bool
	blocked      driver.UpdateManaged
}

func newMiniUpdater(app, version string, snapshot updateSnapshot, dispatch func(driver.Event)) *miniUpdater {
	return &miniUpdater{app: app, version: version, snapshot: snapshot, dispatch: dispatch, pending: map[uint32]*time.Timer{}}
}

func (u *miniUpdater) state() driver.UpdaterState {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.stateLocked()
}

func (u *miniUpdater) stateLocked() driver.UpdaterState {
	managed := driver.UpdateManagedUnavailable
	if u.snapshot.supported {
		managed = driver.UpdateManagedSelf
	}
	if u.blocked != 0 {
		managed = u.blocked
	}
	return driver.UpdaterState{Managed: managed, NativePreferences: true, LastCheck: u.snapshot.checkedAt}
}

func (u *miniUpdater) answer() (driver.Update, bool) {
	s := u.snapshot
	found := driver.Update{App: u.app, Current: u.version, Managed: driver.UpdateManagedSelf}
	switch {
	case u.blocked != 0:
		found.Managed = u.blocked
	case !s.supported:
		found.Managed, found.Err = driver.UpdateManagedUnavailable, "updates: this mini program has no update manager"
		if s.err != "" {
			found.Err = s.err
		}
	case s.err != "":
		found.Err = s.err
	case s.ready:
		found.Restart = true
	case s.checked && !s.available:
	default:
		return found, false
	}
	return found, true
}

func (u *miniUpdater) check(id uint32) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return
	}
	answer, terminal := u.answer()
	if !terminal {
		if _, exists := u.pending[id]; !exists {
			u.pending[id] = time.AfterFunc(30*time.Second, func() { u.expire(id) })
		}
		return
	}
	// Dispatch only queues an engine event. Keep it inside the close barrier
	// so no timeout, snapshot or immediate answer can arrive after close.
	u.dispatch(driver.Event{Kind: driver.EventUpdate, Count: id, Data: &answer})
}

func (u *miniUpdater) expire(id uint32) {
	u.mu.Lock()
	defer u.mu.Unlock()
	timer, pending := u.pending[id]
	if !pending || u.closed {
		return
	}
	delete(u.pending, id)
	timer.Stop()
	answer := driver.Update{App: u.app, Current: u.version, Managed: driver.UpdateManagedSelf, Err: "updates: platform update check timed out"}
	u.dispatch(driver.Event{Kind: driver.EventUpdate, Count: id, Data: &answer})
}

func (u *miniUpdater) publish(snapshot updateSnapshot) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.closed {
		return
	}
	u.snapshot = snapshot
	answer, terminal := u.answer()
	var ids []uint32
	if terminal {
		for id, timer := range u.pending {
			timer.Stop()
			ids = append(ids, id)
			delete(u.pending, id)
		}
	}
	state := u.stateLocked()
	u.dispatch(driver.Event{Kind: driver.EventUpdate, Data: &state})
	for _, id := range ids {
		u.dispatch(driver.Event{Kind: driver.EventUpdate, Count: id, Data: &answer})
	}
}

func (u *miniUpdater) close() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.closed = true
	for id, timer := range u.pending {
		timer.Stop()
		delete(u.pending, id)
	}
}
