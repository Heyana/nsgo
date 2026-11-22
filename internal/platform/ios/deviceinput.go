//go:build !ios || nsgotest || nsgoautomation

package ios

import (
	"errors"
	"math"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Physical Apple device input helpers for testsurface.go (iPhone and Apple TV).
// On physical devices, the simulator's private IOKit/BackBoardServices HID path
// is unavailable, so presses, scrolls, drags, alerts and keys go through public
// UIKit APIs, accessibility actions, UIControl actions and NSGo's hit testing
// and event dispatch. This file contains the platform-independent decision and
// state logic so host unit tests can verify it directly.

// isSimulatorEnv reports whether the process environment belongs to Xcode's
// iOS or tvOS simulator (CoreSimulator sets SIMULATOR_MODEL_IDENTIFIER and
// SIMULATOR_UDID).
func isSimulatorEnv(getenv func(string) string) bool {
	if getenv == nil {
		return false
	}
	return getenv("SIMULATOR_MODEL_IDENTIFIER") != "" || getenv("SIMULATOR_UDID") != ""
}

// shouldUseSimulatorHID reports whether testSurface should use the simulator's
// private HID event injection. On physical devices it returns false without
// ever invoking loadHID; on simulators it falls back cleanly if loadHID fails.
func shouldUseSimulatorHID(isSimulator bool, loadHID func() error) bool {
	if !isSimulator || loadHID == nil {
		return false
	}
	return loadHID() == nil
}

// devicePressKind names how a press at a point on a physical iPhone or Apple TV
// is delivered once hit testing resolves the responder chain.
type devicePressKind uint8

const (
	devicePressNone devicePressKind = iota
	devicePressFocusText
	devicePressSwitch
	devicePressTabBar
	devicePressControl
	devicePressAccessibility
	devicePressView
)

// deviceHitNode describes one view along the superview chain from the leaf view
// returned by -[UIWindow hitTest:withEvent:] up to the window.
type deviceHitNode struct {
	index          int
	isTextEditor   bool
	isSwitch       bool
	isTabBar       bool
	isControl      bool
	isNSGoView     bool
	pressable      bool
	hasActivate    bool
	hasPressDownUp bool
	hasLinkAtPoint bool
	isAxElement    bool
}

// devicePressTarget is the resolved action and index into the hit chain.
type devicePressTarget struct {
	kind  devicePressKind
	index int
}

// resolveDevicePress picks how to deliver a press along the hit-test chain
// (innermost view first) on a physical iOS or tvOS device.
func resolveDevicePress(chain []deviceHitNode, hold bool, mods driver.Modifiers) devicePressTarget {
	axFallback := -1
	for i, n := range chain {
		switch {
		case n.isTextEditor:
			return devicePressTarget{kind: devicePressFocusText, index: i}
		case n.isSwitch:
			return devicePressTarget{kind: devicePressSwitch, index: i}
		case n.isTabBar:
			return devicePressTarget{kind: devicePressTabBar, index: i}
		case n.isControl:
			return devicePressTarget{kind: devicePressControl, index: i}
		case n.isNSGoView:
			// A view offering the standard accessibility activate action takes
			// accessibilityActivate on a plain tap when it does not need press
			// down/up, long press, modifiers or link-span coordinates.
			if n.hasActivate && !hold && mods == (driver.Modifiers{}) && !n.hasPressDownUp && !n.hasLinkAtPoint {
				return devicePressTarget{kind: devicePressAccessibility, index: i}
			}
			if n.pressable {
				return devicePressTarget{kind: devicePressView, index: i}
			}
			if n.hasActivate {
				return devicePressTarget{kind: devicePressAccessibility, index: i}
			}
		case n.isAxElement && axFallback < 0:
			axFallback = i
		}
	}
	if axFallback >= 0 {
		return devicePressTarget{kind: devicePressAccessibility, index: axFallback}
	}
	return devicePressTarget{kind: devicePressNone, index: -1}
}

// clampScrollOffset clamps cur + delta to a UIScrollView's valid contentOffset
// range and reports whether the scroll view can move from cur toward delta.
func clampScrollOffset(cur, delta driver.Point, contentSize, boundsSize driver.Size, insetTop, insetLeft, insetBottom, insetRight driver.Points) (to driver.Point, canMove bool) {
	minX := -insetLeft
	maxX := max(minX, contentSize.Width+insetRight-boundsSize.Width)
	minY := -insetTop
	maxY := max(minY, contentSize.Height+insetBottom-boundsSize.Height)
	to = driver.Point{
		X: min(max(cur.X+delta.X, minX), maxX),
		Y: min(max(cur.Y+delta.Y, minY), maxY),
	}
	const eps = 0.5
	canMove = math.Abs(float64(to.X-cur.X)) > eps || math.Abs(float64(to.Y-cur.Y)) > eps
	return to, canMove
}

// resolveAlertAction validates an alert answer index and returns the button's
// label and the EventAlert Index that UIAlertController's handler dispatches.
func resolveAlertAction(actions []driver.AlertAction, index int) (label string, dispatchIndex int, err error) {
	switch {
	case index >= 0 && index < len(actions):
		return actions[index].Label, index, nil
	case len(actions) == 0:
		return "OK", -1, nil
	default:
		cancelIdx := -1
		for i, a := range actions {
			if a.Role == driver.AlertCancel {
				label = a.Label
				cancelIdx = i
			}
		}
		if label == "" {
			return "", 0, errors.New("the alert cannot be dismissed")
		}
		return label, cancelIdx, nil
	}
}

// interpolateDragPath expands path into steps of at most 10 points along each
// segment, matching fingerAlong's cadence for synthetic drag/pan dispatch.
func interpolateDragPath(path []driver.Point) []driver.Point {
	if len(path) == 0 {
		return nil
	}
	out := []driver.Point{path[0]}
	at := path[0]
	for _, to := range path[1:] {
		n := int(math.Max(math.Abs(float64(to.X-at.X)), math.Abs(float64(to.Y-at.Y)))/10) + 1
		for i := 1; i <= n; i++ {
			f := driver.Points(i) / driver.Points(n)
			out = append(out, driver.Point{
				X: at.X + (to.X-at.X)*f,
				Y: at.Y + (to.Y-at.Y)*f,
			})
		}
		at = to
	}
	return out
}

// UIKeyModifierFlags values matching UIKit and internal/platform/apple.KeyNames.
const (
	deviceModShift uint = 1 << 17
	deviceModCtrl  uint = 1 << 18
	deviceModAlt   uint = 1 << 19
	deviceModMeta  uint = 1 << 20
)

var deviceKeyAliases = map[string]string{
	"enter": "enter", "return": "enter", "escape": "escape", "backspace": "backspace",
	"tab": "tab", "space": "space", "right": "right", "left": "left",
	"down": "down", "up": "up", "delete": "delete",
	"home": "home", "end": "end", "pageup": "pageup", "pagedown": "pagedown",
	"f1": "f1", "f2": "f2", "f3": "f3", "f4": "f4", "f5": "f5", "f6": "f6",
	"f7": "f7", "f8": "f8", "f9": "f9", "f10": "f10", "f11": "f11", "f12": "f12",
}

func normalizeDeviceModifier(part string) (name string, ok bool) {
	switch part {
	case "shift":
		return "shift", true
	case "ctrl", "control":
		return "ctrl", true
	case "alt", "option":
		return "alt", true
	case "mod", "cmd", "command", "meta":
		return "meta", true
	}
	return "", false
}

func normalizeDeviceSingleKey(part string) (string, bool) {
	if m, ok := normalizeDeviceModifier(part); ok {
		return m, true
	}
	if alias, ok := deviceKeyAliases[part]; ok {
		return alias, true
	}
	if len(part) == 1 {
		c := part[0]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			return part, true
		}
	}
	return "", false
}

