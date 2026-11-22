package scrollbar

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// TouchView is the state needed to arbitrate and move a desktop touch drag.
type TouchView struct {
	Config          *driver.ScrollConfig
	Offset          driver.Point
	Frame, Content  driver.Size
	Deleted, Moving bool
	Refreshing      bool
}

// TouchHost keeps native press cancellation, scrolling and indicators in
// the backend while TouchSession owns which scroll view takes a finger.
type TouchHost[V comparable] struct {
	State     func(V) TouchView
	Interrupt func(V)
	Begin     func(V, bool)
	Move      func(V, driver.Point)
	Show      func(V)
	Blocked   bool
}

// TouchSession follows the scroll views under one finger, innermost first.
type TouchSession[V comparable] struct {
	views  []V
	drags  []*TouchDrag
	Taken  V
	Drag   *TouchDrag
	Braked bool
}

// NewTouchSession brakes ongoing motion and skips views that cannot scroll.
// under is the outermost-first hit-test result.
func NewTouchSession[V comparable](under []V, at driver.Point, now time.Time, host TouchHost[V]) *TouchSession[V] {
	session := &TouchSession[V]{}
	for i := len(under) - 1; i >= 0; i-- {
		v := under[i]
		state := host.State(v)
		if state.Config.Behavior.NoScroll {
			continue
		}
		if state.Moving {
			host.Interrupt(v)
			session.Braked = true
		}
		session.views = append(session.views, v)
		session.drags = append(session.drags, NewTouchDrag(at, Along(state.Config, state.Offset), now))
	}
	if len(session.views) == 0 {
		return nil
	}
	return session
}

// Move returns whether the finger was handled and whether to keep its session.
// Braking makes the finger the scroll view's immediately; otherwise a pan
// may win before the drag crosses its slop.
func (session *TouchSession[V]) Move(at driver.Point, now time.Time, host TouchHost[V]) (handled, keep bool) {
	if session == nil {
		return false, false
	}
	var zero V
	if session.Taken == zero {
		if !session.Braked && host.Blocked {
			return false, false
		}
		for i, v := range session.views {
			state := host.State(v)
			if !state.Deleted && session.drags[i].Claims(state.Config, at) {
				session.Taken, session.Drag = v, session.drags[i]
				break
			}
		}
		if session.Taken == zero {
			return session.Braked, true
		}
		host.Begin(session.Taken, session.Braked)
	}
	v := session.Taken
	state := host.State(v)
	if state.Deleted {
		return true, false
	}
	if host.Show != nil {
		host.Show(v)
	}
	c := state.Config
	raw := session.Drag.Move(c, at, now)
	lo, hi := Limits(c, state.Frame, state.Content, state.Refreshing)
	viewport := Along(c, driver.Point{X: state.Frame.Width, Y: state.Frame.Height})
	offset := state.Offset
	SetAlong(c, &offset, Dragged(raw, lo, hi, viewport, !c.Behavior.NoBounces))
	host.Move(v, offset)
	return true, true
}
