package tsx_test

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gamepad"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/bundler"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
	"testing/fstest"
)

func TestGamepadDeclarationsIncluded(t *testing.T) {
	decl := tsx.Declarations()["types/nsgo/gamepad.d.ts"]
	for _, want := range []string{
		"export interface Pad",
		"export declare const Button:",
		"export declare const Axis:",
		"export declare function supported(): boolean;",
		"export declare function pads(): Pad[];",
		"export declare function state(id: number): Pad | null;",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("types/nsgo/gamepad.d.ts missing %q", want)
		}
	}
}

func TestGamepadTSXModuleReactivePadsAndFrameState(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("synthetic gamepads run headless only")
	}
	code, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(`
import { mount, signal, View, Text } from "nsgo";
import { supported, pads, state, Button, Axis } from "nsgo/gamepad";

let renders = 0;
const frame = signal("none");

function App() {
  const tick = () => {
    const p = state(0);
    if (p) {
      frame.set(p.name + ":" + p.buttons[Button.South] + ":" + p.buttons[Button.RT] + ":" + p.axes[Axis.LeftX]);
    } else {
      frame.set("none");
    }
    requestAnimationFrame(tick);
  };
  requestAnimationFrame(tick);

  return <View>
    <Text testID="supported">{() => "supported:" + supported()}</Text>
    <Text testID="constants">{() => Button.South + "," + Button.East + "," + Button.DPadUp + "," + Axis.LeftX + "," + Axis.RightY}</Text>
    <Text testID="pads">{() => {
      renders++;
      const list = pads();
      return renders + "|" + list.map((p) => p.id + ":" + p.name).join(",");
    }}</Text>
    <Text testID="frame">{frame}</Text>
  </View>;
}
mount(App);
`)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	app := nsgotest.Start(t, tsx.App("App.tsx", code),
		nsgotest.Environment(core.Environment{Gamepads: true}),
		nsgotest.ManualFrames(),
	)

	app.Find("text:supported:true")
	app.Find("text:0,1,12,0,3")
	app.Find("text:1|")

	app.ConnectGamepad(0, "DualSense")
	app.Find("text:2|0:DualSense")

	// Button and axis updates must not rerender scopes reading pads().
	app.SetGamepadButton(0, gamepad.ButtonSouth, 1)
	app.SetGamepadButton(0, gamepad.ButtonRT, 0.75)
	app.SetGamepadAxis(0, gamepad.AxisLeftX, -0.5)
	app.Advance(16 * time.Millisecond)

	app.Find("text:2|0:DualSense")
	app.Find("text:DualSense:1:0.75:-0.5")

	app.DisconnectGamepad(0)
	app.Advance(16 * time.Millisecond)
	app.Find("text:3|")
	app.Find("text:none")
}

func TestSandboxGamepadCapabilityGrantedAndDenied(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("synthetic gamepads run headless only")
	}
	const src = `
import { mount, signal, View, Text } from "nsgo";
import { supported, pads, state, Button, Axis } from "nsgo/gamepad";

const frame = signal("idle");
const errLog = signal("");

function App() {
  const tick = () => {
    try {
      const p = state(0);
      frame.set(p ? p.name + ":" + p.buttons[Button.A] + ":" + p.axes[Axis.LeftY] : "null");
    } catch (e: any) {
      frame.set("frame-err:" + e.code + " " + e.permission);
    }
    requestAnimationFrame(tick);
  };
  requestAnimationFrame(tick);

  return <View>
    <Text testID="supported">{() => "supported:" + supported()}</Text>
    <Text testID="pads">{() => {
      try {
        return "pads:" + pads().map((p) => p.id + ":" + p.name).join(",");
      } catch (e: any) {
        errLog.set(e.code + " " + e.permission);
        return "pads-err:" + e.code + " " + e.permission;
      }
    }}</Text>
    <Text testID="frame">{frame}</Text>
  </View>;
}
mount(App);
`
	t.Run("granted", func(t *testing.T) {
		p := verified(t, src, pkg.Capabilities{Gamepad: true})
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return tsx.View{Package: p, Sandbox: tsx.Sandbox{WebUnisolated: true}}
		}, nsgotest.Environment(core.Environment{Gamepads: true}), nsgotest.ManualFrames())

		app.Find("text:supported:true")
		app.Find("text:pads:")

		app.ConnectGamepad(0, "Xbox Pad")
		app.SetGamepadButton(0, gamepad.ButtonA, 1)
		app.SetGamepadAxis(0, gamepad.AxisLeftY, 0.5)
		app.Advance(16 * time.Millisecond)

		app.Find("text:pads:0:Xbox Pad")
		app.Find("text:Xbox Pad:1:0.5")
	})

	t.Run("denied_undeclared", func(t *testing.T) {
		p := verified(t, src, pkg.Capabilities{})
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return tsx.View{Package: p, Sandbox: tsx.Sandbox{WebUnisolated: true}}
		}, nsgotest.Environment(core.Environment{Gamepads: true}), nsgotest.ManualFrames())

		app.ConnectGamepad(0, "Xbox Pad")
		app.SetGamepadButton(0, gamepad.ButtonA, 1)
		app.Advance(16 * time.Millisecond)

		app.Find("text:supported:false")
		app.Find("text:pads-err:ERR_ACCESS_DENIED gamepad")
		app.Find("text:frame-err:ERR_ACCESS_DENIED gamepad")
	})

	t.Run("denied_by_host_grant", func(t *testing.T) {
		p := verified(t, src, pkg.Capabilities{Gamepad: true})
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return tsx.View{Package: p, Sandbox: tsx.Sandbox{Grant: &pkg.Capabilities{}, WebUnisolated: true}}
		}, nsgotest.Environment(core.Environment{Gamepads: true}), nsgotest.ManualFrames())

		app.ConnectGamepad(0, "Xbox Pad")
		app.Advance(16 * time.Millisecond)

		app.Find("text:supported:false")
		app.Find("text:pads-err:ERR_ACCESS_DENIED gamepad")
		app.Find("text:frame-err:ERR_ACCESS_DENIED gamepad")
	})
}
