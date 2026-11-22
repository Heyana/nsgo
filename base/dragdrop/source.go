package dragdrop

import (
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// Activation chooses how a source starts a drag, independently of device.
type Activation uint8

const (
	// AfterMove starts after the pointer moves Threshold points. It is the default.
	AfterMove Activation = iota
	// AfterLongPress first waits for View.DelayLongPress, or the normal long-press
	// delay, then for movement. A scroll may take an earlier swipe as usual.
	AfterLongPress
)

// Source makes View draggable within its Area. View retains its press, keyboard,
// accessibility and styling props; its Responder is owned by Source and must be
// unset. Put a button or another existing control in View.Children when it owns
// the normal action. A movement below Threshold leaves that action intact.
type Source struct {
	View core.View
	// Data is sampled once when the drag starts. A nil function disables dragging.
	// Copy mutable data here when the drag must retain a snapshot.
	Data func() any
	// Preview supplies a label or picture. Without it View.Label/LabelFunc is used.
	Preview    func(data any) core.Node
	Activation Activation
	// Threshold is movement in points, 4 when zero or negative.
	Threshold core.Points
	OnStart   func(data any)
	OnEnd     func(Result)
}

type source struct {
	p           Source
	cx          *core.Scope
	area        *area
	live, armed bool
	blocked     *core.Memo[bool]
}

func (s *source) disabled() bool {
	return !s.live || s.cx.Disposed() || s.p.Data == nil || s.blocked.GetUntracked()
}

// Render implements core.Node.
func (p Source) Render(cx *core.Scope) core.Node {
	a := areaKey.Use(cx)
	if a == nil || a.cx.Env() != cx.Env() {
		panic("dragdrop.Source needs an Area in the same window")
	}
	if p.View.Responder != nil {
		panic("dragdrop.Source owns View.Responder")
	}
	s := &source{p: p, cx: cx, area: a, live: true, blocked: core.NewMemo(cx, func() bool { return p.View.Disabled != nil && p.View.Disabled() })}
	cx.Cleanup(func() {
		if drag := a.current.GetUntracked(); drag != nil && drag.source == s {
			a.finish(false)
		}
		s.live = false
	})
	cx.Effect(func() {
		if s.blocked.Get() {
			if drag := a.current.GetUntracked(); drag != nil && drag.source == s {
				a.finish(false)
			}
		}
	})
	view := p.View
	threshold := p.Threshold
	if threshold <= 0 {
		threshold = 4
	}
	var origin core.Point
	var stopHold func() bool
	stop := func() {
		if stopHold != nil {
			stopHold()
			stopHold = nil
		}
	}
	cx.Cleanup(stop)
	oldTouch := view.OnTouchCapture
	view.OnTouchCapture = func(e core.TouchEvent) {
		switch e.Phase {
		case core.TouchStart:
			stop()
			s.armed = false
			if p.Activation == AfterLongPress && len(e.Touches) == 1 && !s.disabled() {
				origin = e.Touches[0].PagePosition
				delay := view.DelayLongPress
				if delay <= 0 {
					delay = 500 * time.Millisecond
				}
				stopHold = cx.AfterFunc(delay, func() { stopHold = nil; s.armed = !s.disabled() })
			}
		case core.TouchMove:
			if !s.armed && stopHold != nil && len(e.Touches) == 1 {
				at := e.Touches[0].PagePosition
				dx, dy := at.X-origin.X, at.Y-origin.Y
				if dx*dx+dy*dy >= threshold*threshold {
					stop()
				}
			}
		case core.TouchEnd, core.TouchCancel:
			stop()
		}
		if oldTouch != nil {
			oldTouch(e)
		}
	}
	view.Children = append(append([]core.Node(nil), view.Children...), a.preview(s))
	view.Responder = &core.Responder{Keep: true, Claims: []core.TouchClaim{{OnMove: true, Ask: func(e core.ResponderEvent) bool {
		if s.disabled() || e.Gesture.Touches != 1 {
			return false
		}
		if p.Activation == AfterLongPress && !s.armed {
			return false
		}
		delta := e.Gesture.Delta
		return delta.X*delta.X+delta.Y*delta.Y >= threshold*threshold
	}}}, OnGrant: func(e core.ResponderEvent) { a.begin(s, e.PagePosition) }, OnMove: func(e core.ResponderEvent) {
		if e.Gesture.Touches != 1 {
			a.finish(false)
			return
		}
		if drag := a.current.GetUntracked(); drag != nil && drag.source == s {
			a.move(e.PagePosition)
		}
	}, OnRelease: func(e core.ResponderEvent) {
		s.armed = false
		if drag := a.current.GetUntracked(); drag != nil && drag.source == s {
			a.move(e.PagePosition)
			a.finish(true)
		}
	}, OnTerminate: func(core.ResponderEvent) {
		s.armed = false
		if drag := a.current.GetUntracked(); drag != nil && drag.source == s {
			a.finish(false)
		}
	}}
	return view
}
