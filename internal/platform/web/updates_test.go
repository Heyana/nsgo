//go:build js && wasm

package web

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

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
