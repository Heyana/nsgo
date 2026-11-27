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

type harmonyEditFixture struct {
	s        *surface
	host     *testSurface
	v        *view
	rendered *string
	clipsets *[]string
	cleanup  func()
}

func newHarmonyEditFixture(t *testing.T, value string, cfg driver.InputConfig, app func(*core.Scope) core.Node) harmonyEditFixture {
	t.Helper()
	if app == nil {
		app = func(*core.Scope) core.Node { return core.View{} }
	}
	s := &surface{engine: engine.Start(selectionTestPlatform{}, app, engine.Options{})}
	s.m = newMounter(s)
	v := &view{id: 4, kind: driver.KindInput, value: value}
	s.m.views[4] = v
	s.m.input(4, &cfg)
	s.m.o.reset()
	s.m.focused = 4
	s.m.nativeFocused = 4
	bridge := js.Global().Get("Object").New()
	s.bridge = bridge
	var rendered string
	var clipsets []string
	submit := js.FuncOf(func(_ js.Value, args []js.Value) any {
		var ops [][]any
		if err := json.Unmarshal([]byte(args[0].String()), &ops); err != nil {
			t.Error(err)
			return nil
		}
		for _, op := range ops {
			if op[0] != "sel" || op[1] != float64(4) {
				continue
			}
			start, end := int(op[2].(float64)), int(op[3].(float64))
			time.AfterFunc(5*time.Millisecond, func() {
				s.event([]js.Value{js.ValueOf("selection"), js.ValueOf(4), js.ValueOf(start), js.ValueOf(end), js.ValueOf("")})
			})
		}
		return nil
	})
	typeInput := js.FuncOf(func(_ js.Value, args []js.Value) any {
		rendered = args[1].String()
		return nil
	})
	clipset := js.FuncOf(func(_ js.Value, args []js.Value) any {
		clipsets = append(clipsets, args[0].String())
		return nil
	})
	bridge.Set("submit", submit)
	bridge.Set("typeInput", typeInput)
	bridge.Set("clipset", clipset)
	return harmonyEditFixture{
		s: s, host: &testSurface{s: s}, v: v, rendered: &rendered, clipsets: &clipsets,
		cleanup: func() {
			submit.Release()
			typeInput.Release()
			clipset.Release()
			s.engine.Close()
		},
	}
}

func TestNativeClipboardSelectAllAndBackspaceKeys(t *testing.T) {
	f := newHarmonyEditFixture(t, "hello world", driver.InputConfig{}, nil)
	defer f.cleanup()

	if _, err := f.host.SelectRange(driver.TextRange{Start: 0, End: 5}); err != nil {
		t.Fatal(err)
	}
	if err := f.host.Key("mod+c"); err != nil {
		t.Fatal(err)
	}
	if got := *f.clipsets; len(got) != 1 || got[0] != "hello" {
		t.Fatalf("copy clipsets = %v, want [hello]", got)
	}
	if _, err := f.host.SelectRange(driver.TextRange{Start: 6, End: 11}); err != nil {
		t.Fatal(err)
	}
	if err := f.host.Key("Mod+V"); err != nil {
		t.Fatal(err)
	}
	if *f.rendered != "hello hello" || f.v.value != "hello hello" || f.v.selection != (driver.TextRange{Start: 11, End: 11}) {
		t.Fatalf("after paste: rendered %q, value %q, selection %v", *f.rendered, f.v.value, f.v.selection)
	}

	// Empty selection copy must keep the existing clipboard.
	if err := f.host.Key("mod+c"); err != nil {
		t.Fatal(err)
	}
	if f.s.m.clipboard != "hello" || len(*f.clipsets) != 1 {
		t.Fatalf("empty copy overwrote clipboard: %q / %v", f.s.m.clipboard, *f.clipsets)
	}

	// Cut over UTF-16 supplementary emoji and combining accent, then paste at the end.
	f.v.value = "A😀e\u0301Z"
	if _, err := f.host.SelectRange(driver.TextRange{Start: 1, End: 5}); err != nil {
		t.Fatal(err)
	}
	if err := f.host.Key("ctrl+x"); err != nil {
		t.Fatal(err)
	}
	if *f.rendered != "AZ" || f.v.value != "AZ" || f.v.selection != (driver.TextRange{Start: 1, End: 1}) {
		t.Fatalf("after cut: rendered %q, value %q, selection %v", *f.rendered, f.v.value, f.v.selection)
	}
	if got := (*f.clipsets)[len(*f.clipsets)-1]; got != "😀e\u0301" {
		t.Fatalf("cut clipboard = %q", got)
	}
	if _, err := f.host.SelectRange(driver.TextRange{Start: 2, End: 2}); err != nil {
		t.Fatal(err)
	}
	if err := f.host.Key("mod+v"); err != nil {
		t.Fatal(err)
	}
	if *f.rendered != "AZ😀e\u0301" || f.v.selection != (driver.TextRange{Start: 6, End: 6}) {
		t.Fatalf("after UTF-16 paste: rendered %q, selection %v", *f.rendered, f.v.selection)
	}

	// Select-all followed by Backspace clears the field.
	if err := f.host.Key("mod+a"); err != nil {
		t.Fatal(err)
	}
	if f.v.selection != (driver.TextRange{Start: 0, End: 6}) {
		t.Fatalf("select-all selection = %v, want {0 6}", f.v.selection)
	}
	if err := f.host.Key("Backspace"); err != nil {
		t.Fatal(err)
	}
	if *f.rendered != "" || f.v.value != "" || f.v.selection != (driver.TextRange{Start: 0, End: 0}) {
		t.Fatalf("after select-all+backspace: rendered %q, value %q, selection %v", *f.rendered, f.v.value, f.v.selection)
	}

	// App clipboard writes (ItemSetClipboard) feed mod+v, normalizing line breaks in single-line fields.
	f.s.m.mount(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemSetClipboard, Value: "one\r\ntwo\nthree"}}})
	if err := f.host.Key("mod+v"); err != nil {
		t.Fatal(err)
	}
	if *f.rendered != "one two three" {
		t.Fatalf("single-line paste = %q, want %q", *f.rendered, "one two three")
	}
}

