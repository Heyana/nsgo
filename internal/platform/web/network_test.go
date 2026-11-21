//go:build js && wasm && nsgotest

package web

import (
	"slices"
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestBrowserNetworkFollowsNavigator(t *testing.T) {
	window := js.Global()
	navigator := window.Get("navigator")
	set := func(online bool) {
		window.Get("Object").Call("defineProperty", navigator, "onLine", map[string]any{"value": online, "configurable": true})
	}
	defer window.Get("Reflect").Call("deleteProperty", navigator, "onLine")
	set(true)
	var states []driver.Network
	initial, close := watchNetwork(func(value driver.Network) { states = append(states, value) })
	if close == nil {
		t.Fatal("the browser did not provide navigator.onLine")
	}
	online := driver.Network{Online: true}
	if initial != online {
		close()
		t.Fatalf("initial browser state = %+v", initial)
	}
	set(false)
	window.Call("dispatchEvent", window.Get("Event").New("offline"))
	set(true)
	window.Call("dispatchEvent", window.Get("Event").New("online"))
	close()
	window.Call("dispatchEvent", window.Get("Event").New("offline"))
	if want := []driver.Network{{}, online}; !slices.Equal(states, want) {
		t.Fatalf("network changes = %+v, want %+v", states, want)
	}
}
