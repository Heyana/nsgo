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

func TestFamilyStringAndDetectFamily(t *testing.T) {
	if got := FamilyGeneric.String(); got != "generic" {
		t.Errorf("FamilyGeneric.String() = %q, want generic", got)
	}
	if got := FamilyXbox.String(); got != "xbox" {
		t.Errorf("FamilyXbox.String() = %q, want xbox", got)
	}
	if got := FamilyPlayStation.String(); got != "playstation" {
		t.Errorf("FamilyPlayStation.String() = %q, want playstation", got)
	}
	if got := FamilyNintendo.String(); got != "nintendo" {
		t.Errorf("FamilyNintendo.String() = %q, want nintendo", got)
	}

	cases := []struct {
		name string
		vid  uint16
		pid  uint16
		want Family
	}{
		{"Microsoft VID Xbox Series", 0x045E, 0x0B13, FamilyXbox},
		{"Sony VID DualSense", 0x054C, 0x0CE6, FamilyPlayStation},
		{"Sony VID DualShock 4 generic name", 0x054C, 0x09CC, FamilyPlayStation},
		{"Nintendo VID Switch Pro", 0x057E, 0x2009, FamilyNintendo},
		{"Nintendo VID Joy-Con", 0x057E, 0x2006, FamilyNintendo},
		{"Logitech F310 XInput", 0x046D, 0xC21D, FamilyXbox},
		{"PDP Xbox third-party", 0x0E6F, 0x02A1, FamilyXbox},
		{"PDP Faceoff Deluxe Nintendo Switch", 0x0E6F, 0x0180, FamilyNintendo},
		{"DualSense Wireless Controller", 0, 0, FamilyPlayStation},
		{"DualShock 4", 0, 0, FamilyPlayStation},
		{"Xbox One", 0, 0, FamilyXbox},
		{"XInput Controller 1", 0, 0, FamilyXbox},
		{"Switch Pro Controller", 0, 0, FamilyNintendo},
		{"Pro Controller", 0, 0, FamilyNintendo},
		{"Joy-Con (L/R)", 0, 0, FamilyNintendo},
		{"Xbox Pro Controller", 0, 0, FamilyXbox},
		{"Generic USB Gamepad", 0x1234, 0x5678, FamilyGeneric},
		{"MFi", 0, 0, FamilyGeneric},
		{"", 0, 0, FamilyGeneric},
	}
	for _, tc := range cases {
		if got := DetectFamily(tc.vid, tc.pid, tc.name); got != tc.want {
			t.Errorf("DetectFamily(0x%04X, 0x%04X, %q) = %v, want %v", tc.vid, tc.pid, tc.name, got, tc.want)
		}
	}
}

