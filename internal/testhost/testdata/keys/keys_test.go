package keys_test

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

type fieldEvents struct {
	bindings, submits, blurs atomic.Int32
	text, selection          atomic.Value
}

func startField(t *testing.T, key string, options *core.KeyOptions) (*nsgotest.App, *fieldEvents) {
	t.Helper()
	events := new(fieldEvents)
	events.text.Store("")
	events.selection.Store(core.TextRange{})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		value := core.NewSignal(cx, "")
		if options != nil {
			cx.OnKeyWith(key, *options, func() { events.bindings.Add(1) })
		}
		return core.Input{
			TestID: "field", AutoFocus: true, Value: value.Get,
			Style: core.Style{Width: core.Pt(240), Height: core.Pt(40)},
			OnChange: func(text string) {
				events.text.Store(text)
				value.Set(text)
			},
			OnSubmit:          func(string) { events.submits.Add(1) },
			OnBlur:            func() { events.blurs.Add(1) },
			OnSelectionChange: func(r core.TextRange) { events.selection.Store(r) },
		}
	}, nsgotest.Size(300, 120))
	app.Type("id:field", "entry")
	if events.text.Load() != "entry" || !app.Find("id:field").Focused {
		t.Fatal("the field did not receive the setup text and focus")
	}
	return app, events
}

// Editing keys take the same keymap path as ordinary keys, before an editor acts.
func TestEditingKeysHonorAlwaysBindings(t *testing.T) {
	for _, key := range []string{"enter", "return", "backspace", "mod+a"} {
		t.Run(key, func(t *testing.T) {
			app, events := startField(t, key, &core.KeyOptions{When: core.KeyAlways})
			selection := events.selection.Load()
			for n := range 2 {
				app.Key(key)
				if got := events.bindings.Load(); got != int32(n+1) {
					t.Fatalf("%s fired %d times, want %d; each press must release the binding", key, got, n+1)
				}
				if events.text.Load() != "entry" || events.submits.Load() != 0 || events.blurs.Load() != 0 ||
					!app.Find("id:field").Focused || events.selection.Load() != selection {
					t.Fatalf("%s reached the editor despite its binding: text=%q submits=%d blurs=%d selection=%v",
						key, events.text.Load(), events.submits.Load(), events.blurs.Load(), events.selection.Load())
				}
			}
		})
	}
}

func TestEditingKeysKeepTheirUnboundActions(t *testing.T) {
	for _, binding := range []struct {
		name    string
		options *core.KeyOptions
	}{
		{"unbound", nil},
		{"auto", &core.KeyOptions{When: core.KeyAuto}},
		{"not-editing", &core.KeyOptions{When: core.KeyNotEditing}},
	} {
		t.Run(binding.name, func(t *testing.T) {
			for _, key := range []string{"enter", "return", "backspace", "mod+a"} {
				t.Run(key, func(t *testing.T) {
					app, events := startField(t, key, binding.options)
					app.Key(key)
					if events.bindings.Load() != 0 {
						t.Fatal("a binding that yields to text editing fired in the field")
					}
					switch key {
					case "enter", "return":
						if events.submits.Load() != 1 || events.blurs.Load() != 1 || app.Find("id:field").Focused {
							t.Fatalf("Return: submits=%d blurs=%d focused=%v", events.submits.Load(), events.blurs.Load(), app.Find("id:field").Focused)
						}
					case "backspace":
						if events.text.Load() != "entr" {
							t.Fatalf("Backspace left %q, want entr", events.text.Load())
						}
					case "mod+a":
						if got := events.selection.Load(); got != (core.TextRange{Start: 0, End: 5}) {
							t.Fatalf("select all selected %v, want 0-5", got)
						}
					}
					if key == "backspace" || key == "mod+a" {
						if events.submits.Load() != 0 || events.blurs.Load() != 0 || !app.Find("id:field").Focused {
							t.Fatal("editing submitted or blurred the field")
						}
					}
				})
			}
		})
	}
}

// logicalKey keeps the parts these tests exercise with layout-dependent steps. Some target hosts can also provide a position for those steps.
func logicalKey(e core.KeyEvent) core.KeyEvent {
	e.Code = ""
	return e
}

