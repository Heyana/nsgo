package nsgotest_test

import (
	"slices"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gamepad"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

func TestGamepadFocusNavigationMenuAndCaptureInput(t *testing.T) {
	t.Parallel()
	var pressed []string
	var backs int
	var inGame *core.Signal[bool]
	var rawSouthFrames int

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		gamepad.EnableFocusNavigation(cx)
		inGame = core.NewSignal(cx, false)
		cx.OnBackPress(func() bool {
			backs++
			return true
		})

		return core.View{
			Style: core.Style{Direction: core.Column, Gap: 10, Padding: core.All(16)},
			Children: []core.Node{
				ui.Button{TestID: "play", Text: "Play", OnPress: func() { pressed = append(pressed, "play") }},
				ui.Button{TestID: "disabled", Text: "Locked", Disabled: func() bool { return true }, OnPress: func() { pressed = append(pressed, "disabled") }},
				ui.Button{TestID: "settings", Text: "Settings", OnPress: func() { pressed = append(pressed, "settings") }},
				ui.Button{TestID: "quit", Text: "Quit", OnPress: func() { pressed = append(pressed, "quit") }},
				core.Show{When: inGame.Get, Then: func(gcx *core.Scope) core.Node {
					gamepad.CaptureInput(gcx)
					var loop func(time.Time)
					loop = func(time.Time) {
						if p, ok := gamepad.State(gcx, 0); ok && p.Pressed(gamepad.ButtonSouth) {
							rawSouthFrames++
						}
						gcx.RequestAnimationFrame(loop)
					}
					gcx.RequestAnimationFrame(loop)
					return core.View{TestID: "game-canvas", Style: core.Style{Height: core.Pt(40)}}
				}},
			},
		}
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Gamepads: true}), nsgotest.ManualFrames())

	app.ConnectGamepad(0, "Controller")

	// D-pad Down focuses the first stop ("play").
	app.SetGamepadButton(0, gamepad.ButtonDPadDown, 1)
	app.Advance(16 * time.Millisecond)
	if !app.Find("id:play").Focused {
		t.Fatalf("first D-pad Down did not focus play\n%s", app.Dump())
	}

	// Release D-pad Down, then press again: skips disabled button and lands on "settings".
	app.SetGamepadButton(0, gamepad.ButtonDPadDown, 0)
	app.Advance(16 * time.Millisecond)
	app.SetGamepadButton(0, gamepad.ButtonDPadDown, 1)
	app.Advance(16 * time.Millisecond)
	if !app.Find("id:settings").Focused {
		t.Fatalf("second D-pad Down did not skip disabled and focus settings\n%s", app.Dump())
	}
	app.SetGamepadButton(0, gamepad.ButtonDPadDown, 0)
	app.Advance(16 * time.Millisecond)

	// Left stick inside deadzone (0.4 < 0.5) must not move focus.
	app.SetGamepadAxis(0, gamepad.AxisLeftY, 0.4)
	app.Advance(16 * time.Millisecond)
	if !app.Find("id:settings").Focused {
		t.Fatal("left stick inside deadzone moved focus")
	}

	// Left stick past deadzone (0.75 >= 0.5) moves focus to "quit".
	app.SetGamepadAxis(0, gamepad.AxisLeftY, 0.75)
	app.Advance(16 * time.Millisecond)
	if !app.Find("id:quit").Focused {
		t.Fatalf("left stick down did not focus quit\n%s", app.Dump())
	}
	app.SetGamepadAxis(0, gamepad.AxisLeftY, 0)
	app.Advance(16 * time.Millisecond)

	// South button activates the focused ui.Button ("quit") once on rising edge.
	app.SetGamepadButton(0, gamepad.ButtonSouth, 1)
	app.Advance(16 * time.Millisecond)
	// Holding South across another frame must not re-fire.
	app.Advance(16 * time.Millisecond)
	if !slices.Equal(pressed, []string{"quit"}) {
		t.Fatalf("pressed = %v, want [quit]", pressed)
	}
	app.SetGamepadButton(0, gamepad.ButtonSouth, 0)
	app.Advance(16 * time.Millisecond)

	// East button triggers Back (OnBackPress).
	app.SetGamepadButton(0, gamepad.ButtonEast, 1)
	app.Advance(16 * time.Millisecond)
	app.Advance(16 * time.Millisecond)
	if backs != 1 {
		t.Fatalf("backs = %d, want 1", backs)
	}
	app.SetGamepadButton(0, gamepad.ButtonEast, 0)
	app.Advance(16 * time.Millisecond)

	// Mount gameplay view with CaptureInput: focus navigation pauses completely, while raw gamepad.State still reaches the game view.
	app.Post(func() { inGame.Set(true) })
	app.Settle()
	app.Find("id:game-canvas")

	app.SetGamepadButton(0, gamepad.ButtonDPadUp, 1)
	app.SetGamepadButton(0, gamepad.ButtonSouth, 1)
	app.SetGamepadButton(0, gamepad.ButtonEast, 1)
	app.Advance(16 * time.Millisecond)
	if !app.Find("id:quit").Focused {
		t.Fatal("CaptureInput did not pause focus movement")
	}
	if len(pressed) != 1 || backs != 1 {
		t.Fatalf("CaptureInput did not pause activation/back: pressed=%v backs=%d", pressed, backs)
	}
	if rawSouthFrames == 0 {
		t.Fatal("game view did not receive raw gamepad.State while CaptureInput was held")
	}

	// Unmount gameplay view while South is still held: CaptureInput releases automatically, held South must NOT misfire on resume, and new input works.
	app.SetGamepadButton(0, gamepad.ButtonDPadUp, 0)
	app.SetGamepadButton(0, gamepad.ButtonEast, 0)
	app.Post(func() { inGame.Set(false) })
	app.Settle()
	app.Advance(16 * time.Millisecond)
	if len(pressed) != 1 {
		t.Fatalf("held South misfired when resuming from CaptureInput: pressed=%v", pressed)
	}

	// Fresh D-pad Up moves focus back to "settings".
	app.SetGamepadButton(0, gamepad.ButtonSouth, 0)
	app.SetGamepadButton(0, gamepad.ButtonDPadUp, 1)
	app.Advance(16 * time.Millisecond)
	if !app.Find("id:settings").Focused {
		t.Fatalf("focus navigation did not resume after CaptureInput release\n%s", app.Dump())
	}
}

