//go:build js && wasm && nsgotest

package web

import (
	"syscall/js"
	"testing"
)

func TestScreenAwakeRetriesAfterUserActivation(t *testing.T) {
	g := js.Global()
	navigator := g.Get("navigator")
	api := navigator.Get("wakeLock")
	if !api.Truthy() || g.Get("nsgoTest").Type() != js.TypeFunction {
		t.Skip("requires the browser test host and Screen Wake Lock API")
	}
	// Reproduce a browser requiring activation before the first grant.
	original := api.Get("request")
	requests := 0
	request := js.FuncOf(func(_ js.Value, args []js.Value) any {
		requests++
		if requests == 1 || !navigator.Get("userActivation").Get("isActive").Bool() {
			return g.Get("Promise").Call("reject", g.Get("DOMException").New("Permission was denied", "NotAllowedError"))
		}
		return original.Call("call", api, args[0])
	})
	g.Get("Object").Call("defineProperty", api, "request", map[string]any{"value": request, "configurable": true})
	defer func() {
		g.Get("Reflect").Call("deleteProperty", api, "request")
		request.Release()
	}()
	s := &surface{doc: g.Get("document")}
	defer s.clearDisplayHolds()
	s.keepDisplayAwake(1)
	waitForScreenWakeLock(t, "the first refusal", func() bool { return requests == 1 && !screenAwake.pending })
	if err := new(testSurface).do(map[string]any{"op": "press", "x": 10, "y": 10, "hold": 0}); err != nil {
		t.Fatal(err)
	}
	waitForScreenWakeLock(t, "a native grant after trusted input", func() bool {
		return screenAwake.lock.Truthy() && !screenAwake.lock.Get("released").Bool()
	})
	if requests != 2 {
		t.Fatalf("requests = %d, want one retry", requests)
	}
}
