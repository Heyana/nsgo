package main

import (
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/ui"
)

// styledPage shows ui's components with the default theme, on its own
// background so the gallery's palette does not mix in.
func styledPage(cx *core.Scope, t *theme) core.Node {
	agree := core.NewSignal(cx, true)
	mixed := core.NewSignal(cx, true)
	wifi := core.NewSignal(cx, true)
	size := core.NewSignal(cx, "M")
	tab := core.NewSignal(cx, 0)
	dialog := core.NewSignal(cx, false)
	sheet := core.NewSignal(cx, false)

	var radios, tabs []core.Node
	for _, v := range []string{"S", "M", "L"} {
		radios = append(radios, ui.Radio{Label: v, Checked: func() bool { return size.Get() == v },
			OnChange: func(bool) { size.Set(v) }})
	}
	for i, n := range []string{"Day", "Week", "Month"} {
		tabs = append(tabs, ui.Tab{Label: n, Selected: func() bool { return tab.Get() == i }, OnPress: func() { tab.Set(i) }})
	}
	theme := ui.UseTheme(cx)
	panel := func(children ...core.Node) core.Node {
		return core.View{StyleFunc: func() core.Style {
			th := theme()
			return core.Style{Padding: core.All(16), Gap: 14, CornerRadius: th.RadiusLarge,
				BorderWidth: 1, BorderColor: th.Border}
		}, Children: children}
	}
	return ui.ThemeView{Style: core.Style{Padding: core.All(16), Gap: 16, CornerRadius: cardRadius}, Children: []core.Node{
		panel(
			flow(
				ui.Button{Label: "Primary"},
				ui.Button{Label: "Secondary", Variant: ui.Secondary},
				ui.Button{Label: "Danger", Variant: ui.Danger},
				ui.Button{Label: "Ghost", Variant: ui.Ghost},
				ui.Button{Label: "Disabled", Disabled: func() bool { return true }},
			),
		),
		panel(
			ui.Checkbox{Label: "Accept the terms", Checked: agree.Get, OnChange: agree.Set},
			ui.Checkbox{Label: "Select all", Checked: func() bool { return !mixed.Get() }, Indeterminate: mixed.Get,
				OnChange: func(bool) { mixed.Update(func(m bool) bool { return !m }) }},
			ui.Switch{Label: "Wi-Fi", Checked: wifi.Get, OnChange: wifi.Set},
			ui.RadioGroup{Label: "Size", Style: core.Style{Direction: core.Row, Gap: 16}, Children: radios},
			ui.Tabs{Label: "Range", Children: tabs},
		),
		panel(
			flow(
				ui.Menu{Label: "Menu", Variant: ui.Secondary, Children: []core.Node{
					ui.MenuItem{Label: "New file"},
					ui.MenuItem{Label: "Open…"},
					ui.MenuSeparator{},
					ui.Submenu{Label: "Recent", Children: []core.Node{
						ui.MenuItem{Label: "notes.md"}, ui.MenuItem{Label: "todo.txt"},
					}},
				}},
				ui.Popover{Label: "Popover", Variant: ui.Secondary, Children: []core.Node{
					core.Text{Content: "A popover on the theme's surface."},
				}},
				ui.Tooltip{Text: "A tooltip", Trigger: core.Text{Content: "Hover for a tooltip"}},
				ui.Button{Label: "Dialog", Variant: ui.Secondary, OnPress: func() { dialog.Set(true) }},
				ui.Button{Label: "Sheet", Variant: ui.Secondary, OnPress: func() { sheet.Set(true) }},
			),
		),
		ui.Dialog{Open: dialog.Get, OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) }, Label: "Dialog",
			Children: []core.Node{
				core.Text{Content: "Discard changes?", Style: core.Style{FontSize: 17, FontWeight: core.FontWeightSemibold}},
				core.Text{Content: "Your edits since the last save will be lost."},
				hstack(
					ui.Button{Label: "Cancel", Variant: ui.Secondary, OnPress: func() { dialog.Set(false) }},
					ui.Button{Label: "Discard", Variant: ui.Danger, OnPress: func() { dialog.Set(false) }},
				),
			}},
		ui.Sheet{Open: sheet.Get, OnOpenChange: func(open bool, _ base.Reason) { sheet.Set(open) }, Label: "Sheet",
			Children: []core.Node{
				core.Text{Content: "A sheet", Style: core.Style{FontSize: 17, FontWeight: core.FontWeightSemibold}},
				ui.Button{Label: "Close", OnPress: func() { sheet.Set(false) }},
			}},
	}}
}