func TestGamepadFocusNavigationHeldRepeatAndOptIn(t *testing.T) {
	t.Parallel()
	// Without EnableFocusNavigation, gamepad input must not move focus.
	t.Run("disabled_by_default", func(t *testing.T) {
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Direction: core.Column}, Children: []core.Node{
				ui.Button{TestID: "a", Text: "A"},
				ui.Button{TestID: "b", Text: "B"},
			}}
		}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Gamepads: true}), nsgotest.ManualFrames())
		app.ConnectGamepad(0, "Pad")
		app.SetGamepadButton(0, gamepad.ButtonDPadDown, 1)
		app.Advance(32 * time.Millisecond)
		if app.Find("id:a").Focused || app.Find("id:b").Focused {
			t.Fatal("gamepad moved focus without EnableFocusNavigation")
		}
	})

	// Held D-pad repeats after initial delay at the configured repeat interval.
	t.Run("repeat_while_held", func(t *testing.T) {
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			gamepad.EnableFocusNavigation(cx, gamepad.WithRepeatDelay(100*time.Millisecond, 50*time.Millisecond))
			return core.View{Style: core.Style{Direction: core.Column, Gap: 8}, Children: []core.Node{
				ui.Button{TestID: "b0", Text: "Zero"},
				ui.Button{TestID: "b1", Text: "One"},
				ui.Button{TestID: "b2", Text: "Two"},
			}}
		}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Gamepads: true}), nsgotest.ManualFrames())

		app.ConnectGamepad(0, "Pad")
		app.SetGamepadButton(0, gamepad.ButtonDPadDown, 1)
		app.Advance(16 * time.Millisecond)
		if !app.Find("id:b0").Focused {
			t.Fatal("initial press did not focus b0")
		}
		// Before initial delay (100ms), focus stays on b0.
		app.Advance(64 * time.Millisecond)
		if !app.Find("id:b0").Focused {
			t.Fatal("repeated before initial delay")
		}
		// Past initial delay -> moves to b1.
		app.Advance(48 * time.Millisecond)
		if !app.Find("id:b1").Focused {
			t.Fatal("did not repeat to b1 after initial delay")
		}
		// Past repeat interval (50ms) -> moves to b2.
		app.Advance(64 * time.Millisecond)
		if !app.Find("id:b2").Focused {
			t.Fatal("did not repeat to b2 after repeat interval")
		}
	})
}

func TestGamepadFocusNavigationOnAndroidEnvironment(t *testing.T) {
	t.Parallel()
	var pressed []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		gamepad.EnableFocusNavigation(cx)
		return core.View{Style: core.Style{Direction: core.Column, Gap: 8}, Children: []core.Node{
			ui.Button{TestID: "one", Text: "One", OnPress: func() { pressed = append(pressed, "one") }},
			ui.Button{TestID: "two", Text: "Two", OnPress: func() { pressed = append(pressed, "two") }},
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSAndroid, Gamepads: true}), nsgotest.ManualFrames())

	app.ConnectGamepad(0, "Android Controller")
	app.SetGamepadButton(0, gamepad.ButtonDPadDown, 1)
	app.Advance(16 * time.Millisecond)
	app.SetGamepadButton(0, gamepad.ButtonDPadDown, 0)
	app.Advance(16 * time.Millisecond)
	app.SetGamepadButton(0, gamepad.ButtonDPadDown, 1)
	app.Advance(16 * time.Millisecond)
	app.SetGamepadButton(0, gamepad.ButtonDPadDown, 0)
	app.SetGamepadButton(0, gamepad.ButtonSouth, 1)
	app.Advance(16 * time.Millisecond)

	if !slices.Equal(pressed, []string{"two"}) {
		t.Fatalf("Android gamepad focus activation pressed = %v, want [two]", pressed)
	}
}
