//go:build js && wasm && !nsgominiprogram && !nsgoharmony

package gamepad

import (
	"slices"
	"sync"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var (
	webDefault     = &webSource{}
	webCatchOnce   sync.Once
	webNoopCatchFn js.Func
)

func webNoopCatch() js.Func {
	webCatchOnce.Do(func() {
		webNoopCatchFn = js.FuncOf(func(js.Value, []js.Value) any { return nil })
	})
	return webNoopCatchFn
}

func platformSource() driver.GamepadSource {
	return webDefault
}

type webSource struct{}

func (w *webSource) Poll() []driver.GamepadState {
	nav := js.Global().Get("navigator")
	if !nav.Truthy() || nav.Get("getGamepads").Type() != js.TypeFunction {
		return nil
	}
	list := nav.Call("getGamepads")
	if !list.Truthy() {
		return nil
	}
	n := list.Length()
	if n == 0 {
		return nil
	}
	out := make([]driver.GamepadState, 0, n)
	for i := 0; i < n; i++ {
		gp := list.Index(i)
		if !gp.Truthy() || !gp.Get("connected").Bool() {
			continue
		}
		rawID := gp.Get("id").String()
		vid, pid := parseWebGamepadID(rawID)
		name := rawID
		if name == "" {
			name = "Gamepad"
		}
		act := gp.Get("vibrationActuator")
		canRumble := act.Truthy() && act.Get("playEffect").Type() == js.TypeFunction
		st := driver.GamepadState{
			ID:        gp.Get("index").Int(),
			Name:      name,
			VendorID:  vid,
			ProductID: pid,
			Family:    driver.GamepadFamily(DetectFamily(vid, pid, rawID)),
			CanRumble: canRumble,
		}
		if buttons := gp.Get("buttons"); buttons.Truthy() {
			bn := min(buttons.Length(), ButtonCount)
			for b := 0; b < bn; b++ {
				btn := buttons.Index(b)
				if !btn.Truthy() {
					continue
				}
				v := float32(btn.Get("value").Float())
				if v == 0 && btn.Get("pressed").Bool() {
					v = 1
				}
				st.Buttons[b] = clamp01(v)
			}
		}
		if axes := gp.Get("axes"); axes.Truthy() {
			an := min(axes.Length(), AxisCount)
			for a := 0; a < an; a++ {
				st.Axes[a] = clampAxis(float32(axes.Index(a).Float()))
			}
		}
		out = append(out, st)
	}
	slices.SortFunc(out, func(a, b driver.GamepadState) int { return a.ID - b.ID })
	return out
}

func (w *webSource) Watch(onChange func()) func() {
	if onChange == nil {
		return func() {}
	}
	g := js.Global()
	if !g.Truthy() || g.Get("addEventListener").Type() != js.TypeFunction {
		return func() {}
	}
	cb := js.FuncOf(func(js.Value, []js.Value) any {
		onChange()
		return nil
	})
	g.Call("addEventListener", "gamepadconnected", cb)
	g.Call("addEventListener", "gamepaddisconnected", cb)
	var once sync.Once
	return func() {
		once.Do(func() {
			g.Call("removeEventListener", "gamepadconnected", cb)
			g.Call("removeEventListener", "gamepaddisconnected", cb)
			cb.Release()
		})
	}
}

func (w *webSource) Rumble(id int, lowFreq, highFreq float32, duration time.Duration) {
	act := findWebVibrationActuator(id)
	if !act.Truthy() || act.Get("playEffect").Type() != js.TypeFunction {
		return
	}
	ms := duration.Milliseconds()
	if ms <= 0 && duration > 0 {
		ms = 1
	}
	opts := map[string]any{
		"startDelay":      0,
		"duration":        ms,
		"weakMagnitude":   float64(highFreq),
		"strongMagnitude": float64(lowFreq),
	}
	p := act.Call("playEffect", "dual-rumble", opts)
	if p.Truthy() && p.Get("catch").Type() == js.TypeFunction {
		p.Call("catch", webNoopCatch())
	}
}

func (w *webSource) StopRumble(id int) {
	act := findWebVibrationActuator(id)
	if !act.Truthy() {
		return
	}
	var p js.Value
	if act.Get("reset").Type() == js.TypeFunction {
		p = act.Call("reset")
	} else if act.Get("playEffect").Type() == js.TypeFunction {
		p = act.Call("playEffect", "dual-rumble", map[string]any{
			"startDelay":      0,
			"duration":        0,
			"weakMagnitude":   0,
			"strongMagnitude": 0,
		})
	}
	if p.Truthy() && p.Get("catch").Type() == js.TypeFunction {
		p.Call("catch", webNoopCatch())
	}
}

func findWebVibrationActuator(id int) js.Value {
	nav := js.Global().Get("navigator")
	if !nav.Truthy() || nav.Get("getGamepads").Type() != js.TypeFunction {
		return js.Undefined()
	}
	list := nav.Call("getGamepads")
	if !list.Truthy() {
		return js.Undefined()
	}
	n := list.Length()
	for i := 0; i < n; i++ {
		gp := list.Index(i)
		if !gp.Truthy() || !gp.Get("connected").Bool() {
			continue
		}
		if gp.Get("index").Int() == id {
			return gp.Get("vibrationActuator")
		}
	}
	return js.Undefined()
}
