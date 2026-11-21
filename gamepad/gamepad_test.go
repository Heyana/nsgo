package gamepad

import (
	"math"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestW3CConstantsAndPressed(t *testing.T) {
	if ButtonCount != 17 || AxisCount != 4 {
		t.Fatalf("counts: buttons=%d axes=%d", ButtonCount, AxisCount)
	}
	if ButtonA != ButtonSouth || ButtonB != ButtonEast || ButtonX != ButtonWest || ButtonY != ButtonNorth || ButtonSelect != ButtonBack {
		t.Fatal("alias constants do not match W3C directional buttons")
	}
	if ButtonSouth != 0 || ButtonEast != 1 || ButtonWest != 2 || ButtonNorth != 3 ||
		ButtonLB != 4 || ButtonRB != 5 || ButtonLT != 6 || ButtonRT != 7 ||
		ButtonBack != 8 || ButtonStart != 9 || ButtonLeftStick != 10 || ButtonRightStick != 11 ||
		ButtonDPadUp != 12 || ButtonDPadDown != 13 || ButtonDPadLeft != 14 || ButtonDPadRight != 15 ||
		ButtonHome != 16 {
		t.Fatal("button indices do not match W3C Standard Gamepad layout")
	}
	if AxisLeftX != 0 || AxisLeftY != 1 || AxisRightX != 2 || AxisRightY != 3 {
		t.Fatal("axis indices do not match W3C Standard Gamepad layout")
	}

	var p Pad
	p.Buttons[ButtonA] = 0.49
	p.Buttons[ButtonRT] = 0.5
	p.Buttons[ButtonStart] = 1.0
	if p.Pressed(ButtonA) {
		t.Error("0.49 should not be pressed")
	}
	if !p.Pressed(ButtonRT) || !p.Pressed(ButtonStart) {
		t.Error("0.5 and 1.0 should be pressed")
	}
	if p.Pressed(Button(99)) {
		t.Error("out-of-range button must not panic or report pressed")
	}
}

func TestNormalizationHelpers(t *testing.T) {
	// Radial stick with XInput deadzone (7849) and Y inversion (+Y up -> -Y up).
	if x, y := normalizeRadialStick(3000, -4000, 7849); x != 0 || y != 0 {
		t.Errorf("inside deadzone got (%v, %v), want (0, 0)", x, y)
	}
	if x, y := normalizeRadialStick(32767, 0, 7849); math.Abs(float64(x)-1) > 1e-4 || y != 0 {
		t.Errorf("full right got (%v, %v), want (1, 0)", x, y)
	}
	if x, y := normalizeRadialStick(0, 32767, 7849); x != 0 || math.Abs(float64(y)+1) > 1e-4 {
		t.Errorf("full up got (%v, %v), want (0, -1)", x, y)
	}
	if x, y := normalizeRadialStick(0, -32768, 7849); x != 0 || math.Abs(float64(y)-1) > 1e-4 {
		t.Errorf("full down got (%v, %v), want (0, 1)", x, y)
	}

	// Trigger deadzone.
	if v := normalizeTrigger(30, 30, 255); v != 0 {
		t.Errorf("trigger at threshold = %v, want 0", v)
	}
	if v := normalizeTrigger(255, 30, 255); v != 1 {
		t.Errorf("trigger at max = %v, want 1", v)
	}

	// Evdev axis & trigger normalization.
	if v := normalizeEvdevAxis(128, 0, 255, 16); v != 0 {
		t.Errorf("evdev centered axis = %v, want 0", v)
	}
	if v := normalizeEvdevAxis(0, 0, 255, 16); math.Abs(float64(v)+1) > 1e-4 {
		t.Errorf("evdev min axis = %v, want -1", v)
	}
	if v := normalizeEvdevAxis(255, 0, 255, 16); math.Abs(float64(v)-1) > 1e-4 {
		t.Errorf("evdev max axis = %v, want 1", v)
	}
	if v := normalizeEvdevTrigger(0, 0, 255); v != 0 {
		t.Errorf("evdev min trigger = %v, want 0", v)
	}
	if v := normalizeEvdevTrigger(255, 0, 255); v != 1 {
		t.Errorf("evdev max trigger = %v, want 1", v)
	}
}

type mockSource struct {
	pads      []driver.GamepadState
	polls     int
	onChange  func()
	unwatched bool
}

func (m *mockSource) Poll() []driver.GamepadState {
	m.polls++
	out := make([]driver.GamepadState, len(m.pads))
	copy(out, m.pads)
	return out
}

func (m *mockSource) Watch(fn func()) func() {
	m.onChange = fn
	return func() { m.unwatched = true }
}

func TestFrameSnapshotCachesPerDisplayFrame(t *testing.T) {
	var posted []func()
	root := core.NewRoot(core.RootOptions{
		Post: func(fn func()) { posted = append(posted, fn) },
	})
	src := &mockSource{
		pads: []driver.GamepadState{{ID: 0, Name: "Controller"}},
	}
	unbind := driver.BindGamepadSource(root, src)
	defer unbind()

	var first, second, nextFrame Pad
	root.RequestAnimationFrame(func(_ time.Time) {
		first, _ = State(root, 0)
		// Mutate underlying hardware state mid-frame; State must keep the same frame snapshot.
		src.pads[0].Buttons[ButtonA] = 1
		src.pads[0].Axes[AxisLeftX] = 0.75
		second, _ = State(root, 0)
	})
	pollsBefore := src.polls
	root.RunFrame()
	if first.Pressed(ButtonA) || second.Pressed(ButtonA) {
		t.Fatalf("mid-frame change leaked into frame snapshot: first=%+v second=%+v", first, second)
	}
	// Initial ensureState poll + 1 poll for frame 1.
	if src.polls-pollsBefore > 2 {
		t.Fatalf("expected at most one poll per display frame after init, got %d", src.polls-pollsBefore)
	}

	root.RequestAnimationFrame(func(_ time.Time) {
		nextFrame, _ = State(root, 0)
	})
	root.RunFrame()
	if !nextFrame.Pressed(ButtonA) || nextFrame.Axes[AxisLeftX] != 0.75 {
		t.Fatalf("next frame did not pick up updated state: %+v", nextFrame)
	}

	root.Dispose()
	if !src.unwatched {
		t.Error("disposing root did not stop gamepad watcher")
	}
}

func TestPlatformGamepadBridge(t *testing.T) {
	changes := 0
	stop := driver.WatchPlatformGamepads(func() { changes++ })
	defer stop()

	driver.ReportGamepadConnect(42, "Android Pad")
	defer driver.ReportGamepadDisconnect(42)

	var btns [driver.GamepadButtonCount]float32
	btns[ButtonSouth] = 1
	var axes [driver.GamepadAxisCount]float32
	axes[AxisRightY] = -0.5
	driver.ReportGamepadState(42, btns, axes)

	pads := driver.PollPlatformGamepads()
	found := false
	for _, p := range pads {
		if p.ID == 42 {
			found = true
			if p.Name != "Android Pad" || p.Buttons[ButtonSouth] != 1 || p.Axes[AxisRightY] != -0.5 {
				t.Fatalf("unexpected pad state: %+v", p)
			}
		}
	}
	if !found {
		t.Fatal("reported pad not returned by PollPlatformGamepads")
	}
	if changes == 0 {
		t.Error("watcher was not notified on connect")
	}
}
