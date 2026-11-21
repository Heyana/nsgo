package gamepad

import (
	"math"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

const (
	defaultDeadzone       float32       = 0.5
	defaultInitialDelay   time.Duration = 400 * time.Millisecond
	defaultRepeatInterval time.Duration = 120 * time.Millisecond
)

type focusNavConfig struct {
	deadzone       float32
	initialDelay   time.Duration
	repeatInterval time.Duration
	wrap           bool
}

// FocusNavOption configures gamepad focus navigation (EnableFocusNavigation).
type FocusNavOption func(*focusNavConfig)

// WithDeadzone sets the left-stick axis threshold in (0, 1] beyond which stick deflection moves focus. The default is 0.5.
func WithDeadzone(threshold float32) FocusNavOption {
	return func(c *focusNavConfig) {
		if threshold > 0 && threshold <= 1 {
			c.deadzone = threshold
		}
	}
}

// WithRepeatDelay sets the initial delay before a held D-pad or left-stick direction begins repeating and the interval between subsequent repeats. The defaults are 400ms and 120ms.
func WithRepeatDelay(initial, interval time.Duration) FocusNavOption {
	return func(c *focusNavConfig) {
		if initial > 0 {
			c.initialDelay = initial
		}
		if interval > 0 {
			c.repeatInterval = interval
		}
	}
}

// WithWrap controls whether directional moves wrap in Tab order when no focus stop lies further in that direction. The default is false.
func WithWrap(wrap bool) FocusNavOption {
	return func(c *focusNavConfig) {
		c.wrap = wrap
	}
}

type padNavState struct {
	dir        core.FocusDirection
	nextRepeat time.Time
	south      bool
	east       bool
}

// EnableFocusNavigation turns on explicit gamepad navigation through native focus stops in cx's tree. An app opts into it explicitly; nothing enables it implicitly by device.
//
// While enabled, at least one gamepad is connected, and no scope holds CaptureInput, EnableFocusNavigation polls connected pads on RequestAnimationFrame:
//   - D-pad directions and the left analog stick (past the deadzone, default
//     0.5, with initial/repeat delay when held) move focus across the same
//     enabled native focus stops Tab reaches.
//   - ButtonSouth (A / Cross) activates the currently focused native view.
//   - ButtonEast (B / Circle) triggers Back (overlays, modals, Scope.OnBackPress,
//     navigation stacks).
//
// When no pads are connected or while any mounted scope holds CaptureInput, the animation frame loop stops so no idle frames run. Navigation stops automatically when cx is disposed or when the returned stop function is called.
func EnableFocusNavigation(cx *core.Scope, opts ...FocusNavOption) (stop func()) {
	if cx == nil {
		return func() {}
	}
	root, _, _ := core.FrameState(cx)
	if root == nil {
		return func() {}
	}
	cfg := focusNavConfig{
		deadzone:       defaultDeadzone,
		initialDelay:   defaultInitialDelay,
		repeatInterval: defaultRepeatInterval,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	st := ensureState(root)
	child := cx.Child()
	prev := map[int]padNavState{}
	stopped := false

	stop = func() {
		if stopped {
			return
		}
		stopped = true
		child.Dispose()
	}

	child.Run(func() {
		child.Effect(func() {
			pads := Pads(child)
			captured := st.captures.Get()
			if stopped || child.Disposed() || len(pads) == 0 || captured > 0 {
				clear(prev)
				return
			}

			// Prune disconnected pads and seed newly-seen pads from their current state so buttons already held when resuming from CaptureInput do not fire spurious edge presses.
			connectedIDs := make(map[int]bool, len(pads))
			for _, p := range pads {
				connectedIDs[p.ID] = true
				if _, exists := prev[p.ID]; !exists {
					prev[p.ID] = padNavState{
						dir:   padDirection(p, cfg.deadzone),
						south: p.Pressed(ButtonSouth),
						east:  p.Pressed(ButtonEast),
					}
				}
			}
			for id := range prev {
				if !connectedIDs[id] {
					delete(prev, id)
				}
			}

			active := true
			var cancel func()
			var tick func(at time.Time)
			tick = func(at time.Time) {
				if !active || stopped || child.Disposed() || st.captures.GetUntracked() > 0 {
					return
				}
				ids := st.connected.GetUntracked()
				if len(ids) == 0 {
					return
				}
				for _, ident := range ids {
					p, ok := State(child, ident.ID)
					if !ok {
						continue
					}
					ps := prev[p.ID]

					dir := padDirection(p, cfg.deadzone)
					var moveDir core.FocusDirection
					switch {
					case dir == core.FocusDirNone:
						ps.dir = core.FocusDirNone
						ps.nextRepeat = time.Time{}
					case dir != ps.dir:
						ps.dir = dir
						ps.nextRepeat = at.Add(cfg.initialDelay)
						moveDir = dir
					case !ps.nextRepeat.IsZero() && !at.Before(ps.nextRepeat):
						ps.nextRepeat = at.Add(cfg.repeatInterval)
						moveDir = dir
					}

					south := p.Pressed(ButtonSouth)
					triggerSouth := south && !ps.south
					ps.south = south

					east := p.Pressed(ButtonEast)
					triggerEast := east && !ps.east
					ps.east = east

					prev[p.ID] = ps

					if moveDir != core.FocusDirNone {
						core.MoveFocus(child, moveDir, cfg.wrap)
						if !active || stopped || child.Disposed() || st.captures.GetUntracked() > 0 {
							return
						}
					}
					if triggerSouth {
						core.ActivateFocused(child)
						if !active || stopped || child.Disposed() || st.captures.GetUntracked() > 0 {
							return
						}
					}
					if triggerEast {
						core.TriggerBack(child)
						if !active || stopped || child.Disposed() || st.captures.GetUntracked() > 0 {
							return
						}
					}
				}

				if active && !stopped && !child.Disposed() && st.captures.GetUntracked() == 0 {
					cancel = child.RequestAnimationFrame(tick)
				}
			}

			cancel = child.RequestAnimationFrame(tick)
			child.Cleanup(func() {
				active = false
				if cancel != nil {
					cancel()
					cancel = nil
				}
			})
		})
	})

	return stop
}

// CaptureInput claims raw gamepad input for the lifetime of cx (or until the returned release function is called). While any scope holds CaptureInput, EnableFocusNavigation pauses completely so active gameplay views receive raw controller input through Pads and State without moving native focus. Multiple mounted scopes may hold captures concurrently; focus navigation resumes once every capture is released.
func CaptureInput(cx *core.Scope) (release func()) {
	if cx == nil {
		return func() {}
	}
	root, _, _ := core.FrameState(cx)
	if root == nil {
		return func() {}
	}
	st := ensureState(root)
	st.captures.Set(st.captures.GetUntracked() + 1)

	released := false
	release = func() {
		if released {
			return
		}
		released = true
		if n := st.captures.GetUntracked(); n > 0 {
			st.captures.Set(n - 1)
		}
	}
	cx.Cleanup(release)
	return release
}

// padDirection returns the dominant directional input from p's D-pad or left analog stick (outside deadzone). D-pad takes precedence over the left stick.
func padDirection(p Pad, deadzone float32) core.FocusDirection {
	up := p.Pressed(ButtonDPadUp)
	down := p.Pressed(ButtonDPadDown)
	left := p.Pressed(ButtonDPadLeft)
	right := p.Pressed(ButtonDPadRight)

	v := 0
	if down {
		v++
	}
	if up {
		v--
	}
	h := 0
	if right {
		h++
	}
	if left {
		h--
	}
	switch {
	case v > 0:
		return core.FocusDirDown
	case v < 0:
		return core.FocusDirUp
	case h > 0:
		return core.FocusDirRight
	case h < 0:
		return core.FocusDirLeft
	}

	x := p.Axes[AxisLeftX]
	y := p.Axes[AxisLeftY]
	ax := float32(math.Abs(float64(x)))
	ay := float32(math.Abs(float64(y)))
	if ax < deadzone && ay < deadzone {
		return core.FocusDirNone
	}
	if ay >= ax {
		if y > 0 {
			return core.FocusDirDown
		}
		return core.FocusDirUp
	}
	if x > 0 {
		return core.FocusDirRight
	}
	return core.FocusDirLeft
}
