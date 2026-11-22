//go:build js && wasm && nsgotest

package harmonyos

import (
	"encoding/json"
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

type selectionTestPlatform struct{ core.ApproxTextEngine }

func (selectionTestPlatform) Mount(*driver.MountBatch)  {}
func (selectionTestPlatform) RequestFrame()             {}
func (selectionTestPlatform) SubmitGPU(driver.GPUFrame) {}

func TestNativeSelectWaitsForArkUIAndTypesOverItsRange(t *testing.T) {
	s := &surface{engine: engine.Start(selectionTestPlatform{}, func(*core.Scope) core.Node {
		return core.View{}
	}, engine.Options{})}
	defer s.engine.Close()
	s.m = newMounter(s)
	s.m.focused = 4
	v := &view{id: 4, kind: driver.KindInput, value: "A😀e\u0301Z"}
	s.m.views[4] = v
	bridge := js.Global().Get("Object").New()
	s.bridge = bridge
	var rendered string
	var callbacks int
	submit := js.FuncOf(func(_ js.Value, args []js.Value) any {
		var ops [][]any
		if err := json.Unmarshal([]byte(args[0].String()), &ops); err != nil {
			t.Error(err)
			return nil
		}
		for _, op := range ops {
			if op[0] != "sel" || op[1] != float64(4) {
				t.Errorf("unexpected renderer operation: %v", op)
				continue
			}
			start, end := int(op[2].(float64)), int(op[3].(float64))
			// Like ArkUI, report after the controller applies the request on
			// its next turn, using the same native-event bridge as the page.
			time.AfterFunc(10*time.Millisecond, func() {
				callbacks++
				s.event([]js.Value{js.ValueOf("selection"), js.ValueOf(4), js.ValueOf(start), js.ValueOf(end), js.ValueOf("")})
			})
		}
		return nil
	})
	defer submit.Release()
	typeInput := js.FuncOf(func(_ js.Value, args []js.Value) any {
		rendered = args[1].String()
		return nil
	})
	defer typeInput.Release()
	bridge.Set("submit", submit)
	bridge.Set("typeInput", typeInput)
	host := &testSurface{s: s}
	want := driver.TextRange{Start: 3, End: 5}
	if actual, err := host.SelectRange(want); err != nil || actual != want || callbacks != 1 {
		t.Fatalf("native selection = %v, %v; callbacks = %d", actual, err, callbacks)
	}
	if err := host.Type("X"); err != nil {
		t.Fatal(err)
	}
	if rendered != "A😀XZ" || v.value != rendered || v.selection != (driver.TextRange{Start: 4, End: 4}) || callbacks != 2 {
		t.Fatalf("typed selection: rendered %q, model %q, native caret %v, callbacks %d", rendered, v.value, v.selection, callbacks)
	}
	s.m.focused = 0
	if _, err := host.SelectRange(want); err == nil {
		t.Fatal("selection succeeded without a focused native field")
	}
}
