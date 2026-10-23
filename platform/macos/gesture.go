//go:build darwin && !ios

package macos

import (
	"math"

	"github.com/ebitengine/purego/objc"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/platform/internal/apple"
)

// Mouse drags pan the nearest view around the press that pans, once its
// PanActivation activates; the press is then the pan's. Trackpad pinches
// and rotations go to the nearest view under the pointer that follows
// them.

// panTrack is a drag that may become a view's pan.
type panTrack struct {
	view     *view
	pressed  *view // the view the press went to, if it is pressable
	start    apple.CGPoint
	active   bool
	at       core.Point // the translation last seen
	velocity core.Point
	time     float64 // when it was last seen, in seconds
}

// gestureTrack is a pinch or rotation in progress.
type gestureTrack struct {
	view     *view
	value    float64
	velocity float64
	time     float64
}

var selTimestamp = sel("timestamp")

// gestureView returns the nearest view from native up that follows
// gestures of kind.
func gestureView(native objc.ID, kind core.EventKind) *view {
	for n := native; n != 0; n = n.Send(selSuperview) {
		if v := views[n]; v != nil && v.native == n && v.events.Has(kind) {
			return v
		}
	}
	return nil
}

// startPan follows a mouse down on native for the pan of the view
// around it, if one pans.
func (s *surface) startPan(native, event objc.ID) {
	s.pan = nil
	v := gestureView(native, core.EventPan)
	if v == nil {
		return
	}
	t := &panTrack{view: v, start: objc.Send[apple.CGPoint](event, selLocationInWindow),
		time: objc.Send[float64](event, selTimestamp)}
	if p := views[native]; p != nil && p.pressable() {
		t.pressed = p
	}
	s.pan = t
}

func mouseDragged(self objc.ID, cmd objc.SEL, event objc.ID) {
	v := views[self]
	if v == nil || v.surface.pan == nil {
		apple.Super(self, cmd, event)
		return
	}
	t := v.surface.pan
	at := objc.Send[apple.CGPoint](event, selLocationInWindow)
	// Window points go up.
	d := core.Point{X: core.Points(at.X - t.start.X), Y: core.Points(t.start.Y - at.Y)}
	now := objc.Send[float64](event, selTimestamp)
	if dt := now - t.time; dt > 0 {
		inst := core.Point{X: (d.X - t.at.X) / core.Points(dt), Y: (d.Y - t.at.Y) / core.Points(dt)}
		// Smoothed, so one uneven sample does not throw a flick.
		t.velocity = core.Point{X: inst.X*0.7 + t.velocity.X*0.3, Y: inst.Y*0.7 + t.velocity.Y*0.3}
	}
	t.at, t.time = d, now
	phase := core.GestureChanged
	if !t.active {
		switch t.view.activation.Decide(d) {
		case 1:
			t.active, phase = true, core.GestureBegan
			if p := t.pressed; p != nil && p.pressing.Swap(false) {
				p.longFired.Store(true)
				p.dispatch(core.EventPressUp)
			}
		case -1:
			v.surface.pan = nil
			return
		default:
			return
		}
	}
	v.surface.panned(t.view, d)
	t.view.report(core.Event{Kind: core.EventPan, Phase: phase, Point: d, Velocity: t.velocity})
}

// endPan ends the pan a mouse up lifts, reporting whether one was active.
func (s *surface) endPan(event objc.ID) bool {
	t := s.pan
	s.pan = nil
	if t == nil || !t.active {
		return false
	}
	velocity := t.velocity
	// A drag that stopped before the release is no flick.
	if objc.Send[float64](event, selTimestamp)-t.time > 0.05 {
		velocity = core.Point{}
	}
	t.view.report(core.Event{Kind: core.EventPan, Phase: core.GestureEnded, Point: t.at, Velocity: velocity})
	return true
}

// NSEventPhase's bits.
const phaseBegan, phaseChanged, phaseEnded, phaseCancelled = 1, 4, 8, 16

// magnify follows a trackpad pinch: its scale is 1 plus the
// magnification so far.
func magnify(self objc.ID, cmd objc.SEL, event objc.ID) {
	m := objc.Send[float64](event, sel("magnification"))
	if !trackpad(self, event, core.EventPinch, 1, m) {
		apple.Super(self, cmd, event)
	}
}

// rotate follows a trackpad rotation; AppKit counts degrees
// counterclockwise, views turn clockwise.
func rotate(self objc.ID, cmd objc.SEL, event objc.ID) {
	r := float64(objc.Send[float32](event, sel("rotation")))
	if !trackpad(self, event, core.EventRotation, 0, -r*math.Pi/180) {
		apple.Super(self, cmd, event)
	}
}

// trackpad moves the gesture of kind by delta from its first value
// initial, reporting whether a view follows it.
func trackpad(self, event objc.ID, kind core.EventKind, initial, delta float64) bool {
	v := views[self]
	if v == nil {
		return false
	}
	s := v.surface
	slot := &s.pinch
	if kind == core.EventRotation {
		slot = &s.rotation
	}
	phase := objc.Send[uint](event, sel("phase"))
	now := objc.Send[float64](event, selTimestamp)
	t := *slot
	report := core.GestureChanged
	if phase&phaseBegan != 0 || t == nil {
		if t != nil {
			t.view.report(core.Event{Kind: kind, Phase: core.GestureCancelled, Value: float32(t.value)})
		}
		target := gestureView(self, kind)
		if target == nil {
			*slot = nil
			return false
		}
		if phase&(phaseBegan|phaseChanged) == 0 {
			return true
		}
		t = &gestureTrack{view: target, value: initial, time: now}
		*slot = t
		report = core.GestureBegan
	}
	if dt := now - t.time; dt > 0 && delta != 0 {
		t.velocity = delta / dt
	}
	t.value += delta
	t.time = now
	switch {
	case phase&phaseEnded != 0:
		report, *slot = core.GestureEnded, nil
	case phase&phaseCancelled != 0:
		report, *slot = core.GestureCancelled, nil
	}
	t.view.report(core.Event{Kind: kind, Phase: report, Value: float32(t.value),
		Velocity: core.Point{X: core.Points(t.velocity)}})
	return true
}
