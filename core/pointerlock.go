package core

import (
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type pointerLock struct {
	node  *node
	token driver.PointerLock
}

// LockPointer asks the platform to confine and hide the pointer until
// the returned function runs, Escape is pressed, the window loses focus
// or the view disappears. OnPointerDelta reports relative motion while
// the view owns the lock. In a browser, ask from a user gesture.
func (r *Ref) LockPointer() (unlock func()) {
	if !r.placed() || !r.t.reported.CanLockPointer {
		return func() {}
	}
	t := r.t
	t.rt.check()
	if t.pointerLock != nil {
		t.releasePointer(t.pointerLock, false)
	}
	t.nextPointerLock++
	lock := &pointerLock{node: r.n, token: driver.PointerLock{ID: r.n.id, Count: t.nextPointerLock}}
	t.pointerLock = lock
	t.setPointerLockState(lock, true)
	t.pointerCommand(lock.token, true)
	return func() {
		t.rt.check()
		t.releasePointer(lock, false)
	}
}

func (t *Tree) releasePointer(lock *pointerLock, notify bool) {
	if t.pointerLock == lock {
		t.pointerLock = nil
		t.setPointerLockState(lock, false)
		if i := slices.IndexFunc(t.commands, func(it driver.MountItem) bool {
			return it.Kind == driver.ItemPointerLock && it.On && lock.token.Matches(it.ID, it.Count)
		}); i >= 0 {
			t.commands = slices.Delete(t.commands, i, i+1)
		} else {
			t.pointerCommand(lock.token, false)
		}
		if notify && !lock.node.deleted && lock.node.h.pointerUnlock != nil {
			t.within(lock.node.owner, lock.node.h.pointerUnlock)
		}
	}
}

func (t *Tree) setPointerLockState(lock *pointerLock, locked bool) {
	if lock.node.pointerLocked != nil {
		t.within(lock.node.owner, func() { lock.node.pointerLocked.Set(locked) })
	}
}

func (t *Tree) pointerCommand(lock driver.PointerLock, on bool) {
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemPointerLock, ID: lock.ID, Count: lock.Count, On: on})
	t.changed = true
}

func (t *Tree) checkPointerLock() {
	lock := t.pointerLock
	if lock == nil {
		return
	}
	if !t.reported.CanLockPointer || lock.node.deleted || t.detached(lock.node) {
		t.releasePointer(lock, !lock.node.deleted)
		return
	}
	for n := lock.node; n != nil; n = n.parent {
		if n.style.Display == DisplayNone || n.style.Visibility == VisibilityHidden || n.hidden ||
			n.attach != nil && n.attach.cut {
			t.releasePointer(lock, true)
			return
		}
	}
}

func (t *Tree) pointerLockEvent(ev driver.Event) {
	lock := t.pointerLock
	if lock == nil || !lock.token.Matches(ev.ID, ev.Count) {
		return
	}
	if ev.Kind == driver.EventPointerUnlocked {
		t.releasePointer(lock, true)
		return
	}
	if delta, ok := ev.Data.(driver.PointerDelta); ok && lock.node.h.pointerDelta != nil && !lock.node.deleted {
		t.within(lock.node.owner, func() { lock.node.h.pointerDelta(delta.X, delta.Y) })
	}
}
