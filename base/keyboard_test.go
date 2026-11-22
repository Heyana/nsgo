package base_test

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// Native runs use hardware KeyEvents / UIKit HID, with focus requested through the same Ref command an app uses rather than a synthetic model focus event.
func TestMobileModifiedKeyboardActivation(t *testing.T) {
	if nsgotest.Target() != "" {
		if target := nsgotest.Target(); !strings.HasPrefix(target, "ios") && !strings.HasPrefix(target, "tvos") && target != "android" {
			t.Skip("mobile keyboard activation")
		}
		mobileKeyboardActivation(t)
		return
	}
	for _, env := range []core.Environment{{OS: core.OSAndroid}, {OS: core.OSIOS}, {OS: core.OSIOS, Idiom: core.IdiomTV}} {
		name := string(env.OS)
		if env.Idiom == core.IdiomTV {
			name += "/tv"
		}
		t.Run(name, func(t *testing.T) { mobileKeyboardActivation(t, nsgotest.Environment(env)) })
	}
}

func mobileKeyboardActivation(t *testing.T, options ...nsgotest.Option) {
	t.Helper()
	var button, check, input, blocked, plain core.Ref
	var phases []string
	var presses, changes []core.PressEvent
	var disable, cover *core.Signal[bool]
	shortcut, yielded, submitted := 0, 0, 0
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		disable, cover = core.NewSignal(cx, false), core.NewSignal(cx, false)
		cx.OnKeyWith("ctrl+shift+enter", core.KeyOptions{When: core.KeyAlways}, func() { shortcut++ })
		cx.OnKey("space", func() { yielded++ })
		row := core.Style{Width: core.Pt(180), Height: core.Pt(40)}
		phase := func(name string) func(core.PressEvent) {
			return func(e core.PressEvent) { phases = append(phases, name); presses = append(presses, e) }
		}
		return core.View{Children: []core.Node{
			core.View{Ref: &button, TestID: "activation", Style: row, Disabled: disable.Get,
				OnPressDown: phase("down"), OnPressUp: phase("up"), OnPress: phase("press")},
			base.Checkbox{Ref: &check, TestID: "check", Style: row,
				OnChangeEvent: func(_ bool, e core.PressEvent) { changes = append(changes, e) }},
			core.View{Ref: &blocked, TestID: "blocked", Style: row, NoFocus: true, OnPress: phase("blocked")},
			core.View{Ref: &plain, TestID: "plain", Style: row},
			core.Input{Ref: &input, TestID: "input", Style: row, DefaultValue: "x", OnSubmit: func(string) { submitted++ }},
			core.Show{When: cover.Get, Then: func(*core.Scope) core.Node { return core.Modal{} }},
		}}
	}, options...)
	focus := func(ref *core.Ref, id string) {
		t.Helper()
		app.Post(ref.Focus)
		app.WaitUntil(5*time.Second, func() bool { return app.Find("id:" + id).Focused })
	}
	focus(&button, "activation")
	if target := nsgotest.Target(); strings.HasPrefix(target, "ios") || strings.HasPrefix(target, "tvos") {
		// Warm UIKit's synthesized keyboard before its first modified chord.
		app.Key("shift")
	}
	for _, key := range []string{"space", "enter", "shift+space", "shift+enter", "ctrl+space", "ctrl+enter", "alt+space", "alt+enter", "meta+space", "meta+enter", "ctrl+alt+shift+meta+space", "ctrl+alt+shift+meta+enter"} {
		before := len(presses)
		app.Key(key)
		want := core.Modifiers{Shift: strings.Contains(key, "shift+"), Control: strings.Contains(key, "ctrl+"), Alt: strings.Contains(key, "alt+"), Platform: strings.Contains(key, "meta+")}
		if len(presses) != before+3 {
			t.Fatalf("%s: phases=%v, want one down/up/press", key, phases)
		}
		for i, phase := range []string{"down", "up", "press"} {
			e := presses[before+i]
			if phases[before+i] != phase || e.Modifiers != want || !e.Keyboard || e.Position != (core.Point{}) || e.Clicks != 1 {
				t.Fatalf("%s %s: event=%+v, want modifiers=%+v at the origin with one click", key, phase, e, want)
			}
		}
	}
	if yielded != 0 {
		t.Fatalf("KeyAuto space ran %d times instead of yielding to the pressable", yielded)
	}
	before := len(presses)
	app.Key("ctrl+shift+enter")
	if shortcut != 1 || len(presses) != before {
		t.Fatalf("keymap precedence: shortcut=%d, presses=%d, want 1 and %d", shortcut, len(presses), before)
	}
	focus(&check, "check")
	app.Key("shift+space")
	if len(changes) != 1 || !changes[0].Keyboard || changes[0].Modifiers != (core.Modifiers{Shift: true}) {
		t.Fatalf("checkbox change lost keyboard modifiers: %+v", changes)
	}
	if target := nsgotest.Target(); target == "android" || strings.HasPrefix(target, "ios") {
		app.Press("id:activation")
		if !app.Find("id:check").Focused {
			t.Fatal("a touch moved keyboard focus from another pressable")
		}
	}
	app.Post(blocked.Focus)
	app.Post(plain.Focus)
	app.Settle()
	if !app.Find("id:check").Focused {
		t.Fatal("NoFocus or a plain view took the keyboard focus")
	}
	focus(&button, "activation")
	app.Post(func() { disable.Set(true) })
	app.Settle()
	before = len(presses)
	app.Key("enter")
	if len(presses) != before {
		t.Fatal("a disabled pressable activated")
	}
	app.Post(func() { disable.Set(false); cover.Set(true) })
	app.Settle()
	app.Post(button.Focus)
	app.Settle()
	app.Key("enter")
	if app.Find("id:activation").Focused || len(presses) != before {
		t.Fatal("a pressable behind a modal took focus or activated")
	}
	app.Post(func() { cover.Set(false) })
	app.Settle()
	focus(&input, "input")
	app.Key("space")
	app.Key("enter")
	if got := app.Find("id:input").Text; got != "x " || submitted != 1 || len(presses) != before {
		t.Fatalf("native editing: text=%q, submits=%d, press phases=%d; want x-space, 1, %d", got, submitted, len(presses), before)
	}
}
