package driver

import (
	"slices"
	"sync"
)

const (
	// GamepadButtonCount is the number of buttons in the W3C Standard Gamepad layout.
	GamepadButtonCount = 17
	// GamepadAxisCount is the number of analog stick axes in the W3C Standard Gamepad layout.
	GamepadAxisCount = 4
)

// GamepadState is one connected game controller's state in the W3C Standard Gamepad layout.
type GamepadState struct {
	ID      int
	Name    string
	Buttons [GamepadButtonCount]float32
	Axes    [GamepadAxisCount]float32
}

// GamepadSource reads connected game controllers for an app or headless test host.
type GamepadSource interface {
	// Poll returns a snapshot of all currently connected controllers, sorted by ID.
	Poll() []GamepadState
	// Watch invokes onChange when a controller connects or disconnects. The returned
	// stop function unregisters the watcher and is safe to call more than once.
	Watch(onChange func()) (stop func())
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
}

// ReportGamepadConnect records that a game controller connected on the host platform.
func ReportGamepadConnect(id int, name string) {
	platformGamepads.mu.Lock()
	if platformGamepads.pads == nil {
		platformGamepads.pads = map[int]*GamepadState{}
	}
	if p, ok := platformGamepads.pads[id]; ok {
		p.Name = name
	} else {
		platformGamepads.pads[id] = &GamepadState{ID: id, Name: name}
	}
	watchers := snapshotGamepadWatchersLocked()
	platformGamepads.mu.Unlock()
	for _, fn := range watchers {
		fn()
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
