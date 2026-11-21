package nsgotest_test

import (
	"fmt"
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
