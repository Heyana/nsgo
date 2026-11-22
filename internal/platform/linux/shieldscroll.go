package linux

import (
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/scrollbar"
)

// shieldScrollIdle is how long without another event a shielded touchpad
// scroll stays latched once the pointer is no longer over a shielded
// hosted control, so a dropped stop event cannot hijack a later scroll
// on an unshielded control.
const shieldScrollIdle = 150 * time.Millisecond

// shieldScroll tracks a touchpad scroll intercepted by the hosted-view
// shield (input.go), whose GdkScrollEvents never reach the surface's
// GtkEventControllerScroll. It records cumulative surface-unit deltas
// in a scrollbar.Velocity window and yields the release speed, in points
// per second, when the scroll-stop event arrives.
type shieldScroll struct {
	active       bool
	moved        bool
	total        driver.Point
	velocity     scrollbar.Velocity
	lastAt       time.Duration
	hasEventTime bool
	eventBase    uint32
	lastWall     time.Time
}

// reset forgets any shielded touchpad scroll in progress.
func (s *shieldScroll) reset() {
	*s = shieldScroll{}
}

// latched reports whether a shielded touchpad scroll stays latched at
// evMS/now: either still over a shielded control, ending on a stop
// event, or continuing within shieldScrollIdle after the hosted control
// scrolled out from under the pointer.
func (s *shieldScroll) latched(evMS uint32, now time.Time, shielded, isStop bool) bool {
	if !s.active {
		return false
	}
	if isStop || shielded {
		return true
	}
	return s.peekTime(evMS, now)-s.lastAt <= shieldScrollIdle
}

// peekTime converts an incoming event's timestamp onto s's monotonic
// duration without mutating s.
func (s *shieldScroll) peekTime(evMS uint32, now time.Time) time.Duration {
	if s.hasEventTime && evMS != 0 {
		diff := int32(evMS - s.eventBase)
		if diff < 0 {
			return s.lastAt
		}
		return time.Duration(diff) * time.Millisecond
	}
	dt := now.Sub(s.lastWall)
	if dt < 0 {
		dt = 0
	}
	return s.lastAt + dt
}

// startTime initializes the gesture's clock from evMS (GDK's 32-bit ms
// timestamp, or 0 for GDK_CURRENT_TIME) and wall time now.
func (s *shieldScroll) startTime(evMS uint32, now time.Time) {
	s.hasEventTime = evMS != 0
	s.eventBase = evMS
	s.lastAt = 0
	s.lastWall = now
}

// advance returns the gesture-relative timestamp for an event at
// evMS/now and updates s's clock state.
func (s *shieldScroll) advance(evMS uint32, now time.Time) time.Duration {
	at := s.peekTime(evMS, now)
	s.lastAt = at
	s.lastWall = now
	return at
}

// hold starts tracking when fingers land on the touchpad over a shielded
// control (GDK_TOUCHPAD_HOLD begin).
func (s *shieldScroll) hold(evMS uint32, now time.Time) {
	s.reset()
	s.active = true
	s.startTime(evMS, now)
	s.velocity.Add(0, driver.Point{})
}

// delta records a smooth surface-unit scroll delta (dx, dy) in points
// and reports whether this began a new touchpad scroll gesture (so the
// caller stops any prior glide and clears wheelTaker).
func (s *shieldScroll) delta(evMS uint32, now time.Time, dx, dy float64) (began bool) {
	if !s.active || (s.moved && s.peekTime(evMS, now)-s.lastAt > shieldScrollIdle) {
		s.reset()
		s.active = true
		s.startTime(evMS, now)
		began = true
	}
	at := s.advance(evMS, now)
	s.total.X += driver.Points(dx)
	s.total.Y += driver.Points(dy)
	s.velocity.Add(at, s.total)
	s.moved = true
	return began
}

// stop ends the shielded touchpad scroll on its stop event and returns
// its release velocity in points per second, along with whether a scroll
// with deltas was in progress.
func (s *shieldScroll) stop(evMS uint32, now time.Time) (vx, vy float64, ok bool) {
	if !s.active {
		return 0, 0, false
	}
	if !s.moved {
		s.reset()
		return 0, 0, false
	}
	at := s.advance(evMS, now)
	s.velocity.Add(at, s.total)
	sx, sy := s.velocity.Speed()
	s.reset()
	return float64(sx) * 1000, float64(sy) * 1000, true
}
