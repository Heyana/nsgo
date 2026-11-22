package base_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestCheckboxChangeEventKeepsThePress(t *testing.T) {
	type change struct {
		checked bool
		press   core.PressEvent
	}
	var changes []change
	var checked, mixed, disabled *core.Signal[bool]
	legacy, shortcut := 0, 0
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		checked, mixed, disabled = core.NewSignal(cx, false), core.NewSignal(cx, true), core.NewSignal(cx, false)
		cx.OnKeyWith("alt+enter", core.KeyOptions{When: core.KeyAlways}, func() { shortcut++ })
		return base.Checkbox{TestID: "check", Checked: checked.Get, Indeterminate: mixed.Get, Disabled: disabled.Get,
			Style:    core.Style{Width: core.Pt(180), Height: core.Pt(30)},
			OnChange: func(bool) { legacy++ },
			OnChangeEvent: func(next bool, e core.PressEvent) {
				changes = append(changes, change{next, e})
				checked.Set(next)
				mixed.Set(false)
			}, Children: []core.Node{core.Text{Content: "Files", TestID: "label"}}}
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	check := func(next bool, mods core.Modifiers, keyboard bool, clicks int) {
		t.Helper()
		if len(changes) != 1 {
			t.Fatalf("changes = %v, want one", changes)
		}
		got := changes[0]
		if got.checked != next || got.press.Modifiers != mods || got.press.Keyboard != keyboard || got.press.Clicks != clicks {
			t.Fatalf("change = %+v, want checked=%v modifiers=%v keyboard=%v clicks=%d", got, next, mods, keyboard, clicks)
		}
		if legacy != 0 {
			t.Fatalf("OnChange ran alongside OnChangeEvent: %d", legacy)
		}
		changes = nil
	}
	mods := core.Modifiers{Shift: true, Control: true, Alt: true, Platform: true}
	app.PressWith("id:label", mods)
	check(true, mods, false, 1) // Indeterminate asks for checked.
	app.Press("id:check")
	check(false, core.Modifiers{}, false, 2)
	app.Focus("id:check")
	for _, key := range []string{"shift+space", "ctrl+space", "alt+space", "mod+space", "meta+space", "enter"} {
		next := !app.Find("id:check").Props.Checked
		app.Key(key)
		wantMods := map[string]core.Modifiers{"shift+space": {Shift: true}, "ctrl+space": {Control: true},
			"alt+space": {Alt: true}, "mod+space": {Platform: true}, "meta+space": {Platform: true}}[key]
		check(next, wantMods, true, 1)
	}
	app.Key("alt+enter")
	if len(changes) != 0 || shortcut != 1 {
		t.Fatalf("shortcut changes=%v shortcut=%d", changes, shortcut)
	}
	disabled.Set(true)
	app.Settle()
	app.Report(nsgotest.Event{Kind: nsgotest.EventPress, ID: app.Find("id:check").ID, Modifiers: mods})
	if len(changes) != 0 || legacy != 0 {
		t.Fatal("disabled checkbox reported a change")
	}
}

func TestCheckboxChangeEventLeavesValueWithTheApp(t *testing.T) {
	var values []bool
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		return base.Checkbox{TestID: "check", OnChangeEvent: func(next bool, _ core.PressEvent) { values = append(values, next) },
			Style: core.Style{Width: core.Pt(40), Height: core.Pt(30)}}
	})
	app.Press("id:check")
	app.Press("id:check")
	if len(values) != 2 || !values[0] || !values[1] || app.Find("id:check").Props.Checked {
		t.Fatalf("values=%v, checked=%v", values, app.Find("id:check").Props.Checked)
	}
}

func TestCheckboxModifiedKeyboardActivation(t *testing.T) {
	for _, os := range []core.OS{core.OSMacOS, core.OSWindows, core.OSLinux} {
		t.Run(string(os), func(t *testing.T) {
			var got []core.PressEvent
			app := nsgotest.Start(t, func(*core.Scope) core.Node {
				return base.Checkbox{TestID: "check", OnChangeEvent: func(_ bool, e core.PressEvent) { got = append(got, e) },
					Style: core.Style{Width: core.Pt(40), Height: core.Pt(30)}}
			}, nsgotest.Environment(core.Environment{OS: os, Idiom: core.IdiomDesktop}))
			app.Focus("id:check")
			app.Key("mod+space")
			app.Key("meta+space")
			mod := core.Modifiers{Control: true}
			if os == core.OSMacOS {
				mod = core.Modifiers{Platform: true}
			}
			if len(got) != 2 || got[0].Modifiers != mod || got[1].Modifiers != (core.Modifiers{Platform: true}) || !got[0].Keyboard || !got[1].Keyboard {
				t.Fatalf("keyboard presses = %+v, want Mod=%+v then Meta", got, mod)
			}
		})
	}
}

func TestCheckboxAltActivationOnWindowsIsTheSystemsKey(t *testing.T) {
	for _, os := range []core.OS{core.OSMacOS, core.OSWindows, core.OSLinux, core.OSWeb} {
		t.Run(string(os), func(t *testing.T) {
			var got []core.PressEvent
			app := nsgotest.Start(t, func(*core.Scope) core.Node {
				return base.Checkbox{TestID: "check", OnChangeEvent: func(_ bool, e core.PressEvent) { got = append(got, e) },
					Style: core.Style{Width: core.Pt(40), Height: core.Pt(30)}}
			}, nsgotest.Environment(core.Environment{OS: os, Idiom: core.IdiomDesktop}))
			app.Focus("id:check")
			app.Key("alt+space")
			app.Key("alt+enter")
			want := 2
			if os == core.OSWindows {
				want = 0 // WM_SYSKEYDOWN: the window menu's, never the focused view's
			}
			if len(got) != want {
				t.Fatalf("alt activations = %+v, want %d", got, want)
			}
			for _, e := range got {
				if e.Modifiers != (core.Modifiers{Alt: true}) || !e.Keyboard {
					t.Fatalf("alt activation = %+v", e)
				}
			}
		})
	}
}

func TestCheckboxChangeEventThroughItsVisibleLabel(t *testing.T) {
	var got []core.PressEvent
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		box := &core.Ref{}
		return core.View{Children: []core.Node{
			base.Checkbox{TestID: "check", Ref: box, Label: "Files", OnChangeEvent: func(_ bool, e core.PressEvent) { got = append(got, e) },
				Style: core.Style{Width: core.Pt(40), Height: core.Pt(30)}},
			core.View{TestID: "label", LabelFor: box, Style: core.Style{Width: core.Pt(120), Height: core.Pt(30)},
				Children: []core.Node{core.Text{Content: "Files"}}},
		}}
	}, nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	app.PressWith("id:label", core.Modifiers{Shift: true})
	if len(got) != 1 || got[0].Modifiers != (core.Modifiers{Shift: true}) || got[0].Position != (core.Point{}) {
		t.Fatalf("label press = %+v, want one Shift press at the checkbox's origin", got)
	}
}
