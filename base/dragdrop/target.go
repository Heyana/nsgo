package dragdrop

import (
	"slices"

	"github.com/nsgo-dev/nsgo/core"
)

// Target receives values on View. Accept may reject a value, allowing an
// accepting ancestor target to take it. OnEnter/OnLeave let the application
// draw the target state; no hover or press appearance is added. OnDrop must be
// set for the target to accept a drop. View.Ref is retained when supplied.
type Target struct {
	View                             core.View
	Accept                           func(data any) bool
	OnEnter, OnMove, OnLeave, OnDrop func(Event)
}

type target struct {
	p       Target
	cx      *core.Scope
	ref     *core.Ref
	live    bool
	blocked *core.Memo[bool]
}

func (t *target) disabled() bool {
	return !t.live || t.cx.Disposed() || t.blocked.GetUntracked()
}
func (t *target) call(fn func(Event), event Event) {
	if fn != nil && t.live && !t.cx.Disposed() {
		t.cx.Run(func() { fn(event) })
	}
}

// Render implements core.Node.
func (p Target) Render(cx *core.Scope) core.Node {
	a := areaKey.Use(cx)
	if a == nil || a.cx.Env() != cx.Env() {
		panic("dragdrop.Target needs an Area in the same window")
	}
	view := p.View
	if view.Ref == nil {
		view.Ref = new(core.Ref)
	}
	t := &target{p: p, cx: cx, ref: view.Ref, live: true, blocked: core.NewMemo(cx, func() bool { return p.View.Disabled != nil && p.View.Disabled() })}
	a.targets = append(a.targets, t)
	cx.Cleanup(func() {
		if at := slices.Index(a.targets, t); at >= 0 {
			a.targets = slices.Delete(a.targets, at, at+1)
		}
		if s := a.current.GetUntracked(); s != nil && s.target == t {
			s.target = nil
			if p.OnLeave != nil {
				p.OnLeave(Event{Data: s.data, Position: s.local, PagePosition: a.point.GetUntracked()})
			}
		}
		t.live = false
	})
	cx.Effect(func() {
		if t.blocked.Get() {
			if s := a.current.GetUntracked(); s != nil && s.target == t {
				s.target = nil
				t.call(t.p.OnLeave, Event{Data: s.data, Position: s.local, PagePosition: a.point.GetUntracked()})
			}
		}
	})
	return view
}
