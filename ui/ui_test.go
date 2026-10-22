package ui_test

import (
	"testing"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/nsgotest"
	"github.com/oneo-me/nsgo/ui"
)

func TestComponentsFollowTheThemeAndAppearance(t *testing.T) {
	light, dark := ui.DefaultTheme().Light, ui.DefaultTheme().Dark
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		return ui.ThemeView{TestID: "root", Children: []core.Node{
			ui.Button{Label: "Save", TestID: "save"},
			ui.Button{Label: "Delete", Variant: ui.Danger, TestID: "delete",
				Style: core.Style{CornerRadius: 2}},
			ui.Switch{Checked: on.Get, OnChange: on.Set, Label: "Wi-Fi", TestID: "wifi"},
		}}
	})
	if got := app.Find("id:root").Style.Background; got != light.Background {
		t.Errorf("root background = %x, want the light background", got)
	}
	app.Find("text:Save")
	if got := app.Find("id:save").Style.Background; got != light.Primary {
		t.Errorf("button background = %x, want primary", got)
	}
	del := app.Find("id:delete").Style
	if del.Background != light.Danger || del.CornerRadius != 2 {
		t.Errorf("danger button = %x radius %v, want danger with the app's radius", del.Background, del.CornerRadius)
	}
	track := func() core.ViewStyle { return app.Find("id:wifi").Children[0].Style }
	if track().Background != light.Input {
		t.Errorf("off track = %x, want the input color", track().Background)
	}
	app.Press("id:wifi")
	if track().Background != light.Primary {
		t.Errorf("on track = %x, want primary", track().Background)
	}

	app.SetEnvironment(core.Environment{Dark: true})
	if got := app.Find("id:save").Style.Background; got != dark.Primary {
		t.Errorf("dark button background = %x, want the dark primary", got)
	}
	if !app.Find("id:wifi").Props.Checked {
		t.Error("switching appearance lost the switch's state")
	}
}

func TestThemeViewProvidesItsTheme(t *testing.T) {
	theme := ui.DefaultTheme()
	theme.Light.Primary = core.RGB(0x7c3aed)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return ui.ThemeView{Theme: func() ui.Theme { return theme }, Children: []core.Node{
			ui.Checkbox{Checked: func() bool { return true }, Label: "Agree", TestID: "agree"},
		}}
	})
	box := app.Find("id:agree").Children[0]
	if box.Style.Background != core.RGB(0x7c3aed) {
		t.Errorf("checked box = %x, want the provided primary", box.Style.Background)
	}
	app.Find("text:✓")
	app.Find("text:Agree")
}

func TestPopupsTakeTheThemeAndKeepBaseBehavior(t *testing.T) {
	var reasons []string
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return ui.Menu{Label: "File", Variant: ui.Secondary, TestID: "menu",
			OnOpenChange: func(_ bool, r base.Reason) { reasons = append(reasons, r.String()) },
			Children: []core.Node{
				ui.MenuItem{Label: "Open", TestID: "open"},
				ui.MenuSeparator{},
				ui.MenuItem{Label: "Quit"},
			}}
	})
	light := ui.DefaultTheme().Light
	if got := app.Find("id:menu").Style.Background; got != light.Secondary {
		t.Errorf("trigger background = %x, want secondary", got)
	}
	app.Press("text:File")
	if got := app.Find("id:open").Parent.Style.Background; got != light.Surface {
		t.Errorf("menu background = %x, want the surface", got)
	}
	app.Press("text:Open")
	if app.Exists("id:open") || len(reasons) != 2 || reasons[1] != "select" {
		t.Errorf("choosing an item: open %v, reasons %v", app.Exists("id:open"), reasons)
	}
}
