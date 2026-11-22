//go:build js && wasm

package web

import (
	"fmt"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWebReleaseChecksAndPreparedReload(t *testing.T) {
	if !js.Global().Get("document").Truthy() {
		t.Skip("run with nsgo test -target web")
	}
	const current = `{"release":true,"version":"1.0","wasm":"app.old.wasm"}`
	for _, test := range []struct {
		name, latest string
		ready        bool
		failure      bool
	}{
		{"unchanged", current, false, false},
		{"new version", `{"wasm":"app.new.wasm","version":"2.0","release":true}`, true, false},
		{"same version changed files", `{"release":true,"version":"1.0","wasm":"app.new.wasm"}`, true, false},
		{"invalid descriptor", `{"release":false,"wasm":"app.new.wasm"}`, false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			wasFetch := js.Global().Get("fetch")
			calls := 0
			mock := js.FuncOf(func(_ js.Value, args []js.Value) any {
				calls++
				if args[1].Get("cache").String() != "no-store" {
					t.Error("release check permits stale HTTP cache")
				}
				response := js.Global().Get("Response").New(test.latest, map[string]any{"status": 200})
				return js.Global().Get("Promise").Call("resolve", response)
			})
			js.Global().Set("fetch", mock)
			defer func() { js.Global().Set("fetch", wasFetch); mock.Release() }()
			results := make(chan core.UpdateCheck, 3)
			s := open(func(cx *core.Scope) core.Node {
				for range 3 {
					cx.CheckForUpdates(false, func(found core.UpdateCheck) { results <- found })
				}
				return core.View{}
			}, Options{UpdateConfig: current}, nil)
			defer s.engine.Close()
			for range 3 {
				select {
				case found := <-results:
					if found.Restart != test.ready || (found.Err != nil) != test.failure {
						t.Fatalf("answer=%+v", found)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("release check did not answer")
				}
			}
			if calls != 1 {
				t.Fatalf("concurrent checks fetched %d times", calls)
			}
			wasReload := reloadWebPage
			reloaded := 0
			reloadWebPage = func() { reloaded++ }
			defer func() { reloadWebPage = wasReload }()
			s.Mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemQuit}}})
			if reloaded != 0 {
				t.Fatal("ordinary quit reloaded the page")
			}
			s.Mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemQuit, On: true}}})
			want := 0
			if test.ready {
				want = 1
			}
			if reloaded != want {
				t.Fatal(fmt.Sprintf("reloads=%d want=%d", reloaded, want))
			}
		})
	}
}

func TestInstallUpdateReportsUnavailable(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-install-update")
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")

	result := make(chan error, 1)
	s := open(func(cx *core.Scope) core.Node {
		core.InstallUpdate(cx, "/update.deb", func(err error) { result <- err })
		return core.View{}
	}, Options{Mount: "#nsgo-install-update"}, nil)
	defer s.engine.Close()

	select {
	case err := <-result:
		if core.ErrorCode(err) != core.CodeUnavailable {
			t.Fatalf("install update = %v, want an unavailable service", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the backend did not answer the install request")
	}
}