func applyDeviceModifier(mods driver.Modifiers, flags uint, name string, held bool) (driver.Modifiers, uint) {
	switch name {
	case "shift":
		mods.Shift = held
		if held {
			flags |= deviceModShift
		} else {
			flags &^= deviceModShift
		}
	case "ctrl":
		mods.Control = held
		if held {
			flags |= deviceModCtrl
		} else {
			flags &^= deviceModCtrl
		}
	case "alt":
		mods.Alt = held
		if held {
			flags |= deviceModAlt
		} else {
			flags &^= deviceModAlt
		}
	case "meta":
		mods.Platform = held
		if held {
			flags |= deviceModMeta
		} else {
			flags &^= deviceModMeta
		}
	}
	return mods, flags
}

// parseDeviceKey parses a key or chord such as "enter", "mod+shift+k", or
// "shift" into its normalized key name, driver.Modifiers and UIKeyModifierFlags.
func parseDeviceKey(key string) (name string, mods driver.Modifiers, flags uint, ok bool) {
	parts := strings.Split(strings.ToLower(key), "+")
	if len(parts) == 0 {
		return "", driver.Modifiers{}, 0, false
	}
	for i, p := range parts {
		if i < len(parts)-1 {
			m, isMod := normalizeDeviceModifier(p)
			if !isMod {
				return "", driver.Modifiers{}, 0, false
			}
			mods, flags = applyDeviceModifier(mods, flags, m, true)
			continue
		}
		k, valid := normalizeDeviceSingleKey(p)
		if !valid {
			return "", driver.Modifiers{}, 0, false
		}
		name = k
		if _, isMod := normalizeDeviceModifier(k); isMod {
			mods, flags = applyDeviceModifier(mods, flags, k, true)
		}
	}
	return name, mods, flags, true
}

