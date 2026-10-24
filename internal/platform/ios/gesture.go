//go:build ios

package ios

import (
	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/internal/driver"
	"github.com/oneo-me/nsgo/internal/platform/apple"
)

// Gestures, as NSH's touch.rs gates pans: a view's pan is a recognizer of
// its own that activates once the finger passes its PanActivation and
// fails once it passes a fail offset first. The pans of the scroll views
// around it wait for it to fail, so a drag it fails still scrolls them;
// once it activates, UIKit cancels the touches of the views below,
// which ends their presses. Pinches and rotations are UIKit's
// recognizers, which recognize together.

// UIGestureRecognizerState values.
const (
	statePossible = iota
	stateBegan
	stateChanged
	stateEnded
	stateCancelled
	stateFailed
)

// panGate is the Go side of a view's pan recognizer.
type panGate struct {
	view     *view
	finger   objc.ID // the touch followed, retained
	start    apple.CGPoint
	at       driver.Point
	velocity driver.Point
	time     float64
}

// gates maps pan recognizers to their Go side.
var gates = map[objc.ID]*panGate{}

var (
	selState, selSetState = sel("state"), sel("setState:")
	selTimestamp          = sel("timestamp")
	selView               = sel("view")
	selPinchAction        = sel("nsgoPinch:")
	selRotateAction       = sel("nsgoRotate:")
	classPanGate          objc.Class
)

func registerGestures() {
	classPanGate = register("NsgoPanGate", objc.GetClass("UIGestureRecognizer"), nil, []objc.MethodDef{
		{Cmd: sel("touchesBegan:withEvent:"), Fn: gateBegan},
		{Cmd: sel("touchesMoved:withEvent:"), Fn: gateMoved},
		{Cmd: sel("touchesEnded:withEvent:"), Fn: func(self objc.ID, _ objc.SEL, touches, _ objc.ID) {
			gateLifted(self, touches, stateEnded)
		}},
		{Cmd: sel("touchesCancelled:withEvent:"), Fn: func(self objc.ID, _ objc.SEL, touches, _ objc.ID) {
			gateLifted(self, touches, stateCancelled)
		}},
		{Cmd: sel("reset"), Fn: func(self objc.ID, cmd objc.SEL) {
			apple.Super(self, cmd)
			if g := gates[self]; g != nil && g.finger != 0 {
				g.finger.Send(selRelease)
				g.finger = 0
			}
		}},
		// The pans of the scroll views around the view wait for the gate.
		{Cmd: sel("shouldBeRequiredToFailByGestureRecognizer:"), Fn: gateHoldsBack},
		// Recognizers on the same view (pinch, rotation) go on with it.
		{Cmd: sel("canPreventGestureRecognizer:"), Fn: func(self objc.ID, _ objc.SEL, other objc.ID) bool {
			return self.Send(selView) != other.Send(selView)
		}},
	})
}

// gestureMethods are the views' methods for their pinch and rotation
// recognizers, whose target and delegate they are.
func gestureMethods() []objc.MethodDef {
	return []objc.MethodDef{
		{Cmd: selPinchAction, Fn: func(self objc.ID, _ objc.SEL, r objc.ID) {
			transformed(self, r, driver.EventPinch, sel("scale"))
		}},
		{Cmd: selRotateAction, Fn: func(self objc.ID, _ objc.SEL, r objc.ID) {
			transformed(self, r, driver.EventRotation, sel("rotation"))
		}},
		{Cmd: sel("gestureRecognizer:shouldRecognizeSimultaneouslyWithGestureRecognizer:"),
			Fn: func(self objc.ID, _ objc.SEL, r, other objc.ID) bool { return other.Send(selView) == self }},
	}
}

// setGestures adds and removes v's recognizers for the gestures m follows.
func (v *view) setGestures(m driver.EventMask) {
	toggle := func(want bool, r *objc.ID, make func() objc.ID) {
		switch {
		case want && *r == 0:
			*r = make()
			v.native.Send(sel("addGestureRecognizer:"), *r)
			(*r).Send(selRelease)
		case !want && *r != 0:
			v.native.Send(sel("removeGestureRecognizer:"), *r)
			delete(gates, *r)
			*r = 0
		}
	}
	newRecognizer := func(class objc.Class, action objc.SEL) objc.ID {
		target, a := v.native, action
		if action == 0 {
			target = 0
		}
		return objc.ID(class).Send(selAlloc).Send(sel("initWithTarget:action:"), target, a)
	}
	toggle(m.Has(driver.EventPan), &v.pan, func() objc.ID {
		r := newRecognizer(classPanGate, 0)
		// The views below keep their touches until the pan activates.
		r.Send(sel("setDelaysTouchesEnded:"), false)
		gates[r] = &panGate{view: v}
		return r
	})
	toggle(m.Has(driver.EventPinch), &v.pinch, func() objc.ID {
		r := newRecognizer(objc.GetClass("UIPinchGestureRecognizer"), selPinchAction)
		r.Send(selSetDeleg, v.native)
		return r
	})
	toggle(m.Has(driver.EventRotation), &v.rotation, func() objc.ID {
		r := newRecognizer(objc.GetClass("UIRotationGestureRecognizer"), selRotateAction)
		r.Send(selSetDeleg, v.native)
		return r
	})
}