// A focused view hears the keys that go down on it before the keymap, with their modifiers, a modifier alone included, and a key it consumes fires no binding (#287).
func TestAFocusedViewHearsItsKeysBeforeTheKeymap(t *testing.T) {
	switch nsgotest.Target() {
	case "", "macos", "windows", "linux", "web":
	default:
		t.Skip("key downs on mobile are tracked in the roadmap")
	}
	var got []core.KeyEvent
	var bindings atomic.Int32
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		for _, key := range []string{"alt+space", "mod+shift+k", "f5", "x"} {
			cx.OnKeyWith(key, core.KeyOptions{When: core.KeyAlways}, func() { bindings.Add(1) })
		}
		return core.View{Style: core.Style{Padding: core.All(20)}, Children: []core.Node{core.View{
			TestID: "recorder", Focusable: func() bool { return true },
			OnKeyDown: func(e core.KeyEvent) bool { got = append(got, logicalKey(e)); return e.Key != "x" },
			Style:     core.Style{Width: core.Pt(200), Height: core.Pt(40)},
		}}}
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}), nsgotest.Size(300, 120))
	app.Focus("id:recorder")
	// "mod" is Command on a Mac, where a browser runs too, and Control elsewhere.
	app.Key("mod+shift+k")
	mod := core.Modifiers{Control: true}
	if len(got) == 1 && got[0].Modifiers.Platform {
		mod = core.Modifiers{Platform: true}
	}
	modShiftK := "ctrl+shift+k"
	if mod.Platform {
		modShiftK = "shift+meta+k"
	}
	for _, c := range []struct {
		key, shortcut string
		want          core.KeyEvent
	}{
		{"alt+space", "alt+space", core.KeyEvent{Key: "space", Modifiers: core.Modifiers{Alt: true}}},
		{"mod+shift+k", modShiftK, core.KeyEvent{Key: "k", Modifiers: core.Modifiers{Control: mod.Control, Shift: true, Platform: mod.Platform}}},
		{"f5", "f5", core.KeyEvent{Key: "f5"}},
		{"shift", "", core.KeyEvent{Key: "shift", Modifiers: core.Modifiers{Shift: true}}},
		{"alt", "", core.KeyEvent{Key: "alt", Modifiers: core.Modifiers{Alt: true}}},
	} {
		got = nil
		app.Key(c.key)
		if len(got) != 1 || got[0] != c.want {
			t.Fatalf("%s: the view heard %+v, want %+v", c.key, got, c.want)
		}
		if s := got[0].Shortcut(); s != c.shortcut {
			t.Fatalf("%s: shortcut %q, want %q", c.key, s, c.shortcut)
		}
	}
	if n := bindings.Load(); n != 0 {
		t.Fatalf("keys the view consumed fired %d bindings", n)
	}
	got = nil
	app.Key("x")
	if len(got) != 1 || bindings.Load() != 1 {
		t.Fatalf("a key the view left: heard %d times, %d bindings; want 1 and 1", len(got), bindings.Load())
	}
}

