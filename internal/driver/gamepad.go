package driver

import (
	"slices"
	"sync"
	"time"
)

const (
	// GamepadButtonCount is the number of buttons in the W3C Standard Gamepad layout.
	GamepadButtonCount = 17
	// GamepadAxisCount is the number of analog stick axes in the W3C Standard Gamepad layout.
	GamepadAxisCount = 4
)

// GamepadFamily identifies a game controller's physical button-label family.
type GamepadFamily uint8

const (
	GamepadFamilyGeneric GamepadFamily = iota
	GamepadFamilyXbox
	GamepadFamilyPlayStation
	GamepadFamilyNintendo
)

// String returns the family's lowercase name: generic, xbox, playstation or nintendo.
func (f GamepadFamily) String() string {
	switch f {
	case GamepadFamilyXbox:
		return "xbox"
	case GamepadFamilyPlayStation:
		return "playstation"
	case GamepadFamilyNintendo:
		return "nintendo"
	default:
		return "generic"
	}
}

// GamepadState is one connected game controller's state in the W3C Standard Gamepad layout.
type GamepadState struct {
	ID        int
	Name      string
	VendorID  uint16
	ProductID uint16
	Family    GamepadFamily
	CanRumble bool
	Buttons   [GamepadButtonCount]float32
	Axes      [GamepadAxisCount]float32
}

// GamepadSource reads connected game controllers and drives rumble motors for
// an app or headless test host.
type GamepadSource interface {
	// Poll returns a snapshot of all currently connected controllers, sorted by ID.
	Poll() []GamepadState
	// Watch invokes onChange when a controller connects or disconnects. The returned
	// stop function unregisters the watcher and is safe to call more than once.
	Watch(onChange func()) (stop func())
	// Rumble drives controller id's low-frequency (left/heavy) and high-frequency
	// (right/light) motors for duration, replacing any running rumble.
	Rumble(id int, lowFreq, highFreq float32, duration time.Duration)
	// StopRumble stops any running rumble on controller id.
	StopRumble(id int)
}

// boundGamepads holds per-root synthetic GamepadSources registered by nsgotest,
// keyed by the graph's root *core.Scope so parallel tests stay isolated.
var boundGamepads sync.Map // map[any]GamepadSource

// BindGamepadSource binds src to key (the root scope of a headless test app) and
// returns a function that removes the binding.
func BindGamepadSource(key any, src GamepadSource) func() {
	if key == nil || src == nil {
		return func() {}
	}
	boundGamepads.Store(key, src)
	return func() { boundGamepads.Delete(key) }
}

// GamepadSourceFor returns the synthetic GamepadSource bound to key, or nil if none.
func GamepadSourceFor(key any) GamepadSource {
	if key == nil {
		return nil
	}
	v, ok := boundGamepads.Load(key)
	if !ok {
		return nil
	}
	return v.(GamepadSource)
}

// platformGamepads is the process-wide bridge for backends whose controller input
// arrives through the runtime pipe rather than direct Go syscalls (Android).
var platformGamepads struct {
	mu       sync.Mutex
	pads     map[int]*GamepadState
	watchers map[uint64]func()
	nextID   uint64
	rumble   func(id int, lowFreq, highFreq float32, duration time.Duration)
}

// ReportGamepadConnect records that a game controller connected on the host platform.
func ReportGamepadConnect(id int, name string) {
	ReportGamepadConnectInfo(id, name, 0, 0, false)
}

// ReportGamepadConnectInfo records that a game controller connected on the host
// platform with its USB vendor/product IDs and rumble capability.
func ReportGamepadConnectInfo(id int, name string, vendorID, productID uint16, canRumble bool) {
	platformGamepads.mu.Lock()
	if platformGamepads.pads == nil {
		platformGamepads.pads = map[int]*GamepadState{}
	}
	if p, ok := platformGamepads.pads[id]; ok {
		p.Name = name
		p.VendorID = vendorID
		p.ProductID = productID
		p.CanRumble = canRumble
	} else {
		platformGamepads.pads[id] = &GamepadState{
			ID:        id,
			Name:      name,
			VendorID:  vendorID,
			ProductID: productID,
			CanRumble: canRumble,
		}
	}
	watchers := snapshotGamepadWatchersLocked()
	platformGamepads.mu.Unlock()
	for _, fn := range watchers {
		fn()
	}
}

