//go:build js && wasm

package miniprogram

import (
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMiniUpdateBridgeRetainedStateApplyAndCleanup(t *testing.T) {
	value := js.Global().Get("Object").New()
	for name, setting := range map[string]any{"supported": true, "checked": false, "available": false, "ready": false, "error": "", "checkedAt": float64(1234000)} {
		value.Set(name, setting)
	}
	listener := js.Undefined()
	unsubscribed := 0
	unsub := js.FuncOf(func(js.Value, []js.Value) any { unsubscribed++; listener = js.Undefined(); return nil })
	defer unsub.Release()
	changed := js.FuncOf(func(_ js.Value, args []js.Value) any { listener = args[0]; return unsub })
	defer changed.Release()
	value.Set("changed", changed)
	applied := 0
	apply := js.FuncOf(func(js.Value, []js.Value) any { applied++; value.Set("ready", false); return true })
	defer apply.Release()
	value.Set("apply", apply)
	bridge := js.Global().Get("Object").New()
	bridge.Set("updates", value)
	s := &surface{bridge: bridge}
	state := s.initUpdates(Options{Name: "Fixture", Version: "1.0"})
	defer s.closeUpdates()
	if state.Managed != driver.UpdateManagedSelf || state.LastCheck.Unix() != 1234 {
		t.Fatalf("initial state=%+v", state)
	}
	var events []driver.Event
	s.updates.dispatch = func(ev driver.Event) { events = append(events, ev) }
	s.checkForUpdates(&driver.MountItem{Count: 7})
	if len(events) != 0 {
		t.Fatal("answered before native readiness")
	}
	value.Set("ready", true)
	value.Set("available", true)
	listener.Invoke()
	if len(events) != 2 {
		t.Fatalf("events=%+v", events)
	}
	found := events[1].Data.(*driver.Update)
	if !found.Restart || found.Version != "" || found.App != "Fixture" || found.Current != "1.0" {
		t.Fatalf("native answer=%+v", found)
	}
	s.service(&driver.MountItem{Kind: driver.ItemQuit})
	if applied != 0 {
		t.Fatal("ordinary quit applied an update")
	}
	s.service(&driver.MountItem{Kind: driver.ItemQuit, On: true})
	s.service(&driver.MountItem{Kind: driver.ItemQuit, On: true})
	if applied != 1 {
		t.Fatalf("apply calls=%d", applied)
	}
	s.checkForUpdates(&driver.MountItem{Count: 8})
	bridge.Call("closeUpdates")
	if unsubscribed != 1 || listener.Truthy() || len(s.updates.pending) != 0 {
		t.Fatalf("cleanup: unsub=%d listener=%v pending=%d", unsubscribed, listener, len(s.updates.pending))
	}
}
