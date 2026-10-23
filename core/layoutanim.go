package core

import (
	"math"
	"time"
)

// LayoutAnimation animates the frame changes, insertions and removals of
// one commit on the platform, as React Native's LayoutAnimation does.
// Create animates the views the commit inserts, Update those whose frame
// changes, Delete those it removes; a nil phase leaves its changes
// instant.
type LayoutAnimation struct {
	Create, Update, Delete *LayoutTransition
}

// LayoutTransition is how one kind of change animates.
type LayoutTransition struct {
	Curve LayoutCurve
	// Damping is LayoutSpring's damping ratio, from 0 to 1.
	Damping float32
	// Property is what appearing and disappearing views animate.
	Property LayoutProperty
	Duration time.Duration
	Delay    time.Duration
}

// LayoutCurve is how a layout change progresses.
type LayoutCurve uint8

const (
	LayoutLinear LayoutCurve = iota
	LayoutEaseIn
	LayoutEaseOut
	LayoutEaseInEaseOut
	// LayoutSpring springs with Damping, settling within the duration.
	LayoutSpring
)

// LayoutProperty is what appearing and disappearing views animate.
type LayoutProperty uint8

const (
	// LayoutOpacity fades them.
	LayoutOpacity LayoutProperty = iota
	// LayoutScale grows and shrinks them from their center.
	LayoutScale
)

func allLayout(tr LayoutTransition) LayoutAnimation {
	return LayoutAnimation{Create: &tr, Update: &tr, Delete: &tr}
}

// React Native's presets.
var (
	// LayoutEaseInEaseOutPreset eases every change in and out over 300 ms.
	LayoutEaseInEaseOutPreset = allLayout(LayoutTransition{Curve: LayoutEaseInEaseOut, Duration: 300 * time.Millisecond})
	// LayoutLinearPreset moves every change at constant speed over 500 ms.
	LayoutLinearPreset = allLayout(LayoutTransition{Curve: LayoutLinear, Duration: 500 * time.Millisecond})
	// LayoutSpringPreset springs frames (damping 0.4) over 700 ms and fades
	// views linearly.
	LayoutSpringPreset = LayoutAnimation{
		Create: &LayoutTransition{Curve: LayoutLinear, Duration: 700 * time.Millisecond},
		Update: &LayoutTransition{Curve: LayoutSpring, Damping: 0.4, Duration: 700 * time.Millisecond},
		Delete: &LayoutTransition{Curve: LayoutLinear, Duration: 700 * time.Millisecond},
	}
)

// AnimateLayout animates the changes of the next commit that has any
// with a: views move and resize, and appear and go, on the platform.
func (cx *Scope) AnimateLayout(a LayoutAnimation) {
	t := cx.rt.tree
	t.layoutAnim = &a
	t.changed = true
}

// At returns the transition's progress at elapsed time since the change,
// and whether it is done. A spring may overshoot 1.
func (tr *LayoutTransition) At(elapsed time.Duration) (float64, bool) {
	x := 1.0
	if tr.Duration > 0 {
		x = float64(elapsed-tr.Delay) / float64(tr.Duration)
	}
	if x <= 0 {
		return 0, false
	}
	if x >= 1 {
		return 1, true
	}
	switch tr.Curve {
	case LayoutEaseIn:
		return EaseIn.At(x), false
	case LayoutEaseOut:
		return EaseOut.At(x), false
	case LayoutEaseInEaseOut:
		return EaseInOut.At(x), false
	case LayoutSpring:
		return layoutSpring(float64(tr.Damping), x), false
	}
	return x, false
}

// End is when the transition is done.
func (tr *LayoutTransition) End() time.Duration { return tr.Delay + tr.Duration }

// layoutSpring is a damped spring from 0 to 1 at x, the fraction of the
// duration, stiff enough to settle by the end, as UIKit fits
// usingSpringWithDamping into a duration (NSH's transition.rs).
func layoutSpring(damping, x float64) float64 {
	const settle = 6.907755 // ln(1000): the envelope falls to 1/1000
	zeta := min(max(damping, 0.05), 1)
	if zeta > 0.999 {
		w := 9.233
		return 1 - (1+w*x)*math.Exp(-w*x)
	}
	w := settle / zeta
	wd := w * math.Sqrt(1-zeta*zeta)
	env := math.Exp(-zeta * w * x)
	return 1 - env*(math.Cos(wd*x)+zeta*w/wd*math.Sin(wd*x))
}

// LerpBounds interpolates a frame from a to b at progress p, never to a
// negative size.
func LerpBounds(a, b Bounds, p float64) Bounds {
	l := func(x, y Points) Points { return x + Points(float64(y-x)*p) }
	return Bounds{
		Origin: Point{l(a.Origin.X, b.Origin.X), l(a.Origin.Y, b.Origin.Y)},
		Size:   Size{max(l(a.Size.Width, b.Size.Width), 0), max(l(a.Size.Height, b.Size.Height), 0)},
	}
}
