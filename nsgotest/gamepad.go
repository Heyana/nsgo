package nsgotest

import (
	"math"
	"slices"
	"sync"

	"github.com/nsgo-dev/nsgo/gamepad"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakeGamepads struct {
	mu       sync.Mutex
	pads     map[int]*driver.GamepadState
	watchers map[uint64]func()
	nextID   uint64
}

func newFakeGamepads() *fakeGamepads {
	return &fakeGamepads{
		pads:     map[int]*driver.GamepadState{},
		watchers: map[uint64]func(){},
	}
}

func (f *fakeGamepads) Poll() []driver.GamepadState {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.pads) == 0 {
		return nil
	}
	out := make([]driver.GamepadState, 0, len(f.pads))
	for _, p := range f.pads {
		out = append(out, *p)
	}
	slices.SortFunc(out, func(a, b driver.GamepadState) int { return a.ID - b.ID })
	return out
}

func (f *fakeGamepads) Watch(onChange func()) func() {
	if onChange == nil {
		return func() {}
	}
	f.mu.Lock()
	f.nextID++
	id := f.nextID
	f.watchers[id] = onChange
	f.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			f.mu.Lock()
			delete(f.watchers, id)
			f.mu.Unlock()
		})
	}
}

func (f *fakeGamepads) notify() {
	f.mu.Lock()
	ws := make([]func(), 0, len(f.watchers))
	for _, fn := range f.watchers {
		ws = append(ws, fn)
	}
	f.mu.Unlock()
	for _, fn := range ws {
		fn()
	}
}

func (a *App) requireGamepadModel() *fakeGamepads {
	a.t.Helper()
	if a.owner != nil {
		return a.owner.requireGamepadModel()
	}
	if a.target != nil {
		a.t.Skip("nsgotest: synthetic gamepads run headless only")
	}
	return a.gamepads
}

// ConnectGamepad connects a fake game controller with the given ID and name
// (or updates its name if already connected) and settles the app so scopes
// reading gamepad.Pads rerender.
func (a *App) ConnectGamepad(id int, name string) {
	a.t.Helper()
	f := a.requireGamepadModel()
	f.mu.Lock()
	if p, ok := f.pads[id]; ok {
		p.Name = name
	} else {
		f.pads[id] = &driver.GamepadState{ID: id, Name: name}
	}
	f.mu.Unlock()
	f.notify()
	a.Settle()
}

// DisconnectGamepad disconnects the fake game controller with the given ID
// and settles the app so scopes reading gamepad.Pads rerender.
func (a *App) DisconnectGamepad(id int) {
	a.t.Helper()
	f := a.requireGamepadModel()
	f.mu.Lock()
	_, ok := f.pads[id]
	delete(f.pads, id)
	f.mu.Unlock()
	if ok {
		f.notify()
	}
	a.Settle()
}

// SetGamepadButton sets button btn on connected fake controller id to value
// (clamped to [0, 1]). Button updates do not trigger connect/disconnect rerenders
// of gamepad.Pads; read them from gamepad.State inside RequestAnimationFrame.
func (a *App) SetGamepadButton(id int, btn gamepad.Button, value float32) {
	a.t.Helper()
	if int(btn) >= gamepad.ButtonCount {
		a.t.Fatalf("nsgotest: gamepad button %d out of range [0, %d)", btn, gamepad.ButtonCount)
	}
	f := a.requireGamepadModel()
	f.mu.Lock()
	p, ok := f.pads[id]
	if ok {
		p.Buttons[btn] = clampGamepad01(value)
	}
	f.mu.Unlock()
	if !ok {
		a.t.Fatalf("nsgotest: gamepad %d is not connected", id)
	}
}

// SetGamepadAxis sets axis on connected fake controller id to value
// (clamped to [-1, 1]). Axis updates do not trigger connect/disconnect rerenders
// of gamepad.Pads; read them from gamepad.State inside RequestAnimationFrame.
func (a *App) SetGamepadAxis(id int, axis gamepad.Axis, value float32) {
	a.t.Helper()
	if int(axis) >= gamepad.AxisCount {
		a.t.Fatalf("nsgotest: gamepad axis %d out of range [0, %d)", axis, gamepad.AxisCount)
	}
	f := a.requireGamepadModel()
	f.mu.Lock()
	p, ok := f.pads[id]
	if ok {
		p.Axes[axis] = clampGamepadAxis(value)
	}
	f.mu.Unlock()
	if !ok {
		a.t.Fatalf("nsgotest: gamepad %d is not connected", id)
	}
}

func clampGamepad01(v float32) float32 {
	if math.IsNaN(float64(v)) || v <= 0 {
		return 0
	}
	if v >= 1 {
		return 1
	}
	return v
}

func clampGamepadAxis(v float32) float32 {
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
