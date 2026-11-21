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

// FrameRateRange requests a display rate in Hz for a surface. A zero
// range follows the display. Use Preferred alone to request one rate.
// Within a range, omitted bounds take Preferred's value, and an omitted
// Preferred uses Maximum, or Minimum if Maximum is also omitted.
type FrameRateRange struct {
	Minimum, Maximum, Preferred float32
}

func (r FrameRateRange) normalized() (FrameRateRange, bool) {
	if r == (FrameRateRange{}) {
		return r, true
	}
	if r.Preferred == 0 {
		r.Preferred = max(r.Minimum, r.Maximum)
	}
	if r.Minimum == 0 {
		r.Minimum = r.Preferred
	}
	if r.Maximum == 0 {
		r.Maximum = r.Preferred
	}
	if r.Minimum <= 0 || r.Minimum > r.Preferred || r.Preferred > r.Maximum ||
		math.IsInf(float64(r.Maximum), 0) || math.IsNaN(float64(r.Minimum+r.Maximum+r.Preferred)) {
		return FrameRateRange{}, false
	}
	return r, true
}

// SetPreferredFrameRate changes the rate wanted by RequestAnimationFrame
// on the current surface. A zero range restores its usual display rate.
// The platform may deliver a slower rate. An invalid range is ignored.
func (cx *Scope) SetPreferredFrameRate(rate FrameRateRange) {
	rt := cx.rt
	rate, ok := rate.normalized()
	if !ok {
		return
	}
	id, q := cx.current().frameQueue()
	if q.rate == rate {
		return
	}
	q.rate = rate
	q.rateChanged = true
	if rt.setFrameRate != nil {
		rt.setFrameRate(id, rate)
	}
}

type frameQueue struct {
	waiting     []*frameCallback
	requested   bool
	rate        FrameRateRange
	rateChanged bool
	at          time.Time // last display timestamp, which the next never precedes
	delivered   time.Time // last callback delivery at this preference
}

func (cx *Scope) frameQueue() (uint32, *frameQueue) {
	if t := cx.tree(); t != nil {
		return t.id, t.frameQueue()
	}
	return 0, &cx.rt.firstFrames
}

func (t *Tree) frameQueue() *frameQueue {
	if t.id == 0 {
		return &t.rt.firstFrames
	}
	return &t.animationFrames
}

// PreferredFrameRate gives the host the rate requested for this surface.
func (t *Tree) PreferredFrameRate() FrameRateRange { return t.frameQueue().rate }

// FramePending reports whether the surface awaits its first or next
// display frame, so an attaching host can start its clock.
func (t *Tree) FramePending() bool { return t.frameQueue().requested }

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
	owner := cx.current()
	id, q := owner.frameQueue()
	cb := &frameCallback{owner: owner, fn: fn}
	q.waiting = append(q.waiting, cb)
	cx.rt.wantFrame(id, q)
	return func() {
		for i, c := range q.waiting {
			if c == cb {
				q.waiting = append(q.waiting[:i], q.waiting[i+1:]...)
				return
			}
		}
	}
}

// RunFrame runs the callbacks waiting for the display frame that begins
// now. A host with one clock calls it once per RootOptions.RequestFrame.
func (cx *Scope) RunFrame() { cx.RunFrameAt(time.Time{}) }

// RunFrameAt is RunFrame for a frame stamped by the display on the graph
// clock's base. The zero time uses the graph clock's now.
func (cx *Scope) RunFrameAt(at time.Time) {
	rt := cx.rt
	rt.frameRequested = false
	at = rt.frameTime(at)
	first := rt.takeFrame(0, &rt.firstFrames, at)
	if len(rt.surfaces) == 0 {
		rt.runFrames(first)
		return
	}
	// Take the queues before calling any callbacks, so a callback that
	// requests a second surface's frame waits for the next tick as well.
	runs := make([]frameRun, 0, len(rt.surfaces))
	for _, t := range rt.surfaces {
		runs = append(runs, rt.takeFrame(t.id, t.frameQueue(), at))
	}
	rt.runFrames(append([]frameRun{first}, runs...)...)
}

// RunFrameAt dispatches a frame to this tree only. A host with independent
// clocks calls it for the surface named by RootOptions.RequestSurfaceFrame.
func (t *Tree) RunFrameAt(at time.Time) {
	t.rt.runFrames(t.rt.takeFrame(t.id, t.frameQueue(), t.rt.frameTime(at)))
}

func (rt *runtime) frameTime(at time.Time) time.Time {
	if at.IsZero() {
		return rt.clock.Now()
	}
	return at
}

type frameRun struct {
	at      time.Time
	waiting []*frameCallback
}

func (rt *runtime) takeFrame(id uint32, q *frameQueue, at time.Time) frameRun {
	q.requested = false
	if len(q.waiting) == 0 {
		return frameRun{}
	}
	if at.Before(q.at) {
		at = q.at
	}
	q.at = at
	if !q.due(at) {
		rt.wantFrame(id, q)
		return frameRun{}
	}
	q.rateChanged = false
	q.delivered = at
	run := frameRun{at: at, waiting: q.waiting}
	q.waiting = nil
	return run
}

func (q *frameQueue) due(at time.Time) bool {
	if q.delivered.IsZero() {
		return true
	}
	const tolerance = 500 * time.Microsecond
	gap := at.Sub(q.delivered)
	if q.rateChanged {
		// A retuned Apple link can predict the same presentation time
		// twice. Take the next distinct frame at the new rate.
		return gap >= tolerance
	}
	if q.rate.Preferred == 0 {
		return true
	}
	// Anchor to the last delivered display time. A display at 59.94 Hz
	// or with small timestamp errors does not accumulate a phase error.
	interval := float64(time.Second) / float64(q.rate.Preferred)
	return float64(gap) >= interval-min(interval/8, float64(tolerance))
}

// runFrames runs the callbacks taken for one display frame. Every callback in it sees the same frame sequence number, so package gamepad's snapshot holds for the whole pass (FrameState).
func (rt *runtime) runFrames(runs ...frameRun) {
	rt.frameSeq++
	rt.inFrame = true
	defer func() { rt.inFrame = false }()
	for _, run := range runs {
		for _, cb := range run.waiting {
			// A callback whose owner is gone does not run, and the frame that was asked for it takes it away.
			if !cb.owner.disposed {
				rt.run(cb.owner, func() { cb.fn(run.at) })
			}
		}
	}
}

// FrameState returns the root scope of cx's graph, the display frame sequence number, and whether a display frame's callbacks are currently running: the core's part of package gamepad's per-frame snapshot, which apps reach through gamepad.Pads and gamepad.State.
func FrameState(cx *Scope) (root *Scope, seq uint64, inFrame bool) {
	if cx == nil || cx.rt == nil {
		return nil, 0, false
	}
	return cx.rt.root, cx.rt.frameSeq, cx.rt.inFrame
}

func (rt *runtime) wantFrame(id uint32, q *frameQueue) {
	if rt.requestSurfaceFrame != nil {
		if !q.requested {
			q.requested = true
			rt.requestSurfaceFrame(id)
		}
		return
	}
	if !rt.frameRequested && rt.requestFrame != nil {
		rt.frameRequested = true
		rt.requestFrame()
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
