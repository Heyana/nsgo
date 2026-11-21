package nsgotest_test

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gamepad"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestGamepadConnectDisconnectAndReactivePads(t *testing.T) {
	t.Parallel()
	renders := 0
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Children: []core.Node{
			core.Text{TestID: "support", ContentFunc: func() string {
				return fmt.Sprintf("supported:%v", cx.Env().SupportsGamepads())
			}},
			core.Text{TestID: "pads", ContentFunc: func() string {
				renders++
				pads := gamepad.Pads(cx)
				if len(pads) == 0 {
					return "none"
				}
				names := make([]string, len(pads))
				for i, p := range pads {
					names[i] = fmt.Sprintf("%d:%s", p.ID, p.Name)
				}
				return strings.Join(names, ",")
			}},
		}}
	}, nsgotest.Environment(core.Environment{Gamepads: true}))

	app.Find("text:supported:true")
	app.Find("text:none")
	if renders != 1 {
		t.Fatalf("initial renders = %d, want 1", renders)
	}

	app.ConnectGamepad(1, "DualSense")
	app.Find("text:1:DualSense")
	if renders != 2 {
		t.Fatalf("renders after connect = %d, want 2", renders)
	}

	// Button and axis changes must NOT rerender scopes reading gamepad.Pads.
	app.SetGamepadButton(1, gamepad.ButtonA, 1)
	app.SetGamepadAxis(1, gamepad.AxisLeftX, 0.5)
	app.Settle()
	if renders != 2 {
		t.Fatalf("button/axis changes rerendered Pads scope: renders = %d, want 2", renders)
	}

	app.ConnectGamepad(0, "Xbox Controller")
	app.Find("text:0:Xbox Controller,1:DualSense")
	if renders != 3 {
		t.Fatalf("renders after second connect = %d, want 3", renders)
	}

	app.DisconnectGamepad(0)
	app.Find("text:1:DualSense")
	if renders != 4 {
		t.Fatalf("renders after disconnect = %d, want 4", renders)
	}

	app.DisconnectGamepad(1)
	app.Find("text:none")
}

func TestGamepadFrameStateInRequestAnimationFrame(t *testing.T) {
	t.Parallel()
	var seen []gamepad.Pad
	var okSeen []bool

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		var loop func(time.Time)
		loop = func(time.Time) {
			p, ok := gamepad.State(cx, 0)
			seen = append(seen, p)
			okSeen = append(okSeen, ok)
			cx.RequestAnimationFrame(loop)
		}
		cx.RequestAnimationFrame(loop)
		return core.View{}
	}, nsgotest.ManualFrames())

	app.ConnectGamepad(0, "Pro Controller")
	app.SetGamepadButton(0, gamepad.ButtonSouth, 1)
	app.SetGamepadButton(0, gamepad.ButtonRT, 0.8)
	app.SetGamepadAxis(0, gamepad.AxisLeftX, -0.75)
	app.SetGamepadAxis(0, gamepad.AxisRightY, 0.5)

	app.Advance(16 * time.Millisecond)
	if len(seen) == 0 || !okSeen[len(okSeen)-1] {
		t.Fatalf("expected connected pad in frame callback, got len=%d ok=%v", len(seen), okSeen)
	}
	p := seen[len(seen)-1]
	if !p.Pressed(gamepad.ButtonA) || !p.Pressed(gamepad.ButtonRT) || p.Pressed(gamepad.ButtonB) {
		t.Fatalf("unexpected buttons: %+v", p.Buttons)
	}
	if p.Buttons[gamepad.ButtonRT] != 0.8 || p.Axes[gamepad.AxisLeftX] != -0.75 || p.Axes[gamepad.AxisRightY] != 0.5 {
		t.Fatalf("unexpected analog values: buttons=%+v axes=%+v", p.Buttons, p.Axes)
	}

	app.SetGamepadButton(0, gamepad.ButtonSouth, 0)
	app.SetGamepadAxis(0, gamepad.AxisLeftX, 0)
	app.Advance(16 * time.Millisecond)
	p = seen[len(seen)-1]
	if p.Pressed(gamepad.ButtonA) || p.Axes[gamepad.AxisLeftX] != 0 {
		t.Fatalf("expected released state on next frame: %+v", p)
	}

	app.DisconnectGamepad(0)
	app.Advance(16 * time.Millisecond)
	if okSeen[len(okSeen)-1] {
		t.Fatal("expected State to report false after disconnect")
	}
}

func TestGamepadFamilyAndInfo(t *testing.T) {
	t.Parallel()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Text{TestID: "info", ContentFunc: func() string {
			pads := gamepad.Pads(cx)
			if len(pads) == 0 {
				return "none"
			}
			p := pads[0]
			return fmt.Sprintf("%s:%04x:%04x:%v", p.Family, p.VendorID, p.ProductID, p.CanRumble)
		}}
	})

	app.ConnectGamepad(0, "DualSense Wireless Controller")
	app.Find("text:playstation:0000:0000:false")

	app.SetGamepadInfo(0, gamepad.FamilyPlayStation, 0x054C, 0x0CE6, true)
	app.Find("text:playstation:054c:0ce6:true")

	app.SetGamepadInfo(0, gamepad.FamilyNintendo, 0x057E, 0x2009, false)
	app.Find("text:nintendo:057e:2009:false")
}