// deviceKeyState tracks hardware keys held via KeyDown/KeyUp on physical
// devices so held modifiers join subsequent key presses until KeyUp.
type deviceKeyState struct {
	held map[string]driver.NodeID
}

// down records name going down (targeting id) and returns the effective
// modifiers and UIKeyModifierFlags, plus whether an earlier down for name was
// still held (with its prior target id).
func (st *deviceKeyState) down(name string, id driver.NodeID) (mods driver.Modifiers, flags uint, prevID driver.NodeID, wasHeld bool) {
	if st.held == nil {
		st.held = map[string]driver.NodeID{}
	}
	prevID, wasHeld = st.held[name]
	st.held[name] = id
	mods, flags = st.modifiers()
	return mods, flags, prevID, wasHeld
}

// up removes name from the held set and returns its prior target id, whether it
// was held, and the remaining modifiers.
func (st *deviceKeyState) up(name string) (id driver.NodeID, wasHeld bool, mods driver.Modifiers, flags uint) {
	if st.held == nil {
		return 0, false, driver.Modifiers{}, 0
	}
	id, wasHeld = st.held[name]
	if !wasHeld {
		return 0, false, driver.Modifiers{}, 0
	}
	delete(st.held, name)
	mods, flags = st.modifiers()
	return id, true, mods, flags
}

// modifiers returns the currently held modifiers and UIKeyModifierFlags.
func (st *deviceKeyState) modifiers() (mods driver.Modifiers, flags uint) {
	for k := range st.held {
		mods, flags = applyDeviceModifier(mods, flags, k, true)
	}
	return mods, flags
}

// hidPressTracker tracks synthetic simulator HID press releases posted to a
// UIWindow until UIKit delivers their touch end/cancel through sendEvent:,
// accessed only on the main thread.
type hidPressTracker struct {
	posted  int
	handled int
	target  uintptr
}

// mark records a press release enqueued to window w and returns its serial.
func (p *hidPressTracker) mark(window uintptr) int {
	p.posted++
	p.target = window
	return p.posted
}

// delivered records that UIKit finished dispatching a touch end/cancel on window w.
func (p *hidPressTracker) delivered(window uintptr) {
	if window != 0 && window == p.target && p.handled < p.posted {
		p.handled = p.posted
	}
}

// clear drops any pending press expectation (for example when an alert or closing window takes input).
func (p *hidPressTracker) clear() {
	p.handled = p.posted
	p.target = 0
}

// pending reports whether serial has not yet been delivered.
func (p *hidPressTracker) pending(serial int) bool {
	return serial > p.handled
}

// testSurfaceBusy reports whether the iOS test host still has queued mount
// batches, finite animations, layout motions, vanishing ghosts, or an
// undelivered press release in flight.
func testSurfaceBusy(queuedBatches int, animatorEnding bool, motions, ghosts int, pressPending, windowAnimating bool) bool {
	return queuedBatches > 0 || animatorEnding || motions > 0 || ghosts > 0 || pressPending || windowAnimating
}
