//go:build js && wasm && nsgotest

package web

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestBrowserPointerLock(t *testing.T) {
	doc := js.Global().Get("document")
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-pointer-lock-test")
	setStyle(mount, map[string]string{"width": "200px", "height": "200px"})
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")
	var scope *core.Scope
	var ref core.Ref
	var unlock func()
	var x, y float64
	s := open(func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{
			Ref: &ref, TestID: "lock", Style: core.Style{Width: core.Pt(100), Height: core.Pt(100)},
			OnPress:        func(core.PressEvent) { unlock = ref.LockPointer() },
			OnPointerDelta: func(dx, dy float64) { x, y = x+dx, y+dy },
		}
	}, Options{Mount: "#nsgo-pointer-lock-test"}, nil)
	defer s.engine.Close()
	wait := func(what string, ready func() bool) {
		t.Helper()
		for until := time.Now().Add(4 * time.Second); !ready(); time.Sleep(10 * time.Millisecond) {
			if time.Now().After(until) {
				t.Fatalf("timed out waiting for %s", what)
			}
		}
	}
	wait("the view to mount", func() bool { return mount.Call("querySelector", "[data-testid=lock]").Truthy() })
	if !s.env.CanLockPointer {
		t.Skip("the browser has no pointer lock API")
	}
	host := &testSurface{s: s}
	press := func() {
		t.Helper()
		if err := host.Press(driver.Point{X: 25, Y: 25}, 0, driver.Modifiers{}); err != nil {
			t.Fatal(err)
		}
		wait("the native pointer lock", func() bool {
			return doc.Get("pointerLockElement").Equal(mount.Call("querySelector", "[data-testid=lock]"))
		})
	}
	press()
	// Exercise the browser's movement fields followed by the test host's
	// relative input step. Neither is expressed as an absolute point.
	doc.Call("dispatchEvent", js.Global().Get("MouseEvent").New("mousemove", map[string]any{"movementX": 65, "movementY": -70}))
	host.PointerDelta(2.5, 4)
	wait("relative motion", func() bool { return x == 67.5 && y == -66 })
	if err := host.Key("Escape"); err != nil {
		t.Fatal(err)
	}
	wait("Escape to release the browser", func() bool { return !doc.Get("pointerLockElement").Truthy() })
	host.PointerDelta(7, 9)
	s.engine.Sync()
	if x != 67.5 || y != -66 {
		t.Fatalf("relative motion continued after Escape: %g, %g", x, y)
	}
	press()
	scope.Post(unlock)
	wait("the app to unlock the pointer", func() bool { return !doc.Get("pointerLockElement").Truthy() })
}
