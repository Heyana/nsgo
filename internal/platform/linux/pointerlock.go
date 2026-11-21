//go:build linux && !android

package linux

import "github.com/nsgo-dev/nsgo/internal/driver"

type pointerCapture interface {
	close()
}

var lockedPointer struct {
	owner   *window
	token   driver.PointerLock
	capture pointerCapture
}

func (w *window) canLockPointer() bool {
	return xPointerDisplay() != nil || waylandPointerDisplay() != nil
}

func (w *window) canHoldPointer() bool {
	root := w.rootWindow()
	return root != 0 && gtkWindowIsActive(root) != 0 && gtkWidgetGetMapped(w.surface) != 0
}

func (w *window) setPointerLock(it *driver.MountItem) {
	token := driver.PointerLock{ID: it.ID, Count: it.Count}
	if !it.On {
		if lockedPointer.owner == w && lockedPointer.token == token {
			w.releasePointer(false)
		}
		return
	}
	if !w.canHoldPointer() || w.closing {
		w.engine.Dispatch(token.Unlocked())
		return
	}
	if old := lockedPointer.owner; old != nil {
		if old == w {
			lockedPointer.token = token
			return
		}
		old.releasePointer(true)
	}
	var capture pointerCapture
	switch {
	case xPointerDisplay() != nil:
		capture = w.lockXPointer()
	case waylandPointerDisplay() != nil:
		capture = w.lockWaylandPointer()
	}
	if capture == nil {
		w.engine.Dispatch(token.Unlocked())
		return
	}
	lockedPointer.owner, lockedPointer.token, lockedPointer.capture = w, token, capture
}

func (w *window) releasePointer(report bool) {
	if lockedPointer.owner != w {
		return
	}
	token, capture := lockedPointer.token, lockedPointer.capture
	lockedPointer.owner, lockedPointer.capture = nil, nil
	capture.close()
	if report && w.engine != nil {
		w.engine.Dispatch(token.Unlocked())
	}
}

func (w *window) pointerMotion(dx, dy float64) {
	if lockedPointer.owner == w && (dx != 0 || dy != 0) {
		w.engine.Dispatch(lockedPointer.token.Motion(dx, dy))
	}
}
