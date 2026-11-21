// Package gamepad reads the buttons and axes of connected game controllers in
// the W3C Standard Gamepad layout on every target that supports controllers
// (macOS, iOS, Windows, Linux, Android and the web; see core.Env.SupportsGamepads).
//
// Pads is a tracked reactive read that rerenders scopes reading it when a
// controller connects or disconnects without rerendering on every button press
// or stick movement. State returns one controller's buttons and axes as of the
// current display frame, intended for core.Scope.RequestAnimationFrame loops.
package gamepad

import (
	"math"
	"slices"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Compile-time check that public counts match the driver bridge.
var (
	_ [driver.GamepadButtonCount - ButtonCount]byte
	_ [ButtonCount - driver.GamepadButtonCount]byte
	_ [driver.GamepadAxisCount - AxisCount]byte
	_ [AxisCount - driver.GamepadAxisCount]byte
)

// Button is a button index in the W3C Standard Gamepad layout (0..16).
type Button uint8

// W3C Standard Gamepad button indices.
const (
	ButtonSouth      Button = 0 // Bottom face button (Xbox A, PlayStation Cross, Nintendo B)
	ButtonA                 = ButtonSouth
	ButtonEast       Button = 1 // Right face button (Xbox B, PlayStation Circle, Nintendo A)
	ButtonB                 = ButtonEast
	ButtonWest       Button = 2 // Left face button (Xbox X, PlayStation Square, Nintendo Y)
	ButtonX                 = ButtonWest
	ButtonNorth      Button = 3 // Top face button (Xbox Y, PlayStation Triangle, Nintendo X)
	ButtonY                 = ButtonNorth
	ButtonLB         Button = 4 // Left shoulder / bumper (L1)
	ButtonRB         Button = 5 // Right shoulder / bumper (R1)
	ButtonLT         Button = 6 // Left trigger (L2), analog in [0, 1]
	ButtonRT         Button = 7 // Right trigger (R2), analog in [0, 1]
	ButtonBack       Button = 8 // Left center button (Back, Select, Share, View)
	ButtonSelect            = ButtonBack
	ButtonStart      Button = 9  // Right center button (Start, Options, Menu)
	ButtonLeftStick  Button = 10 // Left stick click (L3)
	ButtonRightStick Button = 11 // Right stick click (R3)
	ButtonDPadUp     Button = 12
	ButtonDPadDown   Button = 13
	ButtonDPadLeft   Button = 14
	ButtonDPadRight  Button = 15
	ButtonHome       Button = 16 // Center logo / guide / Home button

	ButtonCount = 17
)

// Axis is an analog stick axis index in the W3C Standard Gamepad layout (0..3).
// Negative values point left or up; positive values point right or down, in [-1, 1].
type Axis uint8

// W3C Standard Gamepad analog stick axis indices.
const (
	AxisLeftX  Axis = 0
	AxisLeftY  Axis = 1
	AxisRightX Axis = 2
	AxisRightY Axis = 3

	AxisCount = 4
)

// Pad is a connected game controller's state in the W3C Standard Gamepad layout.
type Pad struct {
	ID        int
	Name      string
	VendorID  uint16
	ProductID uint16
	Family    Family
	CanRumble bool
	Buttons   [ButtonCount]float32
	Axes      [AxisCount]float32
}

// RumbleEvent records one rumble command or stop sent to a controller.
type RumbleEvent struct {
	ID       int
	LowFreq  float32
	HighFreq float32
	Duration time.Duration
	Stopped  bool
}

// Pressed reports whether button b is pressed (value >= 0.5).
func (p Pad) Pressed(b Button) bool {
	if int(b) >= len(p.Buttons) {
		return false
	}
	return p.Buttons[b] >= 0.5
}

type padIdentity struct {
	ID        int
	Name      string
	VendorID  uint16
	ProductID uint16
	Family    Family
	CanRumble bool
}

type activeRumble struct {
	gen       uint64
	owner     *core.Scope
	stopTimer func() bool
}

type scopeRumbleWatch struct {
	stopEffect func()
	pads       map[int]uint64
}

type runtimeState struct {
	root       *core.Scope
	src        driver.GamepadSource
	connected  *core.Signal[[]padIdentity]
	snapshot   []Pad
	snapSeq    uint64
	hasSnap    bool
	stopWatch  func()
	syncQueued atomic.Bool
	closed     bool

	nextGen uint64
	rumbles map[int]*activeRumble
	scopes  map[*core.Scope]*scopeRumbleWatch
}

var stateKey = core.NewKey[*runtimeState]("gamepad.state", nil)

// Pads returns the currently connected game controllers, sorted by ID.
// It is a tracked reactive read: scopes reading it rerender when controllers
// connect or disconnect, but not when buttons or axes change.
func Pads(cx *core.Scope) []Pad {
	root, seq, inFrame := core.FrameState(cx)
	if root == nil {
		return nil
	}
	st := ensureState(root)
	// Subscribe the caller's computation to connect/disconnect changes only.
	_ = st.connected.Get()
	if inFrame {
		st.pollForFrame(seq, false)
	} else {
		st.refreshSnapshot()
	}
	return slices.Clone(st.snapshot)
}

// State returns controller id's buttons and axes as of the current display
// frame, or (Pad{}, false) if no controller with that ID is connected.
// During a core.Scope.RequestAnimationFrame pass all calls observe the same
// frame snapshot; outside a display frame State polls the current values.
func State(cx *core.Scope, id int) (Pad, bool) {
	root, seq, inFrame := core.FrameState(cx)
	if root == nil {
		return Pad{}, false
	}
	st := ensureState(root)
	if inFrame {
		st.pollForFrame(seq, true)
	} else {
		st.refreshSnapshot()
	}
	return st.findPad(id)
}

// Rumble drives controller id's low-frequency (left/heavy) and high-frequency
// (right/light) rumble motors for duration, replacing any running rumble on
// that controller. Intensities are clamped to [0, 1] (NaN becomes 0).
//
// A controller that is disconnected or has CanRumble == false does nothing.
// Zero intensities or a non-positive duration stops any running rumble.
// Rumble stops automatically after duration, when the app enters the background
// or loses focus (cx.Env().Lifecycle() != core.LifecycleActive), when cx is
// disposed, or when the controller disconnects.
func Rumble(cx *core.Scope, id int, lowFreq, highFreq float32, duration time.Duration) {
	if cx == nil || cx.Disposed() {
		return
	}
	root, seq, inFrame := core.FrameState(cx)
	if root == nil {
		return
	}
	lowFreq = clamp01(lowFreq)
	highFreq = clamp01(highFreq)
	if duration <= 0 || (lowFreq == 0 && highFreq == 0) {
		StopRumble(cx, id)
		return
	}
	if core.Untracked(cx, func() core.Lifecycle { return cx.Env().Lifecycle() }) != core.LifecycleActive {
		return
	}
	st := ensureState(root)
	if inFrame {
		st.pollForFrame(seq, false)
	} else {
		st.refreshSnapshot()
	}
	pad, ok := st.findPad(id)
	if !ok || !pad.CanRumble {
		return
	}
	st.startRumble(cx, id, lowFreq, highFreq, duration)
}

// StopRumble stops any running rumble on controller id.
// A controller that is disconnected or has CanRumble == false does nothing.
func StopRumble(cx *core.Scope, id int) {
	if cx == nil {
		return
	}
	root, seq, inFrame := core.FrameState(cx)
	if root == nil {
		return
	}
	st := ensureState(root)
	if st.stopRumble(id) {
		return
	}
	if inFrame {
		st.pollForFrame(seq, false)
	} else {
		st.refreshSnapshot()
	}
	if pad, ok := st.findPad(id); ok && pad.CanRumble {
		st.src.StopRumble(id)
	}
}

func ensureState(root *core.Scope) *runtimeState {
	var st *runtimeState
	root.Run(func() {
		st = stateKey.Use(root)
		if st != nil {
			return
		}
		src := driver.GamepadSourceFor(root)
		if src == nil {
			src = platformSource()
		}
		initial := fromDriverPads(src.Poll())
		st = &runtimeState{
			root:      root,
			src:       src,
			connected: core.NewSignalFunc(root, padIdentities(initial), slices.Equal[[]padIdentity]),
			snapshot:  initial,
			rumbles:   map[int]*activeRumble{},
			scopes:    map[*core.Scope]*scopeRumbleWatch{},
		}
		st.stopWatch = src.Watch(func() {
			if st.syncQueued.CompareAndSwap(false, true) {
				root.Post(func() {
					st.syncQueued.Store(false)
					if st.closed || root.Disposed() {
						return
					}
					st.sync()
				})
			}
		})
		stateKey.Provide(root, st)
		root.Cleanup(st.close)
	})
	return st
}

func (st *runtimeState) close() {
	if st.closed {
		return
	}
	st.closed = true
	st.stopAllRumbles()
	if st.stopWatch != nil {
		st.stopWatch()
		st.stopWatch = nil
	}
}

func (st *runtimeState) findPad(id int) (Pad, bool) {
	for _, p := range st.snapshot {
		if p.ID == id {
			return p, true
		}
	}
	return Pad{}, false
}

func (st *runtimeState) watchScope(cx *core.Scope) *scopeRumbleWatch {
	if w, ok := st.scopes[cx]; ok {
		return w
	}
	w := &scopeRumbleWatch{pads: map[int]uint64{}}
	st.scopes[cx] = w
	cx.Run(func() {
		w.stopEffect = cx.Effect(func() {
			if cx.Env().Lifecycle() != core.LifecycleActive {
				st.stopScopeRumbles(cx)
			}
		})
		cx.Cleanup(func() {
			st.stopScopeRumbles(cx)
			if w.stopEffect != nil {
				w.stopEffect()
				w.stopEffect = nil
			}
			delete(st.scopes, cx)
		})
	})
	return w
}

func (st *runtimeState) startRumble(cx *core.Scope, id int, lowFreq, highFreq float32, duration time.Duration) {
	// Cancel any prior timer/ownership for this pad without emitting an extra stop.
	if prev, ok := st.rumbles[id]; ok {
		if prev.stopTimer != nil {
			prev.stopTimer()
		}
		if w, ok := st.scopes[prev.owner]; ok && w.pads[id] == prev.gen {
			delete(w.pads, id)
		}
		delete(st.rumbles, id)
	}

	w := st.watchScope(cx)
	st.nextGen++
	gen := st.nextGen
	var stopTimer func() bool
	cx.Run(func() {
		stopTimer = cx.AfterFunc(duration, func() {
			st.stopRumbleIfGen(id, gen)
		})
	})
	st.rumbles[id] = &activeRumble{
		gen:       gen,
		owner:     cx,
		stopTimer: stopTimer,
	}
	w.pads[id] = gen
	st.src.Rumble(id, lowFreq, highFreq, duration)
}

func (st *runtimeState) stopRumble(id int) bool {
	ar, ok := st.rumbles[id]
	if !ok {
		return false
	}
	delete(st.rumbles, id)
	if ar.stopTimer != nil {
		ar.stopTimer()
	}
	if w, ok := st.scopes[ar.owner]; ok && w.pads[id] == ar.gen {
		delete(w.pads, id)
	}
	st.src.StopRumble(id)
	return true
}

func (st *runtimeState) stopRumbleIfGen(id int, gen uint64) {
	ar, ok := st.rumbles[id]
	if !ok || ar.gen != gen {
		return
	}
	st.stopRumble(id)
}

func (st *runtimeState) stopScopeRumbles(cx *core.Scope) {
	w, ok := st.scopes[cx]
	if !ok || len(w.pads) == 0 {
		return
	}
	type pair struct {
		id  int
		gen uint64
	}
	list := make([]pair, 0, len(w.pads))
	for id, gen := range w.pads {
		list = append(list, pair{id: id, gen: gen})
	}
	for _, p := range list {
		st.stopRumbleIfGen(p.id, p.gen)
	}
}

func (st *runtimeState) stopAllRumbles() {
	if len(st.rumbles) == 0 {
		return
	}
	ids := make([]int, 0, len(st.rumbles))
	for id := range st.rumbles {
		ids = append(ids, id)
	}
	for _, id := range ids {
		st.stopRumble(id)
	}
}

func (st *runtimeState) pruneDisconnectedRumbles(pads []Pad) {
	if len(st.rumbles) == 0 {
		return
	}
	var gone []int
	for id := range st.rumbles {
		alive := false
		for _, p := range pads {
			if p.ID == id {
				alive = true
				break
			}
		}
		if !alive {
			gone = append(gone, id)
		}
	}
	for _, id := range gone {
		st.stopRumble(id)
	}
}

func (st *runtimeState) sync() {
	pads := fromDriverPads(st.src.Poll())
	st.snapshot = pads
	st.hasSnap = false
	st.pruneDisconnectedRumbles(pads)
	st.connected.Set(padIdentities(pads))
}

func (st *runtimeState) pollForFrame(seq uint64, updateSignal bool) {
	if st.hasSnap && st.snapSeq == seq {
		return
	}
	pads := fromDriverPads(st.src.Poll())
	st.snapshot = pads
	st.snapSeq = seq
	st.hasSnap = true
	st.pruneDisconnectedRumbles(pads)
	if updateSignal {
		st.connected.Set(padIdentities(pads))
	}
}

func (st *runtimeState) refreshSnapshot() {
	pads := fromDriverPads(st.src.Poll())
	st.snapshot = pads
	st.hasSnap = false
	st.pruneDisconnectedRumbles(pads)
}

func fromDriverPads(in []driver.GamepadState) []Pad {
	if len(in) == 0 {
		return nil
	}
	out := make([]Pad, len(in))
	for i, s := range in {
		out[i] = Pad{
			ID:        s.ID,
			Name:      s.Name,
			VendorID:  s.VendorID,
			ProductID: s.ProductID,
			Family:    Family(s.Family),
			CanRumble: s.CanRumble,
			Buttons:   s.Buttons,
			Axes:      s.Axes,
		}
	}
	slices.SortFunc(out, func(a, b Pad) int { return a.ID - b.ID })
	return out
}

func padIdentities(pads []Pad) []padIdentity {
	if len(pads) == 0 {
		return nil
	}
	ids := make([]padIdentity, len(pads))
	for i, p := range pads {
		ids[i] = padIdentity{
			ID:        p.ID,
			Name:      p.Name,
			VendorID:  p.VendorID,
			ProductID: p.ProductID,
			Family:    p.Family,
			CanRumble: p.CanRumble,
		}
	}
	return ids
}

func clamp01(v float32) float32 {
	if v <= 0 || math.IsNaN(float64(v)) {
		return 0
	}
	if v >= 1 {
		return 1
	}
	return v
}

func clampAxis(v float32) float32 {
	if math.IsNaN(float64(v)) {
		return 0
	}
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}

// normalizeRadialStick converts a signed 16-bit thumbstick pair (with +Y up,
// as XInput reports) into W3C axes in [-1, 1] with +Y down, applying a radial
// deadzone in [0, 32767].
func normalizeRadialStick(x, y int16, deadzone float64) (float32, float32) {
	fx, fy := float64(x), float64(y)
	mag := math.Hypot(fx, fy)
	if mag <= deadzone {
		return 0, 0
	}
	if mag > 32767 {
		mag = 32767
	}
	norm := (mag - deadzone) / (32767 - deadzone)
	nx := (fx / mag) * norm
	ny := -(fy / mag) * norm
	return clampAxis(float32(nx)), clampAxis(float32(ny))
}

// normalizeTrigger converts a raw trigger reading in [0, maxVal] with deadzone
// threshold into [0, 1].
func normalizeTrigger(val, threshold, maxVal float64) float32 {
	if val <= threshold || maxVal <= threshold {
		return 0
	}
	return clamp01(float32((val - threshold) / (maxVal - threshold)))
}

// normalizeEvdevAxis maps a Linux evdev absolute axis value in [minVal, maxVal]
// with center deadzone flat into [-1, 1].
func normalizeEvdevAxis(val, minVal, maxVal, flat int32) float32 {
	span := float64(maxVal) - float64(minVal)
	if span <= 0 {
		return 0
	}
	center := (float64(minVal) + float64(maxVal)) / 2
	half := span / 2
	delta := float64(val) - center
	if flat > 0 && math.Abs(delta) <= float64(flat) {
		return 0
	}
	if flat > 0 && half > float64(flat) {
		if delta > 0 {
			delta = (delta - float64(flat)) / (half - float64(flat))
		} else {
			delta = (delta + float64(flat)) / (half - float64(flat))
		}
		return clampAxis(float32(delta))
	}
	return clampAxis(float32(delta / half))
}

// normalizeEvdevTrigger maps a Linux evdev absolute trigger value in [minVal, maxVal] into [0, 1].
func normalizeEvdevTrigger(val, minVal, maxVal int32) float32 {
	span := float64(maxVal) - float64(minVal)
	if span <= 0 {
		return 0
	}
	return clamp01(float32((float64(val) - float64(minVal)) / span))
}
