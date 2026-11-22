//go:build js && wasm

package harmonyos

import (
	"slices"
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

type keyboardBridgePlatform struct {
	core.ApproxTextEngine
	m         *mounter
	listeners []driver.NodeID
	input     driver.NodeID
}

func (p *keyboardBridgePlatform) Mount(b *driver.MountBatch) {
	for _, it := range b.Items {
		switch it.Kind {
		case driver.ItemCreate:
			p.m.views[it.ID] = &view{id: it.ID, kind: it.View}
			if it.View == driver.KindInput {
				p.input = it.ID
			}
		case driver.ItemEvents:
			p.m.views[it.ID].events = it.Events
			if it.Events.Has(driver.EventKeyDown) {
				p.listeners = append(p.listeners, it.ID)
			}
		case driver.ItemKeymap:
			p.m.keys = slices.Clone(it.Keys)
			p.m.heldKeys = it.HeldKeys
		}
	}
}
func (*keyboardBridgePlatform) RequestFrame()             {}
func (*keyboardBridgePlatform) SubmitGPU(driver.GPUFrame) {}

func TestNativeKeyBridgeOrdersCallbacksAndItsGoKeymap(t *testing.T) {
	s := &surface{}
	s.m = newMounter(s)
	p := &keyboardBridgePlatform{m: s.m}
	var heard []string
	s.engine = engine.Start(p, func(cx *core.Scope) core.Node {
		_ = core.KeysDown(cx)
		for _, name := range []string{"k", "x"} {
			cx.OnKeyWith(name, core.KeyOptions{When: core.KeyAlways}, func() { heard = append(heard, "binding "+name) })
		}
		control := func(label string) core.Node {
			return core.View{OnKeyDown: func(e core.KeyEvent) bool {
				heard = append(heard, label+" down "+e.Key+" "+strings.Join(core.KeysDown(cx), ","))
				return e.Key == "k"
			}, OnKeyUp: func(e core.KeyEvent) { heard = append(heard, label+" up "+e.Key) }}
		}
		return core.View{Children: []core.Node{control("first"), control("second"), core.Input{}}}
	}, engine.Options{})
	defer s.engine.Close()
	s.engine.Sync()
	if len(p.listeners) != 2 {
		t.Fatalf("listeners = %v", p.listeners)
	}
	first, second := p.listeners[0], p.listeners[1]
	event := func(name string, id driver.NodeID, x, y int) {
		s.event([]js.Value{js.ValueOf(name), js.ValueOf(int(id)), js.ValueOf(x), js.ValueOf(y), js.ValueOf("")})
	}
	wait := func() {
		t.Helper()
		until := time.Now().Add(5 * time.Second)
		for s.m.keyRunning && time.Now().Before(until) {
			time.Sleep(time.Millisecond)
		}
		if s.m.keyRunning {
			t.Fatal("key queue did not drain")
		}
		s.engine.Sync()
	}
	event("focus", first, 0, 0)
	event("key", 2027, 0, 0) // k: consume before Go keymap, no native consumption claim.
	event("key", 2027, 1, 0)
	event("key", 2040, 0, 0) // x: unconsumed binding remains between its down and up.
	event("key", 2040, 1, 0)
	event("focus", second, 0, 0) // Captured downs keep the focus they had before the Go queue ran.
	wait()
	want := []string{"first down k k", "first up k", "first down x x", "binding x", "first up x"}
	if !slices.Equal(heard, want) {
		t.Fatalf("bridge order = %v, want %v", heard, want)
	}
	heard = nil
	event("focus", first, 0, 0)
	event("key", 2018, 0, 0) // b
	wait()
	event("focus", second, 0, 0)
	oldMask := s.m.views[second].events
	s.m.views[second].events = 0
	s.m.heldKeys = false
	event("key", 2018, 0, 0) // Repeats still belong to the original listener without a global observer.
	event("key", 2018, 1, 0)
	wait()
	s.m.views[second].events = oldMask
	s.m.heldKeys = true
	if !slices.Equal(heard, []string{"first down b b", "first down b b", "first up b"}) {
		t.Fatalf("release followed changed focus: %v", heard)
	}
	heard = nil
	event("key", 2047, 0, 4) // Shift, then a key whose native flags omit it.
	event("key", 2017, 0, 0)
	event("key", 2017, 0, 0) // Repeat must not add another held key.
	event("key", 2017, 1, 4)
	event("key", 2047, 1, 4)
	wait()
	if !slices.Equal(heard, []string{"second down shift shift", "second down a a,shift", "second down a a,shift", "second up a", "second up shift"}) {
		t.Fatalf("modifier/repeat bridge = %v", heard)
	}
	if len(s.m.held) != 0 {
		t.Fatalf("keys remained held: %v", s.m.held)
	}
	heard = nil
	event("focus", p.input, 0, 0)
	event("blur", second, 0, 0) // A late blur must not erase the new editor's focus.
	s.m.focused = first         // An unsupported plain-view focus request cannot take the native editor's keys.
	event("key", 2017, 0, 0)
	event("key", 2017, 1, 0)
	wait()
	if len(heard) != 0 {
		t.Fatalf("native editor keys leaked to a view: %v", heard)
	}
	event("focus", first, 0, 0)
	event("key", 2018, 0, 0)
	s.m.key(0, 2, 0, "") // Lifecycle loss, ordered after the still-pending down.
	wait()
	if !slices.Equal(heard, []string{"first down b b", "first up b"}) || len(s.m.held) != 0 {
		t.Fatalf("lifecycle left a key held: %v / %v", heard, s.m.held)
	}
}
