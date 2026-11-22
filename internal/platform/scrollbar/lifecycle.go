package scrollbar

import "github.com/nsgo-dev/nsgo/internal/driver"

// SettleHost applies a snap through the backend's scrolling and animation paths. The callbacks run synchronously in the order the scroll reports.
type SettleHost struct {
	Scroll  func(driver.Point)
	Animate func(driver.Point)
	Event   func(driver.EventKind)
	Rest    func()
}

// Settle snaps a released view or reports its rest. The backend owns the animation curve and reports momentum's end when that animation finishes.
func (h SettleHost) Settle(c *driver.ScrollConfig, frame, content driver.Size, offset driver.Point, velocity float32, reduceMotion bool) {
	to, ok := Settle(c, frame, content, offset, velocity)
	if !ok || to == offset {
		h.Rest()
		return
	}
	h.Event(driver.EventMomentumStart)
	if reduceMotion {
		h.Scroll(to)
		h.Event(driver.EventMomentumEnd)
		h.Rest()
		return
	}
	h.Animate(to)
}

// Rest reports a scroll's end before starting the snapping diff clamps that follow it. The clock runs only while their animator has work.
func Rest(animator *driver.Animator, id driver.NodeID, report, startClock func()) {
	report()
	animator.Settle(id)
	if animator.Running() {
		startClock()
	}
}

// EventSink queues scroll reports either on their own or with a frame already requested by the app.
type EventSink interface {
	Dispatch(driver.Event)
	DispatchBeforeFrame(driver.Event)
}

// Report shares the next display frame's engine pass when one is pending.
func Report(sink EventSink, event driver.Event, frameWanted bool) {
	if frameWanted {
		sink.DispatchBeforeFrame(event)
	} else {
		sink.Dispatch(event)
	}
}
