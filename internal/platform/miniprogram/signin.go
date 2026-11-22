//go:build js && wasm

package miniprogram

import (
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func (s *surface) signIn(it *driver.MountItem) {
	requestMiniProgramSignIn(s.bridge, it.Count, it.On, s.engine.Dispatch)
}

// requestMiniProgramSignIn uses the runtime's normalized host reply, keeping
// a login code out of ordinary URL/open-link events and error text.
func requestMiniProgramSignIn(bridge js.Value, id uint32, prompt bool, dispatch func(driver.Event)) {
	if bridge.Get("signIn").Type() != js.TypeFunction {
		dispatch(driver.Event{Kind: driver.EventMiniProgramSignIn, Count: id, Text: "unavailable"})
		return
	}
	var complete js.Func
	complete = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer complete.Release()
		ev := driver.Event{Kind: driver.EventMiniProgramSignIn, Count: id, Text: "failed"}
		if r := first(args); r.Type() == js.TypeObject && !r.IsNull() {
			if e := r.Get("error"); e.Type() == js.TypeString && e.String() != "" {
				ev.Text = e.String()
			} else if code, platform := r.Get("code"), r.Get("platform"); code.Type() == js.TypeString && code.String() != "" && platform.Type() == js.TypeString {
				ev.Text = ""
				ev.Data = &driver.MiniProgramCredential{Platform: platform.String(), Code: code.String()}
			}
		}
		dispatch(ev)
		return nil
	})
	bridge.Call("signIn", map[string]any{"prompt": prompt, "complete": complete})
}
