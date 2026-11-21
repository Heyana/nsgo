//go:build js && wasm

package web

import (
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func watchNetwork(changed func(driver.Network)) (driver.Network, func()) {
	window := js.Global()
	navigator := window.Get("navigator")
	if !navigator.Truthy() || navigator.Get("onLine").Type() != js.TypeBoolean {
		return driver.Network{Online: true}, nil
	}
	read := func() driver.Network { return driver.Network{Online: navigator.Get("onLine").Bool()} }
	callback := js.FuncOf(func(js.Value, []js.Value) any {
		changed(read())
		return nil
	})
	window.Call("addEventListener", "online", callback)
	window.Call("addEventListener", "offline", callback)
	return read(), func() {
		window.Call("removeEventListener", "online", callback)
		window.Call("removeEventListener", "offline", callback)
		callback.Release()
	}
}
