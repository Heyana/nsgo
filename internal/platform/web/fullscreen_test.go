//go:build js && wasm && nsgotest

package web

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestExplicitFullScreenFollowsBrowser(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() || !doc.Get("fullscreenEnabled").Truthy() {
		t.Skip("requires browser full screen")
	}
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-fullscreen-test")
	mount.Get("style").Set("height", "240px")
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")
	s := open(func(cx *core.Scope) core.Node {
		button := func(id string, action func()) core.Node {
			return core.View{TestID: id, Style: core.Style{Width: core.Pt(160), Height: core.Pt(40)},
				OnPress: func(core.PressEvent) { action() }, Children: []core.Node{core.Text{Content: id}}}
		}
		return core.View{Children: []core.Node{
			button("enter", func() {
				cx.SetFullScreen(true, "desktop-id")
				cx.SetFullScreen(true)
			}),
			button("exit", func() { cx.SetFullScreen(false) }),
			core.Text{TestID: "state", ContentFunc: func() string {
				if cx.Env().FullScreen() {
					return "full screen"
				}
				return "windowed"
			}},
		}}
	}, Options{Mount: "#nsgo-fullscreen-test"}, nil)
	defer s.engine.Close()
	defer s.setBrowserFullscreen(false)
	find := func(id string) js.Value { return mount.Call("querySelector", "[data-testid="+id+"]") }
	wait := func(label string, ready func() bool) {
		t.Helper()
		for limit := time.Now().Add(4 * time.Second); !ready(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(limit) {
				t.Fatalf("timed out waiting for %s", label)
			}
		}
	}
	stateIs := func(label string) bool {
		return find("state").Truthy() && find("state").Get("textContent").String() == label
	}
	test := &testSurface{s: s}
	press := func(id string) {
		t.Helper()
		rect := find(id).Call("getBoundingClientRect")
		if err := test.do(map[string]any{"op": "press", "x": rect.Get("left").Float() + 12, "y": rect.Get("top").Float() + 12, "hold": 0}); err != nil {
			t.Fatal(err)
		}
	}
	wait("initial windowed state", func() bool { return stateIs("windowed") })
	press("enter")
	wait("press handler entering full screen", func() bool { return s.browserFullscreen() && stateIs("full screen") })
	press("enter")
	if !s.browserFullscreen() {
		t.Fatal("repeated entry left full screen")
	}
	press("exit")
	wait("explicit exit", func() bool { return !s.browserFullscreen() && stateIs("windowed") })
	press("enter")
	wait("second entry", func() bool { return s.browserFullscreen() && stateIs("full screen") })
	if err := test.Key("Escape"); err != nil {
		t.Fatal(err)
	}
	wait("Escape reaching Env", func() bool { return !s.browserFullscreen() && stateIs("windowed") })
	press("enter")
	wait("third entry", func() bool { return s.browserFullscreen() && stateIs("full screen") })
	// Simulate the browser ending the session outside NSGo's commands.
	doc.Call("exitFullscreen")
	wait("browser exit reaching Env", func() bool { return !s.browserFullscreen() && stateIs("windowed") })
}
