// Package dragdrop transfers application values between views in one window.
// It owns gesture negotiation, target changes and cancellation, with no look:
// the application supplies each view and its preview, and keeps a menu or
// another keyboard-accessible way to perform the same operation.
package dragdrop

import "github.com/nsgo-dev/nsgo/core"

// Event is a value being dragged over a target. Data is sampled once as the
// drag starts; it may itself be a slice of the app's selected items.
type Event struct {
	Data         any
	Position     core.Point
	PagePosition core.Point
}

// Result ends a source's drag. Dropped says a live accepting target received
// it; Target is that target's view, nil when the drag was cancelled or missed.
type Result struct {
	Data    any
	Dropped bool
	Target  *core.Ref
}

// Area connects its Source and Target descendants in one window. It adds no
// layout container. Nested areas are independent; use one per window. The
// source keeps its original view while a noninteractive preview follows the
// pointer above the content. Escape, native cancellation or source disposal
// ends the drag without dropping.
type Area struct{ Children []core.Node }

var areaKey = core.NewKey[*area]("base.dragdrop", nil)

type area struct {
	cx      *core.Scope
	current *core.Signal[*session]
	point   *core.Signal[core.Point]
	targets []*target
}

type session struct {
	source *source
	data   any
	target *target
	local  core.Point
}

// Render implements core.Node.
func (p Area) Render(cx *core.Scope) core.Node {
	a := &area{cx: cx, current: core.NewSignal[*session](cx, nil), point: core.NewSignal(cx, core.Point{})}
	areaKey.Provide(cx, a)
	cx.Cleanup(func() { a.finish(false) })
	return core.Group(p.Children)
}

// preview is mounted under its source, so it inherits that source's contexts
// and stays in its modal or confined frame rather than appearing below it.
func (a *area) preview(source *source) core.Node {
	return core.Dynamic(func(*core.Scope) core.Node {
		s := a.current.Get()
		if s == nil || s.source != source {
			return nil
		}
		var preview core.Node = core.Text{Content: source.p.View.Label, ContentFunc: source.p.View.LabelFunc}
		if source.p.Preview != nil {
			preview = source.p.Preview(s.data)
		}
		return core.Overlay{PointerEvents: core.PointerEventsNone, OnCancel: func() { a.finish(false) }, StyleFunc: func() core.Style {
			at := a.point.Get()
			return core.Style{Position: core.Absolute, Left: core.Pt(at.X + 8), Top: core.Pt(at.Y + 8)}
		}, Children: []core.Node{core.View{AccessibilityElementsHidden: func() bool { return true }, Children: []core.Node{preview}}}}
	})
}

func (a *area) begin(s *source, at core.Point) {
	if s.disabled() || a.cx.Disposed() {
		return
	}
	a.finish(false)
	var data any
	s.cx.Run(func() { data = s.p.Data() })
	if s.disabled() {
		return
	}
	drag := &session{source: s, data: data}
	a.point.Set(at)
	a.current.Set(drag)
	if s.p.OnStart != nil {
		s.cx.Run(func() { s.p.OnStart(data) })
	}
	if a.current.GetUntracked() == drag {
		a.move(at)
	}
}

func (a *area) pick(s *session, at core.Point) (*target, core.Point) {
	refs := make([]*core.Ref, 0, len(a.targets))
	targets := make(map[*core.Ref]*target, len(a.targets))
	for _, t := range a.targets {
		if !t.disabled() && t.p.OnDrop != nil {
			refs = append(refs, t.ref)
			targets[t.ref] = t
		}
	}
	for _, hit := range core.HitTest(at, refs...) {
		t := targets[hit.Ref]
		accepted := true
		if t.p.Accept != nil {
			t.cx.Run(func() { accepted = t.p.Accept(s.data) })
		}
		if a.current.GetUntracked() != s {
			return nil, core.Point{}
		}
		if accepted && !t.disabled() {
			return t, hit.Position
		}
	}
	return nil, core.Point{}
}

func (a *area) move(at core.Point) {
	s := a.current.GetUntracked()
	if s == nil {
		return
	}
	if s.source.disabled() {
		a.finish(false)
		return
	}
	a.point.Set(at)
	next, local := a.pick(s, at)
	if a.current.GetUntracked() != s {
		return
	}
	if next != s.target {
		old, oldLocal := s.target, s.local
		s.target = nil
		if old != nil && old.live && old.p.OnLeave != nil {
			old.call(old.p.OnLeave, Event{Data: s.data, Position: oldLocal, PagePosition: at})
		}
		if a.current.GetUntracked() != s {
			return
		}
		if next != nil && !next.disabled() {
			s.target, s.local = next, local
			next.call(next.p.OnEnter, Event{Data: s.data, Position: local, PagePosition: at})
		}
	}
	if a.current.GetUntracked() == s && s.target != nil {
		s.local = local
		s.target.call(s.target.p.OnMove, Event{Data: s.data, Position: local, PagePosition: at})
	}
}

func (a *area) finish(drop bool) {
	s := a.current.GetUntracked()
	if s == nil {
		return
	}
	at := a.point.GetUntracked()
	if drop {
		a.move(at)
		if a.current.GetUntracked() != s {
			return
		}
	}
	a.current.Set(nil)
	result := Result{Data: s.data}
	if t := s.target; t != nil {
		event := Event{Data: s.data, Position: s.local, PagePosition: at}
		if drop && !t.disabled() && t.p.OnDrop != nil {
			result.Dropped, result.Target = true, t.ref
			t.call(t.p.OnDrop, event)
		}
		if t.live {
			t.call(t.p.OnLeave, event)
		}
	}
	if s.source.live && s.source.p.OnEnd != nil {
		if s.source.cx.Disposed() {
			// A source's own cleanup still balances its start notification.
			s.source.p.OnEnd(result)
		} else {
			s.source.cx.Run(func() { s.source.p.OnEnd(result) })
		}
	}
}