// A focused view hears a key come up after it went down, a letter, an arrow, Space and a modifier alone, with the modifiers still held, a modifier held over a letter joins it, a key held again goes down as its auto-repeat and comes up once, and core.KeysDown reads the keys held meanwhile (#1046).
func TestAFocusedViewHearsItsKeysComeUp(t *testing.T) {
	target := nsgotest.Target()
	switch target {
	case "", "macos", "windows", "linux", "web", "tvos_simulator":
	default:
		t.Skip("iPhone, iPad and Android report no OnKeyUp; TestKeysDownFollowsAHardwareKeyboard covers iPhone and iPad")
	}
	var mu sync.Mutex
	var downs, ups []core.KeyEvent
	var recorder core.Ref
	var focused atomic.Bool
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.All(20)}, Children: []core.Node{
			core.View{
				TestID: "recorder", Ref: &recorder, Focusable: func() bool { return true }, OnFocus: func() { focused.Store(true) },
				OnKeyDown: func(e core.KeyEvent) bool { mu.Lock(); downs = append(downs, logicalKey(e)); mu.Unlock(); return false },
				OnKeyUp:   func(e core.KeyEvent) { mu.Lock(); ups = append(ups, logicalKey(e)); mu.Unlock() },
				Style:     core.Style{Width: core.Pt(200), Height: core.Pt(40)},
			},
			core.Text{TestID: "held", ContentFunc: func() string { return strings.Join(core.KeysDown(cx), " ") }},
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}), nsgotest.Size(300, 120))
	if target == "tvos_simulator" {
		// The Focus step taps only a text field there; the focus engine moves to a view the app focuses.
		app.Post(recorder.Focus)
		app.WaitUntil(5*time.Second, focused.Load)
	} else {
		app.Focus("id:recorder")
	}
	heard := func(want ...core.KeyEvent) {
		t.Helper()
		mu.Lock()
		defer mu.Unlock()
		if !slices.Equal(ups, want) {
			t.Fatalf("the view heard %+v come up, want %+v", ups, want)
		}
		ups = nil
	}
	held := func(want string) {
		t.Helper()
		if got := app.Find("id:held").Text; got != want {
			t.Fatalf("core.KeysDown reads %q, want %q", got, want)
		}
	}
	for _, key := range []string{"a", "left", "space", "shift"} {
		app.KeyDown(key)
		heard()
		held(key)
		app.KeyUp(key)
		heard(core.KeyEvent{Key: key})
		held("")
	}
	app.KeyDown("shift")
	app.KeyDown("a")
	held("a shift")
	app.KeyUp("a")
	app.KeyUp("shift")
	heard(core.KeyEvent{Key: "a", Modifiers: core.Modifiers{Shift: true}}, core.KeyEvent{Key: "shift"})
	held("")
	mu.Lock()
	downs = nil
	mu.Unlock()
	app.KeyDown("b")
	app.KeyDown("b")
	held("b")
	app.KeyUp("b")
	heard(core.KeyEvent{Key: "b"})
	held("")
	want := []core.KeyEvent{{Key: "b"}, {Key: "b", Repeat: true}}
	if target == "tvos_simulator" {
		// UIKit reports a held key once and never as a repeat.
		want = want[:1]
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(downs, want) {
		t.Fatalf("a key held again went down as %+v, want %+v", downs, want)
	}
}

// startHeldKeys starts a view that records the keys it hears go down and come up where a focused view hears them, and shows core.KeysDown; held checks what it shows.
func startHeldKeys(t *testing.T, record func(down bool, e core.KeyEvent)) (app *nsgotest.App, held func(want string)) {
	t.Helper()
	var recorder core.Ref
	var focused atomic.Bool
	app = nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.All(20)}, Children: []core.Node{
			core.View{
				TestID: "recorder", Ref: &recorder, Focusable: func() bool { return true }, OnFocus: func() { focused.Store(true) },
				OnKeyDown: func(e core.KeyEvent) bool { record(true, e); return false },
				OnKeyUp:   func(e core.KeyEvent) { record(false, e) },
				Style:     core.Style{Width: core.Pt(200), Height: core.Pt(40)},
			},
			core.Text{TestID: "held", ContentFunc: func() string { return strings.Join(core.KeysDown(cx), " ") }},
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}), nsgotest.Size(300, 120))
	switch nsgotest.Target() {
	case "ios", "ios_simulator":
		// No view hears keys go down there; core.KeysDown follows them.
	case "tvos_simulator":
		app.Post(recorder.Focus)
		app.WaitUntil(5*time.Second, focused.Load)
	default:
		app.Focus("id:recorder")
	}
	held = func(want string) {
		t.Helper()
		if got := app.Find("id:held").Text; got != want {
			t.Fatalf("core.KeysDown reads %q, want %q", got, want)
		}
	}
	return app, held
}

