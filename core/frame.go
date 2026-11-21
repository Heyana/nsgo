package core

import (
	"math"
	"time"
)

// frameCallback is one call waiting for the next display frame.
type frameCallback struct {
	owner *Scope
	fn    func(at time.Time)
}

// RequestAnimationFrame calls fn once, on the graph's goroutine, when the next display frame begins, with the frame's time: the display's own where the platform gives one, so the difference between two frames' times is the display's frame length rather than when the graph got to each, and the graph clock's otherwise. The time is on the graph clock's base, never precedes the previous frame's, and may lie a frame ahead of Scope.Now where it is when the frame will show. The core asks the host for that frame when the first callback waits (RootOptions.RequestFrame), and the host calls RunFrame or RunFrameAt when it begins. A host without display frames never does, so the callback waits.
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

// RunFrame runs the callbacks waiting for the display frame that begins now, on the graph's goroutine, with the graph clock's time, and asks the host for another frame for those that wait for one. A host calls it, or RunFrameAt, once for each RequestFrame it was told about.
func (cx *Scope) RunFrame() { cx.RunFrameAt(time.Time{}) }

// RunFrameAt is RunFrame for a frame whose time the display gave, on the graph clock's base: when it shows, or when it began where the platform tells no more. The zero time is the clock's now, and a time before the previous frame's is that frame's, so the callbacks never see time go back.
func (cx *Scope) RunFrameAt(at time.Time) {
	rt := cx.rt
	rt.frameRequested = false
	waiting := rt.frames
	rt.frames = nil
	if at.IsZero() {
		at = rt.clock.Now()
	}
	if at.Before(rt.frameAt) {
		at = rt.frameAt
	}
	rt.frameAt = at
	rt.frameSeq++
	rt.inFrame = true
	defer func() { rt.inFrame = false }()
	for _, cb := range waiting {
		// A callback whose owner is gone does not run, and the frame that
		// was asked for it takes it away.
		if cb.owner.disposed {
			continue
		}
		rt.run(cb.owner, func() { cb.fn(at) })
	}
}

// FrameState returns the root scope of cx's graph, the display frame sequence
// number, and whether RunFrameAt is currently executing: the core's part of
// package gamepad's per-frame snapshot, which apps reach through gamepad.Pads
// and gamepad.State.
func FrameState(cx *Scope) (root *Scope, seq uint64, inFrame bool) {
	if cx == nil || cx.rt == nil {
		return nil, 0, false
	}
	return cx.rt.root, cx.rt.frameSeq, cx.rt.inFrame
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

// Animate calls step with the eased progress from 0 to 1 once a display frame for d, on the graph's goroutine, ending with exactly 1, unless the current owner is disposed or stop is called first. Each step's changes commit as one batch, so prefer a style Transition for what a platform can animate itself.
func (cx *Scope) Animate(d time.Duration, curve Curve, step func(progress float64)) (stop func()) {
	return cx.animate(0, d, BezierEasing(curve), step)
}

// animate is Animate after delay, with any of the animation graph's easings, so Tween steps a number as Timing moves a value.
func (cx *Scope) animate(delay, d time.Duration, easing Easing, step func(progress float64)) (stop func()) {
	o := cx.current()
	start := cx.rt.clock.Now().Add(delay)
	stopped := false
	var cancel func()
	var tick func(at time.Time)
	tick = func(at time.Time) {
		if stopped {
			return
		}
		if at.Before(start) {
			cancel = o.RequestAnimationFrame(tick)
			return
		}
		x := 1.0
		if d > 0 {
			x = math.Min(float64(at.Sub(start))/float64(d), 1)
		}
		step(easing.At(x))
		if x < 1 {
			cancel = o.RequestAnimationFrame(tick)
		}
	}
	cancel = o.RequestAnimationFrame(tick)
	stop = func() {
		stopped = true
		if cancel != nil {
			cancel()
		}
	}
	o.addCleanup(stop)
	return stop
}