// ConnectGamepadRumble registers the host platform's rumble handler (Android's pipe writer).
func ConnectGamepadRumble(fn func(id int, lowFreq, highFreq float32, duration time.Duration)) {
	platformGamepads.mu.Lock()
	platformGamepads.rumble = fn
	platformGamepads.mu.Unlock()
}

// SendGamepadRumble forwards a rumble command (or stop when duration <= 0) to the
// host platform's registered handler, if any.
func SendGamepadRumble(id int, lowFreq, highFreq float32, duration time.Duration) {
	platformGamepads.mu.Lock()
	fn := platformGamepads.rumble
	platformGamepads.mu.Unlock()
	if fn != nil {
		fn(id, lowFreq, highFreq, duration)
	}
}

// ReportGamepadDisconnect records that a game controller disconnected on the host platform.
func ReportGamepadDisconnect(id int) {
	platformGamepads.mu.Lock()
	if _, ok := platformGamepads.pads[id]; !ok {
		platformGamepads.mu.Unlock()
		return
	}
	delete(platformGamepads.pads, id)
	watchers := snapshotGamepadWatchersLocked()
	platformGamepads.mu.Unlock()
	for _, fn := range watchers {
		fn()
	}
}

// ReportGamepadState replaces the buttons and axes of a connected controller.
func ReportGamepadState(id int, buttons [GamepadButtonCount]float32, axes [GamepadAxisCount]float32) {
	platformGamepads.mu.Lock()
	if p, ok := platformGamepads.pads[id]; ok {
		p.Buttons = buttons
		p.Axes = axes
	}
	platformGamepads.mu.Unlock()
}

// ReportGamepadButton updates a single button on a connected controller.
func ReportGamepadButton(id, index int, value float32) {
	if index < 0 || index >= GamepadButtonCount {
		return
	}
	platformGamepads.mu.Lock()
	if p, ok := platformGamepads.pads[id]; ok {
		p.Buttons[index] = value
	}
	platformGamepads.mu.Unlock()
}

// ReportGamepadAxis updates a single axis on a connected controller.
func ReportGamepadAxis(id, index int, value float32) {
	if index < 0 || index >= GamepadAxisCount {
		return
	}
	platformGamepads.mu.Lock()
	if p, ok := platformGamepads.pads[id]; ok {
		p.Axes[index] = value
	}
	platformGamepads.mu.Unlock()
}

// PollPlatformGamepads returns a sorted snapshot of controllers reported through the platform bridge.
func PollPlatformGamepads() []GamepadState {
	platformGamepads.mu.Lock()
	defer platformGamepads.mu.Unlock()
	if len(platformGamepads.pads) == 0 {
		return nil
	}
	out := make([]GamepadState, 0, len(platformGamepads.pads))
	for _, p := range platformGamepads.pads {
		out = append(out, *p)
	}
	slices.SortFunc(out, func(a, b GamepadState) int { return a.ID - b.ID })
	return out
}

// WatchPlatformGamepads registers fn to be called when controllers connect or disconnect
// on the platform bridge.
func WatchPlatformGamepads(fn func()) func() {
	if fn == nil {
		return func() {}
	}
	platformGamepads.mu.Lock()
	if platformGamepads.watchers == nil {
		platformGamepads.watchers = map[uint64]func(){}
	}
	platformGamepads.nextID++
	id := platformGamepads.nextID
	platformGamepads.watchers[id] = fn
	platformGamepads.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			platformGamepads.mu.Lock()
			delete(platformGamepads.watchers, id)
			platformGamepads.mu.Unlock()
		})
	}
}

func snapshotGamepadWatchersLocked() []func() {
	if len(platformGamepads.watchers) == 0 {
		return nil
	}
	out := make([]func(), 0, len(platformGamepads.watchers))
	for _, fn := range platformGamepads.watchers {
		out = append(out, fn)
	}
	return out
}
