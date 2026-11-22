package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestViewPropsOwnTheirPointerBackedValues(t *testing.T) {
	ids := []core.NodeID{7, 9}
	actions := []core.AccessibilityAction{{Name: "inspect", Label: "Inspect"}}
	ripple := core.Ripple{Color: core.RGB(0x336699), Radius: 12}
	native := driver.ViewProps{Role: core.RoleButton, Label: "Open", LabelledBy: &ids,
		Actions: &actions, Ripple: &ripple, MultiSelectable: true, SelectionItem: true,
		Checked: true, Indeterminate: true, Pressed: true, Busy: true, Invalid: true,
		Expandable: true, Expanded: true, Value: core.AccessibilityValue{Min: 0, Max: 10, Now: 3},
		Appearance: core.AppearanceDark, HitSlop: core.All(8), TabAfter: 9}
	model := &App{t: t, views: map[driver.NodeID]*View{1: {ID: 1}}}
	model.apply(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemViewProps, ID: 1, ViewProps: &native}}})
	got := model.views[1].Props
	ids[0] = 90
	actions[0].Name = "removed"
	ripple.Radius = 90
	if (*got.LabelledBy)[0] != 7 || !got.HasAction("inspect") || got.Ripple.Radius != 12 {
		t.Fatalf("mounted props retained the sender's mutable values: %+v", got)
	}
	if got.Role != core.RoleButton || got.Label != "Open" || !got.MultiSelectable || !got.SelectionItem ||
		!got.Checked || !got.Indeterminate || !got.Pressed || !got.Busy || !got.Invalid || !got.Expandable || !got.Expanded ||
		got.Value.Now != 3 || got.Appearance != core.AppearanceDark || got.HitSlop != core.All(8) || got.TabAfter != 9 {
		t.Fatalf("mounted accessibility and interaction values changed: %+v", got)
	}
	zero := viewProps(driver.ViewProps{})
	if zero.LabelledBy != nil || zero.Actions != nil || zero.Ripple != nil {
		t.Fatal("absent props gained pointer values")
	}
	emptyIDs, emptyActions := []core.NodeID{}, []core.AccessibilityAction{}
	empty := viewProps(driver.ViewProps{LabelledBy: &emptyIDs, Actions: &emptyActions})
	if empty.LabelledBy == nil || *empty.LabelledBy == nil || empty.Actions == nil || *empty.Actions == nil {
		t.Fatal("explicitly empty lists lost their presence")
	}
}

func TestViewPropsKeepAccessibilityQueriesAndKeyboardStops(t *testing.T) {
	if Target() != "" {
		t.Skip("headless model policies")
	}
	var label *core.Signal[string]
	var disabled *core.Signal[bool]
	var action string
	presses := 0
	app := Start(t, func(cx *core.Scope) core.Node {
		label, disabled = core.NewSignal(cx, "Open"), core.NewSignal(cx, false)
		return core.View{TestID: "button", Role: core.RoleButton, LabelFunc: label.Get, Disabled: disabled.Get,
			AccessibilityDescription: "Choose a file", Hint: "Opens its dialog",
			AccessibilityActions:  []core.AccessibilityAction{{Name: "inspect", Label: "Inspect"}},
			OnAccessibilityAction: func(name string) { action = name },
			OnPress:               func(core.PressEvent) { presses++ }, Style: core.Style{Width: core.Pt(100), Height: core.Pt(30)}}
	}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))
	view := app.Find("role:button Open")
	if view.Props.HelpText() != "Choose a file. Opens its dialog" || len(view.Props.ActionList()) != 1 || !view.Props.FocusStop(view.Events) {
		t.Fatalf("recorded policies differ: %+v", view.Props)
	}
	app.AccessibilityAction("id:button", "inspect")
	if action != "inspect" {
		t.Fatalf("accessibility action = %q", action)
	}
	app.Focus("id:button")
	app.Key("Return")
	if presses != 1 {
		t.Fatalf("focused button pressed %d times", presses)
	}
	label.Set("Close")
	app.Settle()
	if app.Exists("role:button Open") {
		t.Fatal("query retained the old accessible name")
	}
	app.Find("role:button Close")
	disabled.Set(true)
	app.Settle()
	view = app.Find("role:button Close")
	if view.Props.FocusStop(view.Events) || app.Enabled("id:button") {
		t.Fatal("disabled props retained a keyboard stop or enabled query")
	}
}
