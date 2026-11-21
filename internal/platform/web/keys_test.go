//go:build js && wasm

package web

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// TestKeysComeUpAsThePageLosesThem holds keys on a focused view that hears them, for an app that reads the keys held. On a Mac, Command coming up reports the keys held with it but not Shift, whose own up the browser still sends; a key that goes down again without repeating reports its lost up first; and every key held comes up as the page loses the focus, leaving core.KeysDown empty (#1046).
func TestKeysComeUpAsThePageLosesThem(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	mac := apple
	apple = true
	defer func() { apple = mac }()
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-keys-up")
	setStyle(mount, map[string]string{"width": "200px", "height": "200px"})
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")
	var pad core.Ref
	var scope *core.Scope
	var mu sync.Mutex
	var log []string
	var held []string
	var codes []string
	var events []core.KeyEvent
	mark := func(prefix string, e core.KeyEvent) {
		s := prefix + e.Key
		if e.Modifiers.Shift {
			s += "+S"
		}
		if e.Repeat {
			s += "!"
		}
		mu.Lock()
		log = append(log, s)
		events = append(events, e)
		mu.Unlock()
	}
	s := open(func(cx *core.Scope) core.Node {
		scope = cx
		cx.Effect(func() {
			keys, positions := core.KeysDown(cx), core.KeyCodesDown(cx)
			mu.Lock()
			held, codes = keys, positions
			mu.Unlock()
		})
		return core.View{Children: []core.Node{
			core.View{TestID: "pad", Ref: &pad, Focusable: func() bool { return true }, Style: core.Style{Width: core.Pt(100), Height: core.Pt(40)},
				OnKeyDown: func(e core.KeyEvent) bool { mark("d:", e); return false },
				OnKeyUp:   func(e core.KeyEvent) { mark("u:", e) }},
		}}
	}, Options{Mount: "#nsgo-keys-up"}, nil)
	find := func(id string) js.Value { return mount.Call("querySelector", "[data-testid="+id+"]") }
	wait := func(what string, ready func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !ready() {
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for %s", what)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	// heard waits for the view to have heard want and the app to read keys held.
	heard := func(want string, keys ...string) {
		t.Helper()
		var got string
		var down []string
		wait(fmt.Sprintf("%q with %q held", want, keys), func() bool {
			mu.Lock()
			defer mu.Unlock()
			got, down = strings.Join(log, ","), held
			return got == want && slices.Equal(down, keys)
		})
	}
	wait("the mounted pad", func() bool { return find("pad").Truthy() })
	scope.Post(pad.Focus)
	wait("the pad's focus", func() bool { return find("pad").Equal(doc.Get("activeElement")) })
	key := func(kind, key, code string, shift, meta, repeat bool) {
		find("pad").Call("dispatchEvent", js.Global().Get("KeyboardEvent").New(kind, map[string]any{
			"key": key, "code": code, "shiftKey": shift, "metaKey": meta, "repeat": repeat, "bubbles": true, "cancelable": true,
		}))
	}
	key("keydown", "Shift", "ShiftLeft", true, false, false)
	key("keydown", "Meta", "MetaLeft", true, true, false)
	key("keydown", "a", "KeyA", true, true, false)
	heard("d:shift+S,d:meta+S,d:a+S", "a", "meta", "shift")
	// The browser sends no up for A, released while Command is held, but still sends Shift's.
	key("keyup", "Meta", "MetaLeft", true, false, false)
	heard("d:shift+S,d:meta+S,d:a+S,u:meta+S,u:a+S", "shift")
	key("keyup", "Shift", "ShiftLeft", false, false, false)
	heard("d:shift+S,d:meta+S,d:a+S,u:meta+S,u:a+S,u:shift")
	mu.Lock()
	log = nil
	mu.Unlock()
	key("keydown", "b", "KeyB", false, false, false)
	key("keydown", "b", "KeyB", false, false, true)
	heard("d:b,d:b!", "b")
	// Its up never came: the key going down again without repeating reports it first.
	key("keydown", "b", "KeyB", false, false, false)
	heard("d:b,d:b!,u:b,d:b", "b")
	js.Global().Call("dispatchEvent", js.Global().Get("Event").New("blur"))
	heard("d:b,d:b!,u:b,d:b,u:b")
	if n := len(s.keysHeld); n != 0 {
		t.Fatalf("%d keys still recorded after the page lost the focus", n)
	}
	// The up the browser sends later is not heard again.
	key("keyup", "b", "KeyB", false, false, false)
	key("keydown", "c", "KeyC", false, false, false)
	heard("d:b,d:b!,u:b,d:b,u:b,d:c", "c")
	key("keyup", "c", "KeyC", false, false, false)
	heard("d:b,d:b!,u:b,d:b,u:b,d:c,u:c")
	for _, name := range []string{"z", "w"} {
		mu.Lock()
		log, events = nil, nil
		mu.Unlock()
		key("keydown", name, "KeyW", false, false, false)
		wait("the physical W position held", func() bool {
			mu.Lock()
			defer mu.Unlock()
			return slices.Equal(codes, []string{"KeyW"})
		})
		key("keyup", name, "KeyW", false, false, false)
		heard("d:" + name + ",u:" + name)
		mu.Lock()
		want := core.KeyEvent{Key: name, Code: "KeyW"}
		if !slices.Equal(events, []core.KeyEvent{want, want}) || len(codes) != 0 {
			t.Errorf("physical W with %q: events %+v, held %v", name, events, codes)
		}
		mu.Unlock()
	}
	// This test owns the surface's engine; browser listeners last until the per-test page closes, as in the backend's other browser tests.
	s.engine.Close()
}
