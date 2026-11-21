//go:build js && wasm && nsgotest

package web

import (
	"log"
	"strings"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

type screenLog chan string

func (l screenLog) Write(p []byte) (int, error) {
	select {
	case l <- string(p):
	default:
	}
	return len(p), nil
}

func TestBrowserFullscreenAndOrientationFromPress(t *testing.T) {
	doc := js.Global().Get("document")
	if !doc.Get("fullscreenEnabled").Truthy() {
		t.Skip("requires browser full screen")
	}
	orientation := js.Global().Get("screen").Get("orientation")
	if !orientation.Truthy() || orientation.Get("lock").Type() != js.TypeFunction {
		t.Skip("requires the screen orientation API")
	}
	logs := make(screenLog, 16)
	defer log.SetOutput(log.Writer())
	log.SetOutput(logs)
	mount := doc.Call("createElement", "div")
	mount.Set("id", "nsgo-browser-screen-test")
	mount.Get("style").Set("height", "240px")
	doc.Get("body").Call("appendChild", mount)
	defer mount.Call("remove")
	defer func() {
		if doc.Get("fullscreenElement").Truthy() {
			doc.Call("exitFullscreen")
		}
	}()

	type lockAttempt struct {
		mode       string
		fullscreen bool
	}
	locks := make(chan lockAttempt, 2)
	nativeLock := orientation.Get("lock")
	lock := js.FuncOf(func(_ js.Value, args []js.Value) any {
		locks <- lockAttempt{args[0].String(), doc.Get("fullscreenElement").Truthy()}
		return nativeLock.Call("call", orientation, args[0])
	})
	orientation.Set("lock", lock)
	defer lock.Release()
	defer orientation.Delete("lock")

	s := open(func(cx *core.Scope) core.Node {
		playing := core.NewSignal(cx, false)
		// Neither operation is allowed before the trusted press opens full screen.
		cx.RequestOrientation(core.OrientationPortrait)
		cx.AfterFunc(time.Millisecond, cx.ToggleFullScreen)
		return core.View{Children: []core.Node{
			core.View{TestID: "play", Style: core.Style{Width: core.Pt(160), Height: core.Pt(40)},
				OnPress: func(core.PressEvent) {
					cx.ToggleFullScreen()
					playing.Set(true)
				}, Children: []core.Node{core.Text{Content: "Play"}}},
			core.Show{When: playing.Get, Then: func(cx *core.Scope) core.Node {
				cx.RequestOrientation(core.OrientationLandscape)
				return core.View{}
			}},
		}}
	}, Options{Mount: "#nsgo-browser-screen-test"}, nil)
	defer s.engine.Close()
	wantLogs := func(operations ...string) {
		t.Helper()
		deadline := time.After(4 * time.Second)
		for len(operations) != 0 {
			select {
			case msg := <-logs:
				for i, operation := range operations {
					if strings.Contains(msg, "nsgo: "+operation+": ") {
						operations = append(operations[:i], operations[i+1:]...)
						break
					}
				}
			case <-deadline:
				t.Fatalf("did not report refusals for %v", operations)
			}
		}
	}
	wantLogs("enter full screen", "lock screen orientation")
	if s.browserFullscreen() {
		t.Fatal("entered full screen without a trusted press")
	}
	wantLock := func(want lockAttempt) {
		t.Helper()
		select {
		case got := <-locks:
			if got != want {
				t.Fatalf("orientation lock = %+v, want %+v", got, want)
			}
		case <-time.After(4 * time.Second):
			t.Fatalf("did not request orientation lock %+v", want)
		}
	}
	wantLock(lockAttempt{"portrait-primary", false})
	button := mount.Call("querySelector", "[data-testid=play]")
	rect := button.Call("getBoundingClientRect")
	if err := (&testSurface{s: s}).do(map[string]any{
		"op": "press", "x": rect.Get("left").Float() + 12, "y": rect.Get("top").Float() + 12, "hold": 0,
	}); err != nil {
		t.Fatal(err)
	}
	wantLock(lockAttempt{"landscape", true})
}