func TestNativeClipboardKeysRespectFieldRulesAndKeymap(t *testing.T) {
	// MaxLength, PasteReport, Masked and Readonly rules.
	f := newHarmonyEditFixture(t, "secret", driver.InputConfig{Masked: true}, nil)
	defer f.cleanup()
	if _, err := f.host.SelectRange(driver.TextRange{Start: 0, End: 6}); err != nil {
		t.Fatal(err)
	}
	if err := f.host.Key("mod+c"); err != nil {
		t.Fatal(err)
	}
	if err := f.host.Key("mod+x"); err != nil {
		t.Fatal(err)
	}
	if f.v.value != "secret" || f.s.m.clipboard != "" || len(*f.clipsets) != 0 {
		t.Fatalf("masked field leaked or edited: value %q, clip %q, calls %v", f.v.value, f.s.m.clipboard, *f.clipsets)
	}

	// Readonly allows copy and select-all, blocks cut, paste and backspace.
	f.s.m.input(4, &driver.InputConfig{Readonly: true})
	f.s.m.o.reset()
	if err := f.host.Key("mod+a"); err != nil {
		t.Fatal(err)
	}
	if err := f.host.Key("mod+c"); err != nil {
		t.Fatal(err)
	}
	if f.s.m.clipboard != "secret" {
		t.Fatalf("readonly copy = %q, want secret", f.s.m.clipboard)
	}
	for _, key := range []string{"mod+x", "mod+v", "backspace"} {
		if err := f.host.Key(key); err != nil {
			t.Fatal(err)
		}
	}
	if f.v.value != "secret" {
		t.Fatalf("readonly field edited: %q", f.v.value)
	}

	// MaxLength clips pasted text.
	f.s.m.input(4, &driver.InputConfig{MaxLength: 5})
	f.s.m.o.reset()
	f.v.value = "ab"
	if _, err := f.host.SelectRange(driver.TextRange{Start: 2, End: 2}); err != nil {
		t.Fatal(err)
	}
	if err := f.host.Key("mod+v"); err != nil {
		t.Fatal(err)
	}
	if f.v.value != "absec" {
		t.Fatalf("maxLength paste = %q, want absec", f.v.value)
	}

	// PasteReport reports EventPaste without modifying the field, and KeyAlways
	// bindings take precedence over native editing keys while KeyAuto yields.
	s2 := &surface{}
	s2.m = newMounter(s2)
	p2 := &keyboardBridgePlatform{m: s2.m}
	var alwaysFired, autoFired int
	var pasted []string
	s2.engine = engine.Start(p2, func(cx *core.Scope) core.Node {
		cx.OnKeyWith("mod+x", core.KeyOptions{When: core.KeyAlways}, func() { alwaysFired++ })
		cx.OnKeyWith("mod+c", core.KeyOptions{When: core.KeyAuto}, func() { autoFired++ })
		return core.Input{
			DefaultValue:  "hello",
			PasteBehavior: core.PasteReport,
			OnPaste:       func(e core.PasteEvent) { pasted = append(pasted, e.Text) },
		}
	}, engine.Options{})
	defer s2.engine.Close()
	s2.engine.Sync()
	in := s2.m.views[p2.input]
	in.value = "hello"
	in.pasteBehavior = driver.PasteReport
	s2.m.focused = p2.input
	s2.m.nativeFocused = p2.input
	bridge2 := js.Global().Get("Object").New()
	s2.bridge = bridge2
	submit2 := js.FuncOf(func(_ js.Value, args []js.Value) any {
		var ops [][]any
		if err := json.Unmarshal([]byte(args[0].String()), &ops); err == nil {
			for _, op := range ops {
				if op[0] == "sel" && op[1] == float64(p2.input) {
					start, end := int(op[2].(float64)), int(op[3].(float64))
					time.AfterFunc(5*time.Millisecond, func() {
						s2.event([]js.Value{js.ValueOf("selection"), js.ValueOf(int(p2.input)), js.ValueOf(start), js.ValueOf(end), js.ValueOf("")})
					})
				}
			}
		}
		return nil
	})
	defer submit2.Release()
	bridge2.Set("submit", submit2)
	host2 := &testSurface{s: s2}
	if _, err := host2.SelectRange(driver.TextRange{Start: 0, End: 5}); err != nil {
		t.Fatal(err)
	}
	if err := host2.Key("mod+x"); err != nil {
		t.Fatal(err)
	}
	s2.engine.Sync()
	if alwaysFired != 1 || in.value != "hello" || s2.m.clipboard != "" {
		t.Fatalf("KeyAlways mod+x: fired=%d value=%q clip=%q", alwaysFired, in.value, s2.m.clipboard)
	}
	if err := host2.Key("mod+c"); err != nil {
		t.Fatal(err)
	}
	s2.engine.Sync()
	if autoFired != 0 || s2.m.clipboard != "hello" {
		t.Fatalf("KeyAuto mod+c: fired=%d clip=%q", autoFired, s2.m.clipboard)
	}
	if err := host2.Key("mod+v"); err != nil {
		t.Fatal(err)
	}
	s2.engine.Sync()
	if in.value != "hello" || len(pasted) != 1 || pasted[0] != "hello" {
		t.Fatalf("PasteReport mod+v: value=%q pasted=%v", in.value, pasted)
	}
}
