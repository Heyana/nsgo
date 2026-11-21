//go:build linux && !android

package linux

import (
	"fmt"
	"reflect"
	"testing"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

type portalTestPlatform struct{ core.ApproxTextEngine }

func (portalTestPlatform) Mount(*driver.MountBatch)  {}
func (portalTestPlatform) RequestFrame()             {}
func (portalTestPlatform) SubmitGPU(driver.GPUFrame) {}

// TestPortalBindingResponse feeds real GVariant replies to the signal handler without a display, session bus, or compositor permission dialog.
func TestPortalBindingResponse(t *testing.T) {
	if err := openLibs(); err != nil {
		t.Skip(err)
	}
	bindGlobalHotKeys()
	for _, tt := range []struct {
		name, results string
		first, second driver.HotKeyState
		x11Taken      bool
	}{
		{"all", "{'shortcuts': <@a(sa{sv}) [('second', {}), ('first', {})]>}", driver.HotKeyRegistered, driver.HotKeyRegistered, false},
		{"partial", "{'shortcuts': <@a(sa{sv}) [('second', {'trigger_description': <'Ctrl+Alt+K'>})]>}", driver.HotKeyTaken, driver.HotKeyRegistered, false},
		{"empty", "{'shortcuts': <@a(sa{sv}) []>}", driver.HotKeyTaken, driver.HotKeyTaken, false},
		{"unknown", "{'shortcuts': <@a(sa{sv}) [('unknown', {}), ('second', {}), ('second', {})]>}", driver.HotKeyTaken, driver.HotKeyRegistered, false},
		{"missing", "{}", driver.HotKeyUnsupported, driver.HotKeyUnsupported, false},
		{"wrong-type", "{'shortcuts': <'second'>}", driver.HotKeyUnsupported, driver.HotKeyUnsupported, false},
		{"missing-with-x11-conflict", "{}", driver.HotKeyTaken, driver.HotKeyUnsupported, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var events []driver.Event
			e := engine.Start(portalTestPlatform{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{
				CheckCommits: true,
				Event:        func(_ uint32, ev driver.Event) { events = append(events, ev) },
			})
			t.Cleanup(e.Close)
			state := &globalHotKeys{
				stage: globalHotKeyBinding, request: "/request/bind", session: "/session/keys",
				ids:   map[string]uint32{"first": 11, "second": 22},
				specs: []globalHotKeySpec{{id: "first", key: 11}, {id: "second", key: 22}},
			}
			if tt.x11Taken {
				state.x11Taken = map[uint32]bool{11: true}
			}
			w := &window{engine: e, hotKeys: state}
			w.handle = registerWindow(w)
			t.Cleanup(func() { windowHandles.Delete(w.handle) })
			params := portalVariant("(ua{sv})", fmt.Sprintf("(uint32 0, %s)", tt.results))
			if params == 0 {
				t.Fatal("the response did not parse")
			}
			defer gVariantUnref(params)
			globalHotKeyResponse(0, 0, uintptr(unsafe.Pointer(cstr(state.request))), 0, 0, params, w.handle)
			want := []driver.Event{
				{Kind: driver.EventHotKeyState, Count: 11, Index: int(tt.first)},
				{Kind: driver.EventHotKeyState, Count: 22, Index: int(tt.second)},
			}
			if !reflect.DeepEqual(events, want) {
				t.Fatalf("registration events = %v, want %v", events, want)
			}
			// A session that bound something stays open for its activations; one that bound nothing is closed.
			kept := tt.first == driver.HotKeyRegistered || tt.second == driver.HotKeyRegistered
			if (w.hotKeys == state) != kept {
				t.Fatalf("session kept = %v, want %v", w.hotKeys == state, kept)
			}
			// Only a shortcut confirmed bound may dispatch an activation.
			events = nil
			for _, id := range []string{"first", "second", "unknown"} {
				activation := portalVariant("(osta{sv})", fmt.Sprintf("(objectpath '/session/keys', %s, uint64 0, @a{sv} {})", portalQuote(id)))
				if activation == 0 {
					t.Fatal("the activation did not parse")
				}
				globalHotKeyActivated(0, 0, 0, 0, 0, activation, w.handle)
				gVariantUnref(activation)
			}
			want = nil
			if tt.first == driver.HotKeyRegistered {
				want = append(want, driver.Event{Kind: driver.EventHotKey, Count: 11})
			}
			if tt.second == driver.HotKeyRegistered {
				want = append(want, driver.Event{Kind: driver.EventHotKey, Count: 22})
			}
			if !reflect.DeepEqual(events, want) {
				t.Fatalf("activation events = %v, want %v", events, want)
			}
		})
	}
}
