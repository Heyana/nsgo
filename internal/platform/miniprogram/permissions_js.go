//go:build js && wasm

package miniprogram

import (
	"sync"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type jsPermissionHost struct {
	api js.Value
}

func (s *surface) permission(id uint32, r driver.PermissionRequest) {
	handlePermission(id, r, jsPermissionHost{api: s.api}, s.engine.Dispatch)
}

func (h jsPermissionHost) hasMethod(name string) bool {
	return h.api.Type() == js.TypeObject && !h.api.IsNull() && h.api.Get(name).Type() == js.TypeFunction
}

func (h jsPermissionHost) hasGetSetting() bool  { return h.hasMethod("getSetting") }
func (h jsPermissionHost) hasAuthorize() bool   { return h.hasMethod("authorize") }
func (h jsPermissionHost) hasOpenSetting() bool { return h.hasMethod("openSetting") }

func (h jsPermissionHost) invoke(name string, opts map[string]any, done func(res js.Value, ok bool)) {
	var (
		once          sync.Once
		success, fail js.Func
	)
	finish := func(res js.Value, ok bool) {
		once.Do(func() {
			if !success.Value.IsUndefined() {
				success.Release()
			}
			if !fail.Value.IsUndefined() {
				fail.Release()
			}
			if done != nil {
				done(res, ok)
			}
		})
	}
	if !h.hasMethod(name) {
		finish(js.Undefined(), false)
		return
	}
	success = js.FuncOf(func(_ js.Value, args []js.Value) any {
		finish(first(args), true)
		return nil
	})
	fail = js.FuncOf(func(_ js.Value, args []js.Value) any {
		finish(first(args), false)
		return nil
	})
	opts["success"] = success
	opts["fail"] = fail
	defer func() {
		if recover() != nil {
			finish(js.Undefined(), false)
		}
	}()
	h.api.Call(name, opts)
}

func (h jsPermissionHost) getSetting(scope string, done func(status int, ok bool)) {
	h.invoke("getSetting", map[string]any{}, func(res js.Value, ok bool) {
		if !ok || res.Type() != js.TypeObject || res.IsNull() {
			done(driver.PermissionUnknown, false)
			return
		}
		auth := res.Get("authSetting")
		if auth.Type() != js.TypeObject || auth.IsNull() {
			done(driver.PermissionUnknown, false)
			return
		}
		v := auth.Get(scope)
		if v.IsUndefined() {
			done(scopeStatus(false, false, false), true)
			return
		}
		if v.Type() == js.TypeBoolean {
			done(scopeStatus(true, true, v.Bool()), true)
			return
		}
		done(scopeStatus(true, false, false), true)
	})
}

func (h jsPermissionHost) authorize(scope string, done func(ok bool, errMsg string)) {
	h.invoke("authorize", map[string]any{"scope": scope}, func(res js.Value, ok bool) {
		if ok {
			done(true, "")
			return
		}
		done(false, jsErrorMessage(res))
	})
}

func (h jsPermissionHost) openSetting(done func(ok bool)) {
	h.invoke("openSetting", map[string]any{}, func(_ js.Value, ok bool) {
		done(ok)
	})
}

func jsErrorMessage(res js.Value) string {
	if res.Type() == js.TypeString {
		return res.String()
	}
	if res.Type() != js.TypeObject || res.IsNull() {
		return ""
	}
	for _, key := range []string{"errMsg", "errorMessage", "message"} {
		if v := res.Get(key); v.Type() == js.TypeString && v.String() != "" {
			return v.String()
		}
	}
	return ""
}