func TestParseWebGamepadID(t *testing.T) {
	cases := []struct {
		id      string
		wantVID uint16
		wantPID uint16
		wantFam Family
	}{
		{
			id:      "Xbox Wireless Controller (STANDARD GAMEPAD Vendor: 045e Product: 0b13)",
			wantVID: 0x045E, wantPID: 0x0B13, wantFam: FamilyXbox,
		},
		{
			id:      "DualSense Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 0ce6)",
			wantVID: 0x054C, wantPID: 0x0CE6, wantFam: FamilyPlayStation,
		},
		{
			id:      "Wireless Controller (STANDARD GAMEPAD Vendor: 054c Product: 09cc)",
			wantVID: 0x054C, wantPID: 0x09CC, wantFam: FamilyPlayStation,
		},
		{
			id:      "Pro Controller (STANDARD GAMEPAD Vendor: 057e Product: 2009)",
			wantVID: 0x057E, wantPID: 0x2009, wantFam: FamilyNintendo,
		},
		{
			id:      "Generic Pad (STANDARD GAMEPAD Vendor: 1234 Product: 5678)",
			wantVID: 0x1234, wantPID: 0x5678, wantFam: FamilyGeneric,
		},
		{
			id:      "045e-0b13-Xbox Wireless Controller",
			wantVID: 0x045E, wantPID: 0x0B13, wantFam: FamilyXbox,
		},
		{
			id:      "54c-ce6-DualSense Wireless Controller",
			wantVID: 0x054C, wantPID: 0x0CE6, wantFam: FamilyPlayStation,
		},
		{
			id:      "54c-9cc-Wireless Controller",
			wantVID: 0x054C, wantPID: 0x09CC, wantFam: FamilyPlayStation,
		},
		{
			id:      "57e-2009-Pro Controller",
			wantVID: 0x057E, wantPID: 0x2009, wantFam: FamilyNintendo,
		},
		{
			id:      "1234-abcd-Generic Joystick",
			wantVID: 0x1234, wantPID: 0xABCD, wantFam: FamilyGeneric,
		},
		{
			id:      "DualSense Wireless Controller Extended Gamepad",
			wantVID: 0, wantPID: 0, wantFam: FamilyPlayStation,
		},
		{
			id:      "Unknown Controller",
			wantVID: 0, wantPID: 0, wantFam: FamilyGeneric,
		},
	}
	for _, tc := range cases {
		vid, pid := parseWebGamepadID(tc.id)
		if vid != tc.wantVID || pid != tc.wantPID {
			t.Errorf("parseWebGamepadID(%q) = (0x%04X, 0x%04X), want (0x%04X, 0x%04X)",
				tc.id, vid, pid, tc.wantVID, tc.wantPID)
		}
		if fam := DetectFamily(vid, pid, tc.id); fam != tc.wantFam {
			t.Errorf("DetectFamily for %q = %v, want %v", tc.id, fam, tc.wantFam)
		}
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
	rumbles   []RumbleEvent
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

func (m *mockSource) Rumble(id int, lowFreq, highFreq float32, duration time.Duration) {
	m.rumbles = append(m.rumbles, RumbleEvent{
		ID:       id,
		LowFreq:  lowFreq,
		HighFreq: highFreq,
		Duration: duration,
	})
}

func (m *mockSource) StopRumble(id int) {
	m.rumbles = append(m.rumbles, RumbleEvent{
		ID:      id,
		Stopped: true,
	})
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

func TestRumbleClampingReplaceAndAutoStop(t *testing.T) {
	clock := core.NewManualClock(time.Unix(0, 0))
	var posted []func()
	drain := func() {
		for len(posted) > 0 {
			q := posted
			posted = nil
			for _, fn := range q {
				fn()
			}
		}
	}
	root := core.NewRoot(core.RootOptions{
		Clock: clock,
		Post:  func(fn func()) { posted = append(posted, fn) },
	})
	defer root.Dispose()

	src := &mockSource{
		pads: []driver.GamepadState{
			{ID: 0, Name: "Xbox", VendorID: 0x045E, ProductID: 0x0B13, Family: driver.GamepadFamilyXbox, CanRumble: true},
			{ID: 1, Name: "Basic", CanRumble: false},
		},
	}
	unbind := driver.BindGamepadSource(root, src)
	defer unbind()

	// Pad with CanRumble == false and disconnected pad must be no-ops.
	Rumble(root, 1, 0.8, 0.8, 200*time.Millisecond)
	Rumble(root, 99, 0.8, 0.8, 200*time.Millisecond)
	StopRumble(root, 1)
	if len(src.rumbles) != 0 {
		t.Fatalf("expected no rumble events for non-rumble/missing pads, got %+v", src.rumbles)
	}

	// Clamping: >1 -> 1, <0 -> 0, NaN -> 0.
	Rumble(root, 0, 2.5, float32(math.NaN()), 500*time.Millisecond)
	if len(src.rumbles) != 1 {
		t.Fatalf("len(rumbles) = %d, want 1", len(src.rumbles))
	}
	ev := src.rumbles[0]
	if ev.ID != 0 || ev.LowFreq != 1 || ev.HighFreq != 0 || ev.Duration != 500*time.Millisecond || ev.Stopped {
		t.Fatalf("unexpected clamped rumble: %+v", ev)
	}

	// Replace active rumble after 200ms with a new 500ms rumble.
	clock.Advance(200 * time.Millisecond)
	drain()
	Rumble(root, 0, -0.4, 0.6, 500*time.Millisecond)
	if len(src.rumbles) != 2 {
		t.Fatalf("expected replacement without extra stop, got %+v", src.rumbles)
	}
	if src.rumbles[1].LowFreq != 0 || src.rumbles[1].HighFreq != 0.6 {
		t.Fatalf("unexpected replacement rumble: %+v", src.rumbles[1])
	}

	// Advancing 350ms (past the first rumble's 500ms deadline) must NOT stop the second rumble.
	clock.Advance(350 * time.Millisecond)
	drain()
	if len(src.rumbles) != 2 {
		t.Fatalf("first rumble timer leaked into replacement: %+v", src.rumbles)
	}

	// Advancing another 200ms completes the second rumble and auto-stops it.
	clock.Advance(200 * time.Millisecond)
	drain()
	if len(src.rumbles) != 3 || !src.rumbles[2].Stopped || src.rumbles[2].ID != 0 {
		t.Fatalf("expected auto-stop after replacement duration, got %+v", src.rumbles)
	}

	// Child scope disposal must automatically stop an active rumble started under it.
	child := root.Child()
	Rumble(child, 0, 0.5, 0.5, time.Second)
	if len(src.rumbles) != 4 {
		t.Fatalf("expected 4 events, got %+v", src.rumbles)
	}
	child.Dispose()
	if len(src.rumbles) != 5 || !src.rumbles[4].Stopped {
		t.Fatalf("expected auto-stop on child scope disposal, got %+v", src.rumbles)
	}

	// Pad disconnect while rumbling must automatically stop it.
	Rumble(root, 0, 0.7, 0.3, time.Second)
	if len(src.rumbles) != 6 {
		t.Fatalf("expected 6 events, got %+v", src.rumbles)
	}
	src.pads = nil
	src.onChange()
	drain()
	if len(src.rumbles) != 7 || !src.rumbles[6].Stopped {
		t.Fatalf("expected auto-stop on disconnect, got %+v", src.rumbles)
	}
}

func TestPlatformGamepadBridge(t *testing.T) {
	changes := 0
	stop := driver.WatchPlatformGamepads(func() { changes++ })
	defer stop()

	driver.ReportGamepadConnectInfo(42, "Android DualSense", 0x054C, 0x0CE6, true)
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
			if p.Name != "Android DualSense" || p.VendorID != 0x054C || p.ProductID != 0x0CE6 || !p.CanRumble ||
				p.Buttons[ButtonSouth] != 1 || p.Axes[AxisRightY] != -0.5 {
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

	var gotID int
	var gotLow, gotHigh float32
	var gotDur time.Duration
	driver.ConnectGamepadRumble(func(id int, low, high float32, dur time.Duration) {
		gotID, gotLow, gotHigh, gotDur = id, low, high, dur
	})
	defer driver.ConnectGamepadRumble(nil)

	driver.SendGamepadRumble(42, 0.75, 0.25, 300*time.Millisecond)
	if gotID != 42 || gotLow != 0.75 || gotHigh != 0.25 || gotDur != 300*time.Millisecond {
		t.Fatalf("unexpected rumble bridge call: id=%d low=%v high=%v dur=%v", gotID, gotLow, gotHigh, gotDur)
	}
}
