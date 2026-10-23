package main

import (
	"fmt"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"
)

// The pages of the ui package's components show them in the theme the
// gallery's ThemeView provides: NSGo's default.

func buttonsPage(cx *core.Scope, t *theme) core.Node {
	agree := core.NewSignal(cx, true)
	mixed := core.NewSignal(cx, true)
	wifi := core.NewSignal(cx, true)
	size := core.NewSignal(cx, "M")
	tab := core.NewSignal(cx, 0)
	presses := core.NewSignal(cx, 0)
	press := func() { presses.Update(func(n int) int { return n + 1 }) }
	var radios, tabs []core.Node
	for _, v := range []string{"S", "M", "L"} {
		radios = append(radios, ui.Radio{Label: v, Checked: func() bool { return size.Get() == v },
			OnChange: func(bool) { size.Set(v) }})
	}
	for i, n := range []string{"Day", "Week", "Month"} {
		tabs = append(tabs, ui.Tab{Label: n, Selected: func() bool { return tab.Get() == i }, OnPress: func() { tab.Set(i) }})
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Button", "ui.Button in each Variant; Style adjusts one without leaving the theme.",
			flow(
				ui.Button{Label: "Primary", OnPress: press},
				ui.Button{Label: "Secondary", Variant: ui.Secondary, OnPress: press},
				ui.Button{Label: "Danger", Variant: ui.Danger, OnPress: press},
				ui.Button{Label: "Ghost", Variant: ui.Ghost, OnPress: press},
				ui.Button{Label: "Disabled", Disabled: always},
			),
			flow(
				ui.Button{Label: "Download", OnPress: press, Children: []core.Node{
					core.Icon{SVG: lucide.Download(), Style: core.Style{Width: core.Pt(16), Height: core.Pt(16)}},
					core.Text{Content: "Download"},
				}},
				ui.Button{Label: "Pill", Variant: ui.Secondary, OnPress: press, Style: core.Style{CornerRadius: 20, Padding: core.Symmetric(6, 18)}},
			),
			t.live(func() string { return fmt.Sprintf("Pressed %d times.", presses.Get()) }),
		),
		t.demo("Toggles", "ui.Checkbox, ui.Switch and ui.Radio draw their behavior with the theme.",
			ui.Checkbox{Label: "Accept the terms", Checked: agree.Get, OnChange: agree.Set},
			ui.Checkbox{Label: "Select all", Checked: func() bool { return !mixed.Get() }, Indeterminate: mixed.Get,
				OnChange: func(bool) { mixed.Update(func(m bool) bool { return !m }) }},
			ui.Switch{Label: "Wi-Fi", Checked: wifi.Get, OnChange: wifi.Set},
			ui.RadioGroup{Label: "Size", Style: core.Style{Direction: core.Row, Gap: 16}, Children: radios},
		),
		t.demo("Tabs", "ui.Tabs on a muted track, the selected tab raised.",
			ui.Tabs{Label: "Range", Children: tabs},
		),
	}}
}

