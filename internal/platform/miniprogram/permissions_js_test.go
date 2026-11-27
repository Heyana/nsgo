//go:build js && wasm

package miniprogram

import (
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestMiniProgramPermissionWasmBridge(t *testing.T) {
	auth := js.Global().Get("Object").New()
	auth.Set("scope.record", true)
	auth.Set("scope.camera", false)

	api := js.Global().Get("Object").New()
	getSetting := js.FuncOf(func(_ js.Value, args []js.Value) any {
		opts := args[0]
		res := js.Global().Get("Object").New()
		res.Set("authSetting", auth)
		opts.Get("success").Invoke(res)
		// Duplicate callback must be ignored.
		opts.Get("fail").Invoke(map[string]any{"errMsg": "duplicate"})
		return nil
	})
	defer getSetting.Release()
	authorize := js.FuncOf(func(_ js.Value, args []js.Value) any {
		opts := args[0]
		scope := opts.Get("scope").String()
		if scope == "scope.bluetooth" {
			auth.Set(scope, true)
			opts.Get("success").Invoke(map[string]any{})
			opts.Get("fail").Invoke(map[string]any{"errMsg": "duplicate"})
			return nil
		}
		opts.Get("fail").Invoke(map[string]any{"errMsg": "authorize:fail auth deny"})
		return nil
	})
	defer authorize.Release()
	openOK := true
	openSetting := js.FuncOf(func(_ js.Value, args []js.Value) any {
		opts := args[0]
		if openOK {
			opts.Get("success").Invoke(map[string]any{"authSetting": auth})
		} else {
			opts.Get("fail").Invoke(map[string]any{"errMsg": "openSetting:fail"})
		}
		return nil
	})
	defer openSetting.Release()

	api.Set("getSetting", getSetting)
	api.Set("authorize", authorize)
	api.Set("openSetting", openSetting)

	run := func(req driver.PermissionRequest) driver.Event {
		var events []driver.Event
		handlePermission(7, req, jsPermissionHost{api: api}, func(ev driver.Event) { events = append(events, ev) })
		if len(events) != 1 {
			t.Fatalf("%+v dispatched %d events, want 1", req, len(events))
		}
		return events[0]
	}

	if ev := run(driver.PermissionRequest{Command: driver.PermissionCheck, Name: driver.PermissionMicrophone}); ev.Index != driver.PermissionGranted {
		t.Errorf("microphone check = %+v, want granted", ev)
	}
	if ev := run(driver.PermissionRequest{Command: driver.PermissionCheck, Name: driver.PermissionCamera}); ev.Index != driver.PermissionDenied {
		t.Errorf("camera check = %+v, want denied", ev)
	}
	if ev := run(driver.PermissionRequest{Command: driver.PermissionCheck, Name: driver.PermissionBluetooth}); ev.Index != driver.PermissionNotAsked {
		t.Errorf("bluetooth check = %+v, want not-asked", ev)
	}
	if ev := run(driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionBluetooth}); ev.Index != driver.PermissionGranted {
		t.Errorf("bluetooth ask = %+v, want granted", ev)
	}
	if ev := run(driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: driver.PermissionCamera}); !ev.Finished {
		t.Errorf("open settings = %+v, want Finished=true", ev)
	}
	openOK = false
	if ev := run(driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: driver.PermissionCamera}); ev.Finished {
		t.Errorf("failed open settings = %+v, want Finished=false", ev)
	}
	if ev := run(driver.PermissionRequest{Command: driver.PermissionCheck, Name: driver.PermissionContacts}); ev.Index != driver.PermissionUnavailable {
		t.Errorf("contacts check = %+v, want unavailable", ev)
	}

	// Host without permission APIs answers unavailable / not opened cleanly.
	empty := js.Global().Get("Object").New()
	var ev driver.Event
	handlePermission(8, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionCamera}, jsPermissionHost{api: empty}, func(e driver.Event) { ev = e })
	if ev.Index != driver.PermissionUnavailable {
		t.Errorf("missing API ask = %+v, want unavailable", ev)
	}
	handlePermission(9, driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: driver.PermissionCamera}, jsPermissionHost{api: empty}, func(e driver.Event) { ev = e })
	if ev.Finished {
		t.Errorf("missing API openSettings = %+v, want Finished=false", ev)
	}

	// Synchronous throw in host API must not panic or hang.
	throwing := js.Global().Get("Object").New()
	boom := js.FuncOf(func(js.Value, []js.Value) any {
		js.Global().Get("Function").New("throw new Error('boom')").Invoke()
		return nil
	})
	defer boom.Release()
	throwing.Set("getSetting", boom)
	throwing.Set("openSetting", boom)
	handlePermission(10, driver.PermissionRequest{Command: driver.PermissionCheck, Name: driver.PermissionCamera}, jsPermissionHost{api: throwing}, func(e driver.Event) { ev = e })
	if ev.Index != driver.PermissionUnknown {
		t.Errorf("throwing getSetting = %+v, want unknown", ev)
	}
	handlePermission(11, driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: driver.PermissionCamera}, jsPermissionHost{api: throwing}, func(e driver.Event) { ev = e })
	if ev.Finished {
		t.Errorf("throwing openSetting = %+v, want Finished=false", ev)
	}
}
