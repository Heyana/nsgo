//go:build js && wasm

package main

import (
	"net/url"
	"strings"
	"syscall/js"

	"github.com/oneo-me/nsgo/core"
)

// routeName returns the section that location.hash names, as "#/Name".
func routeName() string {
	hash := js.Global().Get("location").Get("hash").String()
	name, ok := strings.CutPrefix(hash, "#/")
	if !ok {
		return ""
	}
	name, err := url.PathUnescape(name)
	if err != nil {
		return ""
	}
	return name
}

// setRoute writes section to the address bar. replace rewrites the current
// entry instead of adding one, for the address the app starts on.
func setRoute(name string, replace bool) {
	hash := "#/" + url.PathEscape(name)
	if replace {
		js.Global().Get("history").Call("replaceState", nil, "", hash)
		return
	}
	js.Global().Get("location").Set("hash", hash)
}

// routeListener is kept for the page's lifetime: a released js.Func would
// stop hearing the browser's history.
var routeListener js.Func

// listenRoute has open run for a section the address names, when the
// browser's back button or a link changes it.
func listenRoute(cx *core.Scope, open func(string)) {
	routeListener = js.FuncOf(func(js.Value, []js.Value) any {
		if name := routeName(); name != "" {
			cx.Post(func() { open(name) })
		}
		return nil
	})
	js.Global().Call("addEventListener", "hashchange", routeListener)
}
