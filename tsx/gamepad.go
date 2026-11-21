package tsx

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/gamepad"
	"github.com/nsgo-dev/nsgo/tsx/internal/js"
)

// nsgo/gamepad exposes connected game controllers and their per-frame W3C Standard Gamepad readings, held to the "gamepad" capability (core.AccessGamepad).
func init() {
	moduleTable["nsgo/gamepad"] = []export{
		fn("supported", (*runtime).gamepadSupported),
		fn("pads", (*runtime).gamepadPads),
		fn("state", (*runtime).gamepadState),
		{"Button", (*runtime).gamepadButtons},
		{"Axis", (*runtime).gamepadAxes},
	}
}

func (r *runtime) checkGamepadAccess() error {
	return r.cx.CheckAccess(core.AccessRequest{Permission: core.AccessGamepad})
}

func (r *runtime) gamepadSupported(_ js.FunctionCall) js.Value {
	if r.checkGamepadAccess() != nil {
		return r.vm.ToValue(false)
	}
	return r.vm.ToValue(r.cx.Env().SupportsGamepads())
}

func (r *runtime) gamepadPads(_ js.FunctionCall) js.Value {
	if err := r.checkGamepadAccess(); err != nil {
		panic(r.goError(err))
	}
	list := gamepad.Pads(r.cx)
	out := make([]any, len(list))
	for i, p := range list {
		out[i] = r.padValue(p)
	}
	return r.vm.NewArray(out...)
}

func (r *runtime) gamepadState(call js.FunctionCall) js.Value {
	if err := r.checkGamepadAccess(); err != nil {
		panic(r.goError(err))
	}
	v := arg(call, 0)
	if !defined(v) {
		return js.Null()
	}
	p, ok := gamepad.State(r.cx, int(v.ToInteger()))
	if !ok {
		return js.Null()
	}
	return r.padValue(p)
}

func (r *runtime) padValue(p gamepad.Pad) *js.Object {
	o := r.vm.NewObject()
	_ = o.Set("id", p.ID)
	_ = o.Set("name", p.Name)
	btns := make([]any, len(p.Buttons))
	for i, b := range p.Buttons {
		btns[i] = js32(b)
	}
	axes := make([]any, len(p.Axes))
	for i, a := range p.Axes {
		axes[i] = js32(a)
	}
	_ = o.Set("buttons", r.vm.NewArray(btns...))
	_ = o.Set("axes", r.vm.NewArray(axes...))
	return o
}

func (r *runtime) gamepadButtons() js.Value {
	o := r.vm.NewObject()
	for _, b := range []struct {
		name string
		val  int
	}{
		{"South", int(gamepad.ButtonSouth)},
		{"A", int(gamepad.ButtonA)},
		{"East", int(gamepad.ButtonEast)},
		{"B", int(gamepad.ButtonB)},
		{"West", int(gamepad.ButtonWest)},
		{"X", int(gamepad.ButtonX)},
		{"North", int(gamepad.ButtonNorth)},
		{"Y", int(gamepad.ButtonY)},
		{"LB", int(gamepad.ButtonLB)},
		{"RB", int(gamepad.ButtonRB)},
		{"LT", int(gamepad.ButtonLT)},
		{"RT", int(gamepad.ButtonRT)},
		{"Back", int(gamepad.ButtonBack)},
		{"Select", int(gamepad.ButtonSelect)},
		{"Start", int(gamepad.ButtonStart)},
		{"LeftStick", int(gamepad.ButtonLeftStick)},
		{"RightStick", int(gamepad.ButtonRightStick)},
		{"DPadUp", int(gamepad.ButtonDPadUp)},
		{"DPadDown", int(gamepad.ButtonDPadDown)},
		{"DPadLeft", int(gamepad.ButtonDPadLeft)},
		{"DPadRight", int(gamepad.ButtonDPadRight)},
		{"Home", int(gamepad.ButtonHome)},
		{"Count", gamepad.ButtonCount},
	} {
		_ = o.Set(b.name, b.val)
	}
	return o
}

func (r *runtime) gamepadAxes() js.Value {
	o := r.vm.NewObject()
	for _, a := range []struct {
		name string
		val  int
	}{
		{"LeftX", int(gamepad.AxisLeftX)},
		{"LeftY", int(gamepad.AxisLeftY)},
		{"RightX", int(gamepad.AxisRightX)},
		{"RightY", int(gamepad.AxisRightY)},
		{"Count", gamepad.AxisCount},
	} {
		_ = o.Set(a.name, a.val)
	}
	return o
}
