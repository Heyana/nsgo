//go:build js && wasm

package web

import (
	"syscall/js"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Run in a browser, with a stand-in for the Notification API: nsgo test -target web.
func TestNotificationsLetGoWhenTheyEnd(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-notifications")
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")
	s := open(func(cx *core.Scope) core.Node { return core.View{} }, Options{Mount: "#nsgo-notifications"}, nil)
	defer s.engine.Close()

	global := js.Global()
	was := global.Get("Object").Call("getOwnPropertyDescriptor", global, "Notification")
	defer func() {
		if was.IsUndefined() {
			global.Delete("Notification")
		} else {
			global.Get("Object").Call("defineProperty", global, "Notification", was)
		}
	}()
	fake := global.Get("Function").New(`
		const N = function (title, options) { this.tag = options.tag; this.closed = false; N.made.push(this) };
		N.prototype.close = function () { this.closed = true };
		N.permission = "granted";
		N.made = [];
		return N;`).Invoke()
	global.Get("Object").Call("defineProperty", global, "Notification",
		map[string]any{"value": fake, "configurable": true, "writable": true})
	made := func(i int) js.Value { return fake.Get("made").Index(i) }

	show := func(id string) {
		s.notify(&driver.MountItem{Kind: driver.ItemShowNotification, Notification: &driver.Notification{ID: id, Title: id}})
	}
	ended := func(what string, note js.Value) {
		t.Helper()
		if !note.Get("closed").Bool() || !note.Get("onclick").IsNull() || !note.Get("onclose").IsNull() {
			t.Errorf("%s: closed %v, onclick %v, onclose %v; want closed without handlers", what,
				note.Get("closed"), note.Get("onclick"), note.Get("onclose"))
		}
	}
	event := func(kind string) js.Value { return js.ValueOf(map[string]any{"type": kind}) }

	show("a")
	show("a")
	ended("replaced", made(0))
	if n := len(shownNotifications); n != 1 {
		t.Fatalf("%d shown after a replace, want 1", n)
	}
	made(1).Call("onclose", event("close")) // the user dismissed it
	ended("dismissed", made(1))
	show("b")
	s.notify(&driver.MountItem{Kind: driver.ItemCloseNotification, Value: "b"})
	ended("closed", made(2))
	show("c")
	show("d")
	s.notify(&driver.MountItem{Kind: driver.ItemCloseNotification})
	ended("closed with the rest", made(3))
	ended("closed with the rest", made(4))
	show("e")
	made(5).Call("onclick", event("click"))
	ended("clicked", made(5))
	if n := len(shownNotifications); n != 0 {
		t.Errorf("%d shown at the end, want 0", n)
	}
}