func gateBegan(self objc.ID, _ objc.SEL, touches, _ objc.ID) {
	g := gates[self]
	if g == nil || g.finger != 0 {
		return
	}
	touch := touches.Send(selAnyObject)
	g.finger = touch.Send(sel("retain"))
	g.start = objc.Send[apple.CGPoint](touch, selLocationInView, objc.ID(0))
	g.at, g.velocity = driver.Point{}, driver.Point{}
	g.time = objc.Send[float64](touch, selTimestamp)
}

func gateMoved(self objc.ID, _ objc.SEL, _, _ objc.ID) {
	g := gates[self]
	if g == nil || g.finger == 0 {
		return
	}
	state := objc.Send[int](self, selState)
	if state != statePossible && state != stateBegan && state != stateChanged {
		return
	}
	at := objc.Send[apple.CGPoint](g.finger, selLocationInView, objc.ID(0))
	d := driver.Point{X: driver.Points(at.X - g.start.X), Y: driver.Points(at.Y - g.start.Y)}
	now := objc.Send[float64](g.finger, selTimestamp)
	if dt := now - g.time; dt > 0 {
		inst := driver.Point{X: (d.X - g.at.X) / driver.Points(dt), Y: (d.Y - g.at.Y) / driver.Points(dt)}
		// Smoothed, so one uneven sample does not throw a flick.
		g.velocity = driver.Point{X: inst.X*0.7 + g.velocity.X*0.3, Y: inst.Y*0.7 + g.velocity.Y*0.3}
	}
	g.at, g.time = d, now
	phase := driver.GestureChanged
	if state == statePossible {
		switch g.view.activation.Decide(d) {
		case 1:
			phase = driver.GestureBegan
			self.Send(selSetState, stateBegan)
		case -1:
			self.Send(selSetState, stateFailed)
			return
		default:
			return
		}
	} else {
		self.Send(selSetState, stateChanged)
	}
	g.view.report(driver.Event{Kind: driver.EventPan, Phase: phase, Point: d, Velocity: g.velocity})
}

func gateLifted(self, touches objc.ID, how int) {
	g := gates[self]
	if g == nil || g.finger == 0 {
		return
	}
	followed := false
	apple.ForEach(touches, func(t objc.ID) { followed = followed || t == g.finger })
	if !followed {
		return
	}
	state := objc.Send[int](self, selState)
	if state != stateBegan && state != stateChanged {
		self.Send(selSetState, stateFailed)
		return
	}
	self.Send(selSetState, how)
	phase, velocity := driver.GestureEnded, g.velocity
	if how == stateCancelled {
		phase = driver.GestureCancelled
	}
	// A drag that stopped before the finger lifted is no flick.
	if objc.Send[float64](g.finger, selTimestamp)-g.time > 0.05 {
		velocity = driver.Point{}
	}
	g.view.report(driver.Event{Kind: driver.EventPan, Phase: phase, Point: g.at, Velocity: velocity})
}

// gateHoldsBack reports whether other is the pan of a scroll view around
// the gate's view.
func gateHoldsBack(self objc.ID, _ objc.SEL, other objc.ID) bool {
	view, scroll := self.Send(selView), other.Send(selView)
	if view == 0 || scroll == 0 || !objc.Send[bool](scroll, sel("isKindOfClass:"), objc.GetClass("UIScrollView")) {
		return false
	}
	return scroll.Send(sel("panGestureRecognizer")) == other && objc.Send[bool](view, sel("isDescendantOfView:"), scroll)
}

// transformed reports a pinch's scale or a rotation's angle (value)
// as its recognizer r moves.
func transformed(self, r objc.ID, kind driver.EventKind, value objc.SEL) {
	v := views[self]
	if v == nil {
		return
	}
	var phase driver.GesturePhase
	switch objc.Send[int](r, selState) {
	case stateBegan:
		phase = driver.GestureBegan
	case stateChanged:
		phase = driver.GestureChanged
	case stateEnded:
		phase = driver.GestureEnded
	case stateCancelled, stateFailed:
		phase = driver.GestureCancelled
	default:
		return
	}
	v.report(driver.Event{Kind: kind, Phase: phase, Value: float32(objc.Send[float64](r, value)),
		Velocity: driver.Point{X: driver.Points(objc.Send[float64](r, sel("velocity")))}})
}