func overlaysPage(cx *core.Scope, t *theme) core.Node {
	dialog := core.NewSignal(cx, false)
	sheet := core.NewSignal(cx, false)
	picked := core.NewSignal(cx, "nothing yet")
	pick := func(label string) func() { return func() { picked.Set(label) } }
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Menus", "ui.Menu on the theme's raised surface, with checkable items and submenus.",
			flow(
				ui.Menu{Label: "Menu", Variant: ui.Secondary, Children: []core.Node{
					ui.MenuItem{Label: "New file", OnSelect: pick("New file")},
					ui.MenuItem{Label: "Open…", OnSelect: pick("Open…")},
					ui.MenuSeparator{},
					ui.Submenu{Label: "Recent", Children: []core.Node{
						ui.MenuItem{Label: "notes.md", OnSelect: pick("notes.md")},
						ui.MenuItem{Label: "todo.txt", OnSelect: pick("todo.txt")},
					}},
				}},
				ui.IconButton{Icon: lucide.Ellipsis(), Label: "More", Menu: []core.Node{
					ui.MenuItem{Label: "Pinned", Checked: func() bool { return picked.Get() == "Pinned" }, OnSelect: pick("Pinned")},
					ui.MenuItem{Label: "Archive", OnSelect: pick("Archive")},
				}},
			),
			t.live(func() string { return "Picked " + picked.Get() + "." }),
		),
		t.demo("Popover and tooltip", "ui.Popover opens beside its trigger; ui.Tooltip shows while the pointer rests on it.",
			flow(
				ui.Popover{Label: "Popover", Variant: ui.Secondary, Children: []core.Node{
					core.Text{Content: "A popover on the theme's surface."},
				}},
				ui.Tooltip{Text: "A tooltip", Trigger: core.Text{Content: "Hover for a tooltip"}},
			),
		),
		t.demo("Dialog and sheet", "ui.Dialog and ui.Sheet over the theme's overlay; Escape or a press outside closes them.",
			flow(
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

// themePage shows a sample of components under a ThemeView of the chosen
// theme: NSGo's default or the app's own, here cool neutrals with a teal
// primary.
func themePage(cx *core.Scope, t *theme) core.Node {
	teal := core.NewSignal(cx, false)
	on := core.NewSignal(cx, true)
	level := core.NewSignal(cx, 60.0)
	pick := func() ui.Theme {
		if teal.Get() {
			return tealTheme()
		}
		return ui.DefaultTheme()
	}
	theme := ui.UseTheme(cx)
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Theme", "ui.ThemeView provides a Theme: two palettes, radii and a font size. Every component below it follows.",
			ui.Tabs{Label: "Theme", Children: []core.Node{
				ui.Tab{Label: "NSGo", TestID: "theme-default", Selected: func() bool { return !teal.Get() }, OnPress: func() { teal.Set(false) }},
				ui.Tab{Label: "Teal", TestID: "theme-teal", Selected: teal.Get, OnPress: func() { teal.Set(true) }},
			}},
			ui.ThemeView{Theme: pick, TestID: "theme-sample",
				Style: core.Style{Padding: core.All(20), Gap: 16, CornerRadius: cardRadius, BorderWidth: 1, BorderColor: theme().Border},
				Children: []core.Node{
					flow(
						ui.Button{Label: "Primary"},
						ui.Button{Label: "Secondary", Variant: ui.Secondary},
						ui.Button{Label: "Danger", Variant: ui.Danger},
						ui.Badge{Text: "New", Tone: ui.ToneAccent},
						ui.Badge{Text: "Passed", Tone: ui.ToneSuccess, Icon: lucide.CircleCheck()},
					),
					ui.Input{Placeholder: "Search", Icon: lucide.Search(), Label: "Search", Style: core.Style{MaxWidth: core.Pt(360)}},
					hstack(
						ui.Switch{Label: "Notifications", Checked: on.Get, OnChange: on.Set},
						ui.Slider{Value: level.Get, OnChange: level.Set, Max: 100, Label: "Level", Style: core.Style{Width: core.Pt(180)}},
					),
					ui.Progress{Value: func() float64 { return level.Get() / 100 }, Label: "Level"},
				}},
		),
	}}
}

// sidebarPage is a mail app's navigation: ui.Sidebar's sections and
// items, with counts after them and an action on a section's heading.
func sidebarPage(cx *core.Scope, t *theme) core.Node {
	at := core.NewSignal(cx, "Inbox")
	labels := core.NewSignalFunc(cx, []string{"Work", "Family"}, nil)
	item := func(label string, icon *core.SVG, count string) core.Node {
		var trailing []core.Node
		if count != "" {
			trailing = append(trailing, core.Text{Content: count, Style: core.Style{FontSize: 12, Color: t.muted}})
		}
		return ui.SidebarItem{Label: label, Icon: icon, Trailing: trailing, TestID: "mail-" + label,
			Selected: func() bool { return at.Get() == label }, OnPress: func() { at.Set(label) }}
	}
	return t.demo("Sidebar", "ui.Sidebar's sections and items; the app owns the selection. Items take trailing content, headings actions.",
		core.View{Style: core.Style{Direction: core.Row, Height: core.Pt(300), CornerRadius: radius, BorderWidth: 1, BorderColor: t.line,
			Overflow: core.Hidden}, Children: []core.Node{
			core.Scroll{Style: core.Style{Width: core.Pt(210), Padding: core.Edges{Top: 12}, Background: t.sidebar,
				Borders: core.Edges{Right: 1}, BorderColors: core.EdgeColors{Right: t.line}}, Children: []core.Node{
				ui.Sidebar{Label: "Mail", Children: []core.Node{
					ui.SidebarSection{Title: "Mail", Children: []core.Node{
						item("Inbox", lucide.Inbox(), "12"),
						item("Starred", lucide.Star(), ""),
						item("Sent", lucide.Send(), ""),
						item("Drafts", lucide.FileText(), "2"),
					}},
					ui.SidebarSection{Title: "Labels",
						Actions: []core.Node{ui.IconButton{Icon: lucide.Plus(), Label: "Add a label", Size: 14, TestID: "add-label",
							OnPress: func() {
								labels.Update(func(l []string) []string { return append(l, fmt.Sprintf("Label %d", len(l)+1)) })
							}}},
						Children: []core.Node{core.Each(labels.Get, func(l string) string { return l },
							func(cx *core.Scope, l func() string, _ func() int) core.Node {
								return item(core.Untracked(cx, l), lucide.Tag(), "")
							})}},
				}},
			}},
			core.View{Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 6},
				Children: []core.Node{
					core.Text{ContentFunc: at.Get, Style: core.Style{FontSize: 20, FontWeight: core.FontWeightSemibold}},
					t.note("The selected item shows here."),
				}},
		}},
	)
}
