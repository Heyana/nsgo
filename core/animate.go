package core

import (
	"math"
	"time"
)

// Transform moves, scales and rotates a view without changing its
// layout, around its center: translate, then rotate, then scale. The
// zero value changes nothing.
type Transform struct {
	TranslateX, TranslateY Points
	// Scale scales both axes; ScaleX and ScaleY one each. 0 is unset (1).
	Scale, ScaleX, ScaleY float32
	// Rotate is in degrees, clockwise.
	Rotate float32
}

// Scales returns the x and y scale factors.
func (t Transform) Scales() (x, y float32) {
	x, y = 1, 1
	if t.Scale != 0 {
		x, y = t.Scale, t.Scale
	}
	if t.ScaleX != 0 {
		x = t.ScaleX
	}
	if t.ScaleY != 0 {
		y = t.ScaleY
	}
	return x, y
}

// Transition animates a view's look when its style changes: the platform
// runs it on the main thread, with no work for the engine per frame.
type Transition struct {
	Duration time.Duration
	Delay    time.Duration
	Easing   Curve
	// Properties lists what animates; every property when 0.
	Properties TransitionProperty
}

// TransitionProperty is a set of animatable properties.
type TransitionProperty uint8

const (
	TransitionOpacity TransitionProperty = 1 << iota
	TransitionBackground
	TransitionTransform
	TransitionBorder
	TransitionShadow
)

// Curve is an easing curve: CSS's cubic Bézier from (0,0) to (1,1)
// through two control points. Its zero value is linear.
type Curve struct{ X1, Y1, X2, Y2 float32 }

// The standard curves, as CSS names them.
var (
	Linear    = Curve{}
	Ease      = Curve{0.25, 0.1, 0.25, 1}
	EaseIn    = Curve{0.42, 0, 1, 1}
	EaseOut   = Curve{0, 0, 0.58, 1}
	EaseInOut = Curve{0.42, 0, 0.58, 1}
)

// At returns the curve's progress at time fraction x from 0 to 1.
func (c Curve) At(x float64) float64 {
	if c == (Curve{}) || x <= 0 || x >= 1 {
		return math.Min(math.Max(x, 0), 1)
	}
	x1, y1, x2, y2 := float64(c.X1), float64(c.Y1), float64(c.X2), float64(c.Y2)
	bez := func(t, p1, p2 float64) float64 {
		u := 1 - t
		return 3*u*u*t*p1 + 3*u*t*t*p2 + t*t*t
	}
	// Find t with bez(t) == x on the x axis by bisection, then map to y.
	lo, hi := 0.0, 1.0
	for range 40 {
		mid := (lo + hi) / 2
		if bez(mid, x1, x2) < x {
			lo = mid
		} else {
			hi = mid
		}
	}
	return bez((lo+hi)/2, y1, y2)
}

// FrameInterval is a frame at 60 Hz: what an app assumes where the
// display's own pace is unknown.
const FrameInterval = time.Second / 60

// Animate calls step with the eased progress from 0 to 1 once a display
// frame for d, on the graph's goroutine, ending with exactly 1, unless the
// current owner is disposed or stop is called first. Each step's changes
// commit as one batch, so prefer a style Transition for what a platform
// can animate itself.
func (cx *Scope) Animate(d time.Duration, curve Curve, step func(progress float64)) (stop func()) {
	o := cx.current()
	start := cx.rt.clock.Now()
	stopped := false
	var cancel func()
	var tick func(at time.Time)
	tick = func(at time.Time) {
		if stopped {
			return
		}
		x := 1.0
		if d > 0 {
			x = math.Min(float64(at.Sub(start))/float64(d), 1)
		}
		step(curve.At(x))
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