func TestGamepadRumbleRecordingAndLifecycle(t *testing.T) {
	t.Parallel()
	var rootCx *core.Scope
	var childCx *core.Scope
	var showChild *core.Signal[bool]

	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		rootCx = cx
		showChild = core.NewSignal(cx, true)
		return core.Show{
			When: showChild.Get,
			Then: func(sc *core.Scope) core.Node {
				childCx = sc
				return core.View{TestID: "child"}
			},
		}
	}, nsgotest.ManualFrames())

	app.ConnectGamepad(0, "Xbox Wireless Controller")
	app.SetGamepadInfo(0, gamepad.FamilyXbox, 0x045E, 0x0B13, true)
	app.ConnectGamepad(1, "No Rumble Pad")
	app.SetGamepadInfo(1, gamepad.FamilyGeneric, 0x1234, 0x5678, false)

	// Pad without rumble does nothing and records nothing.
	app.Post(func() {
		gamepad.Rumble(rootCx, 1, 1.0, 1.0, 200*time.Millisecond)
		gamepad.StopRumble(rootCx, 1)
	})
	app.Settle()
	if got := app.GamepadRumbles(); len(got) != 0 {
		t.Fatalf("expected no rumbles on non-rumble pad, got %+v", got)
	}
	if _, ok := app.LastGamepadRumble(0); ok {
		t.Fatal("expected no rumble yet for pad 0")
	}

	// Clamping + timed auto-stop.
	app.Post(func() {
		gamepad.Rumble(rootCx, 0, 1.5, float32(math.NaN()), 200*time.Millisecond)
	})
	app.Settle()
	last, ok := app.LastGamepadRumble(0)
	if !ok || last.Stopped || last.LowFreq != 1 || last.HighFreq != 0 || last.Duration != 200*time.Millisecond {
		t.Fatalf("unexpected clamped rumble: ok=%v ev=%+v", ok, last)
	}

	// Replace before expiry; original 200ms timer must not stop the replacement.
	app.Advance(100 * time.Millisecond)
	app.Post(func() {
		gamepad.Rumble(rootCx, 0, 0.25, 0.75, 300*time.Millisecond)
	})
	app.Settle()
	app.Advance(150 * time.Millisecond)
	if last, _ = app.LastGamepadRumble(0); last.Stopped {
		t.Fatalf("replacement rumble stopped prematurely: %+v", app.GamepadRumbles())
	}
	app.Advance(200 * time.Millisecond)
	if last, _ = app.LastGamepadRumble(0); !last.Stopped {
		t.Fatalf("replacement rumble did not auto-stop after duration: %+v", app.GamepadRumbles())
	}

	// Going to background stops active rumble and prevents starting a new one.
	app.Post(func() {
		gamepad.Rumble(rootCx, 0, 0.6, 0.4, time.Second)
	})
	app.Settle()
	if last, _ = app.LastGamepadRumble(0); last.Stopped {
		t.Fatalf("expected active rumble before background: %+v", last)
	}
	app.SetEnvironment(core.Environment{Lifecycle: core.LifecycleBackground})
	if last, _ = app.LastGamepadRumble(0); !last.Stopped {
		t.Fatalf("expected auto-stop on background lifecycle: %+v", app.GamepadRumbles())
	}
	countBefore := len(app.GamepadRumbles())
	app.Post(func() {
		gamepad.Rumble(rootCx, 0, 0.9, 0.9, time.Second)
	})
	app.Settle()
	if len(app.GamepadRumbles()) != countBefore {
		t.Fatalf("rumble started while in background: %+v", app.GamepadRumbles())
	}

	// Returning to active allows rumble; losing focus (LifecycleInactive) stops it.
	app.SetEnvironment(core.Environment{Lifecycle: core.LifecycleActive})
	app.Post(func() {
		gamepad.Rumble(rootCx, 0, 0.5, 0.5, time.Second)
	})
	app.Settle()
	if last, _ = app.LastGamepadRumble(0); last.Stopped {
		t.Fatalf("expected rumble when active: %+v", last)
	}
	app.SetEnvironment(core.Environment{Lifecycle: core.LifecycleInactive})
	if last, _ = app.LastGamepadRumble(0); !last.Stopped {
		t.Fatalf("expected auto-stop on window focus loss (inactive): %+v", app.GamepadRumbles())
	}

	// Scope disposal stops active rumble started under childCx.
	app.SetEnvironment(core.Environment{Lifecycle: core.LifecycleActive})
	app.Post(func() {
		gamepad.Rumble(childCx, 0, 0.4, 0.8, time.Second)
	})
	app.Settle()
	if last, _ = app.LastGamepadRumble(0); last.Stopped {
		t.Fatalf("expected child rumble active: %+v", last)
	}
	app.Post(func() {
		showChild.Set(false)
	})
	app.Settle()
	if last, _ = app.LastGamepadRumble(0); !last.Stopped {
		t.Fatalf("expected auto-stop when child scope disposed: %+v", app.GamepadRumbles())
	}

	// Disconnecting a rumbling pad stops rumble automatically.
	app.Post(func() {
		gamepad.Rumble(rootCx, 0, 0.8, 0.2, time.Second)
	})
	app.Settle()
	if last, _ = app.LastGamepadRumble(0); last.Stopped {
		t.Fatalf("expected rumble active before disconnect: %+v", last)
	}
	app.DisconnectGamepad(0)
	if last, _ = app.LastGamepadRumble(0); !last.Stopped {
		t.Fatalf("expected auto-stop on pad disconnect: %+v", app.GamepadRumbles())
	}
}