// The keys a test still holds as its app ends come up then, through the platform under -target, so the next app in the same process, a subtest's under nsgo test -target as headless, finds none held: a fresh press is no auto-repeat and carries no modifier the earlier one held.
func TestNoKeyStaysHeldIntoTheNextTest(t *testing.T) {
	var mu sync.Mutex
	var downs, ups []core.KeyEvent
	record := func(down bool, e core.KeyEvent) {
		mu.Lock()
		defer mu.Unlock()
		if down {
			downs = append(downs, logicalKey(e))
		} else {
			ups = append(ups, logicalKey(e))
		}
	}
	t.Run("holds", func(t *testing.T) {
		app, held := startHeldKeys(t, record)
		app.KeyDown("shift")
		app.KeyDown("a")
		held("a shift")
	})
	switch nsgotest.Target() {
	case "macos", "windows", "linux", "web", "tvos_simulator":
		// The view heard the keys it held come up as its test ended.
		want := []core.KeyEvent{{Key: "a", Modifiers: core.Modifiers{Shift: true}}, {Key: "shift"}}
		var got []core.KeyEvent
		for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(10 * time.Millisecond) {
			mu.Lock()
			got = slices.Clone(ups)
			mu.Unlock()
			if slices.Equal(got, want) || time.Now().After(deadline) {
				break
			}
		}
		if !slices.Equal(got, want) {
			t.Fatalf("the keys held as the test ended came up as %+v, want %+v", got, want)
		}
	}
	mu.Lock()
	downs = nil
	mu.Unlock()
	t.Run("presses again", func(t *testing.T) {
		app, held := startHeldKeys(t, record)
		held("")
		app.KeyDown("a")
		held("a")
		switch nsgotest.Target() {
		case "ios", "ios_simulator":
		default:
			mu.Lock()
			got := slices.Clone(downs)
			mu.Unlock()
			if want := []core.KeyEvent{{Key: "a"}}; !slices.Equal(got, want) {
				t.Fatalf("the key went down as %+v, want %+v", got, want)
			}
		}
		app.KeyUp("a")
		held("")
	})
}

// core.KeysDown follows a hardware keyboard's keys on iPhone and iPad, where no view hears them go down, as on every platform that reports keys (#1046).
func TestKeysDownFollowsAHardwareKeyboard(t *testing.T) {
	switch nsgotest.Target() {
	case "", "ios", "ios_simulator":
	default:
		t.Skip("TestAFocusedViewHearsItsKeysComeUp reads the keys held here")
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Padding: core.All(20)}, Children: []core.Node{
			core.Text{TestID: "held", ContentFunc: func() string { return strings.Join(core.KeysDown(cx), " ") }},
		}}
	}, nsgotest.Size(300, 120))
	held := func(want string) {
		t.Helper()
		if got := app.Find("id:held").Text; got != want {
			t.Fatalf("core.KeysDown reads %q, want %q", got, want)
		}
	}
	app.KeyDown("shift")
	held("shift")
	app.KeyDown("a")
	held("a shift")
	app.KeyUp("a")
	held("shift")
	app.KeyUp("shift")
	held("")
}

// Select moves the native field's selection under a target, so the edit after it acts on the range the test named rather than at the field's own caret.
func TestSelectMovesTheNativeSelection(t *testing.T) {
	if !slices.Contains([]string{"macos", "windows", "linux", "web"}, nsgotest.Target()) {
		t.Skip("headless, Type writes at the field's end, and the phones' test hosts move only the test's model")
	}
	var selection atomic.Value
	selection.Store(core.TextRange{})
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.Input{
			TestID: "field", DefaultValue: "hello world",
			Style:             core.Style{Width: core.Pt(240), Height: core.Pt(40)},
			OnSelectionChange: func(r core.TextRange) { selection.Store(r) },
		}
	}, nsgotest.Size(300, 120))
	app.Select("id:field", 6, 11)
	if got, model := selection.Load(), app.Find("id:field").Selection; got != (core.TextRange{Start: 6, End: 11}) || model != (core.TextRange{Start: 6, End: 11}) {
		t.Fatalf("the app heard the selection %v and the test's model has %v, want 6 to 11", got, model)
	}
	app.Type("id:field", "there")
	app.Select("id:field", 0, 5)
	app.Type("id:field", "goodbye")
	if got := app.Find("id:field").Text; got != "goodbye there" {
		t.Fatalf("typing did not replace the selected ranges: %q", got)
	}
}

func TestRefFocusPlacesTheNativeCaret(t *testing.T) {
	if nsgotest.Target() != "linux" {
		t.Skip("checks the GTK entry's programmatic focus")
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		var field core.Ref
		cx.OnKeyWith("mod+f", core.KeyOptions{When: core.KeyAlways}, field.Focus)
		return core.Input{TestID: "field", Ref: &field, DefaultValue: "hello",
			Style: core.Style{Width: core.Pt(240), Height: core.Pt(40)}}
	}, nsgotest.Size(300, 120))
	app.Key("mod+f")
	if !app.Find("id:field").Focused {
		t.Fatal("Ref.Focus did not focus the field")
	}
	app.Type("id:field", "!")
	if got := app.Find("id:field").Text; got != "hello!" {
		t.Fatalf("typing after Ref.Focus left %q, want hello!", got)
	}
}
