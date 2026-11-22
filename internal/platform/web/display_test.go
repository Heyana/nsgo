//go:build js && wasm

package web

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestDisplayRefreshRateIsEstimated(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Truthy() {
		t.Skip("no document here: run with nsgo test -target web")
	}
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-display-rate")
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")

	rates := make(chan core.DisplayInfo, 1)
	s := open(func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: func() string {
			display := cx.Env().Display()
			if display.RefreshRate > 0 {
				select {
				case rates <- display:
				default:
				}
			}
			return "Display"
		}}
	}, Options{Mount: "#nsgo-display-rate"}, nil)
	defer s.engine.Close()

	select {
	case display := <-rates:
		if display.ID == "" || !display.RefreshRateEstimated || display.MaxRefreshRate != 0 {
			t.Fatalf("browser display = %+v", display)
		}
		t.Logf("estimated %.0f Hz", display.RefreshRate)
	case <-time.After(5 * time.Second):
		t.Fatal("no display refresh estimate reached the tracked read")
	}
}
