//go:build js && wasm

package miniprogram

import (
	"errors"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// WeChat mini programs (base library >= 2.21.3) provide platform-managed
// AES-128 encrypted local storage through asynchronous wx.setStorage /
// wx.getStorage with encrypt: true, where WeChat's native client manages the
// key outside the mini program's JavaScript sandbox. Other mini program hosts
// (older WeChat, QQ, Alipay, Douyin) lack this capability and receive nil so
// core.Secrets answers core.ErrSecretsUnavailable rather than storing secrets
// in plain storage.

// secretsStore returns a driver.SecretStore when the host supports
// platform-encrypted storage, or nil otherwise.
func (s *surface) secretsStore() driver.SecretStore {
	if !supportsEncryptedStorage(s.api) {
		return nil
	}
	return &encryptedStorage{s: s}
}

func supportsEncryptedStorage(api js.Value) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	if !api.Truthy() || api.Get("canIUse").Type() != js.TypeFunction {
		return false
	}
	for _, fn := range []string{"setStorage", "getStorage", "removeStorage"} {
		if api.Get(fn).Type() != js.TypeFunction {
			return false
		}
	}
	return canIUse(api, "setStorage.object.encrypt") && canIUse(api, "getStorage.object.encrypt")
}

func canIUse(api js.Value, schema string) bool {
	v := api.Call("canIUse", schema)
	return v.Type() == js.TypeBoolean && v.Bool()
}

type encryptedStorage struct{ s *surface }

type secretCallResult struct {
	res js.Value
	ok  bool
}

func (e *encryptedStorage) callSync(name string, opts map[string]any) (js.Value, bool) {
	ch := make(chan secretCallResult, 1)
	e.s.call(name, opts, func(res js.Value, ok bool) {
		ch <- secretCallResult{res: res, ok: ok}
	})
	r := <-ch
	return r.res, r.ok
}

func storageErrMsg(res js.Value) string {
	if res.Truthy() {
		if msg := res.Get("errMsg"); msg.Type() == js.TypeString && msg.String() != "" {
			return msg.String()
		}
	}
	return "storage request failed"
}

func (e *encryptedStorage) ReadSecret(item driver.SecretItem) (string, bool, error) {
	res, ok := e.callSync("getStorage", map[string]any{
		"key":     secretStorageKey(item.Service, item.Account),
		"encrypt": true,
	})
	if !ok {
		msg := storageErrMsg(res)
		if isStorageNotFound(msg) {
			return "", false, nil
		}
		return "", false, errors.New("miniprogram secrets: " + msg)
	}
	if !res.Truthy() {
		return "", false, nil
	}
	data := res.Get("data")
	switch data.Type() {
	case js.TypeUndefined, js.TypeNull:
		return "", false, nil
	case js.TypeString:
		return data.String(), true, nil
	default:
		return "", false, errors.New("miniprogram secrets: stored value is not a string")
	}
}

func (e *encryptedStorage) WriteSecret(item driver.SecretItem, value string) error {
	res, ok := e.callSync("setStorage", map[string]any{
		"key":     secretStorageKey(item.Service, item.Account),
		"data":    value,
		"encrypt": true,
	})
	if !ok {
		return errors.New("miniprogram secrets: " + storageErrMsg(res))
	}
	return nil
}

func (e *encryptedStorage) DeleteSecret(item driver.SecretItem) error {
	res, ok := e.callSync("removeStorage", map[string]any{
		"key": secretStorageKey(item.Service, item.Account),
	})
	if !ok {
		msg := storageErrMsg(res)
		if isStorageNotFound(msg) {
			return nil
		}
		return errors.New("miniprogram secrets: " + msg)
	}
	return nil
}
