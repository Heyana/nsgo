//go:build js && wasm && !nsgominiprogram && !nsgoharmony

package gamepad

import (
	"slices"
	"sync"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var webDefault = &webSource{}

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
		st := driver.GamepadState{
			ID:   gp.Get("index").Int(),
			Name: gp.Get("id").String(),
		}
		if st.Name == "" {
			st.Name = "Gamepad"
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
