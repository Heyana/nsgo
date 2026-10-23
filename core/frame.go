package core

import "time"

// frameCallback is one call waiting for the next display frame.
type frameCallback struct {
	owner *Scope
	fn    func(at time.Time)
}

// RequestAnimationFrame calls fn once, on the graph's goroutine, when the
// next display frame begins, with the frame's time from the graph clock.
// The core asks the host for that frame when the first callback waits
// (RootOptions.RequestFrame), and the host calls RunFrame when it begins.
// A host without display frames never does, so the callback waits.
//
// stop cancels the wait, and a callback whose owner is disposed does not
// run. A callback that wants every frame registers again from within fn:
//
//	var loop func(at time.Time)
//	loop = func(at time.Time) {
//		move(at)
//		cx.RequestAnimationFrame(loop)
//	}
//	cx.RequestAnimationFrame(loop)
func (cx *Scope) RequestAnimationFrame(fn func(at time.Time)) (stop func()) {
	if fn == nil {
		return func() {}
	}
	rt := cx.rt
	cb := &frameCallback{owner: cx.current(), fn: fn}
	rt.frames = append(rt.frames, cb)
	rt.wantFrame()
	stopped := false
	return func() {
		if stopped {
			return
		}
		stopped = true
		rt.removeFrame(cb)
	}
}

// RunFrame runs the callbacks waiting for the display frame that begins
// now, on the graph's goroutine, and asks the host for another frame for
// those that wait for one. A host calls it once for each RequestFrame it
// was told about.
func (cx *Scope) RunFrame() {
	rt := cx.rt
	rt.frameRequested = false
	waiting := rt.frames
	rt.frames = nil
	at := rt.clock.Now()
	for _, cb := range waiting {
		// A callback whose owner is gone does not run, and the frame that
		// was asked for it takes it away.
		if cb.owner.disposed {
			continue
		}
		rt.run(cb.owner, func() { cb.fn(at) })
	}
}

// wantFrame asks the host for a display frame unless one is on its way.
func (rt *runtime) wantFrame() {
	if rt.frameRequested || rt.requestFrame == nil {
		return
	}
	rt.frameRequested = true
	rt.requestFrame()
}

// removeFrame takes cb out of the callbacks waiting for the next frame.
func (rt *runtime) removeFrame(cb *frameCallback) {
	for i, c := range rt.frames {
		if c == cb {
			rt.frames = append(rt.frames[:i], rt.frames[i+1:]...)
			return
		}
	}
}
