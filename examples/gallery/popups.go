package main

import (
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

func popupsPage(cx *core.Scope, t *theme) core.Node {
	dialog := core.NewSignal(cx, false)
	sheet := core.NewSignal(cx, false)
	side := core.NewSignal(cx, false)
	picked := core.NewSignal(cx, "nothing")

	trigger := func(label string) core.Node { return core.Text{Content: label} }
	item := func(label string, children ...core.Node) core.Node {
		if len(children) == 0 {
			children = []core.Node{core.Text{Content: label}}
		}
		return base.MenuItem{Style: t.menuItem(), OnSelect: func() { picked.Set(label) }, Label: label, Children: children}
	}
	separator := base.MenuSeparator{Style: core.Style{Height: core.Pt(1), Background: t.line, Margin: core.Symmetric(4, 0)}}
	deleteItem := base.MenuItem{Style: t.menuItem(), Label: "Delete",
		Children: []core.Node{core.Text{Content: "Delete…", Style: core.Style{Color: t.danger}}},
		OnSelect: func() {
			base.ShowAlertDialog(cx, base.AlertDialog{Title: "Delete it?", Description: "This cannot be undone.",
				Actions: []core.AlertAction{{Label: "Cancel", Role: core.AlertCancel}, {Label: "Delete", Role: core.AlertDestructive}}},
				func(choice string) { picked.Set("alert: " + choice) })
		}}
	panel := func(title, body string, close func()) []core.Node {
		return []core.Node{
			core.Text{Content: title, Role: core.RoleHeader, Style: core.Style{FontSize: 18, FontWeight: core.FontWeightBold}},
			core.Text{Content: body, Style: core.Style{Color: t.muted}},
			t.primary("Close", close),
		}
	}

	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Tooltip and popover", "A tooltip opens while the pointer rests on its trigger; a popover on a press.",
			flow(
				base.Tooltip{Trigger: core.Text{Content: "Hover me (desktop)", Style: core.Style{Color: t.accent}},
					PopupStyle: t.popup().Merge(core.Style{Padding: core.Symmetric(4, 8)}),
					Children:   []core.Node{core.Text{Content: "A tooltip", Style: core.Style{FontSize: 13}}}},
				base.Popover{Style: t.secondary(), PopupStyle: t.popup().Merge(core.Style{Padding: core.All(12), MaxWidth: core.Pt(240)}),
					Offset: 6, Trigger: trigger("Popover"), Label: "Popover",
					Children: []core.Node{core.Text{Content: "A popover beside its trigger. Press outside or Escape to close it."}}},
			),
		),
		t.demo("Menus", "A menu opens beside its trigger, a context menu on a secondary click or a long press.",
			flow(
				base.Menu{Style: t.button(), PopupStyle: t.popup(), Offset: 6, Trigger: trigger("Menu"), Label: "Menu", Children: []core.Node{
					item("Copy"), item("Paste"),
					base.Submenu{Style: t.menuItem(), PopupStyle: t.popup(), Label: "Share",
						Trigger: core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween, Gap: 16},
							Children: []core.Node{core.Text{Content: "Share"}, core.Text{Content: "›", Style: core.Style{Color: t.muted}}}},
						Children: []core.Node{item("Mail"), item("Messages")}},
					separator,
					deleteItem,
				}},
				base.ContextMenu{PopupStyle: t.popup(), TestID: "context",
					Trigger: core.View{Style: core.Style{Padding: core.Symmetric(18, 24), CornerRadius: radius, BorderWidth: 1,
						BorderColor: t.line, Background: t.sunken},
						Children: []core.Node{core.Text{Content: "Right-click or long-press here", Style: t.caption()}}},
					Children: []core.Node{item("Inspect"), item("Rename"), separator, item("Archive")}},
			),
			t.live(func() string { return "Picked " + picked.Get() }),
		),
		t.demo("Dialogs and sheets", "Modal layers over the app; Escape, back or the backdrop closes them.",
			flow(
				t.primary("Dialog", func() { dialog.Set(true) }),
				t.plain("Sheet", func() { sheet.Set(true) }),
				t.plain("Side sheet", func() { side.Set(true) }),
				t.destructive("Alert dialog", func() {
					base.ShowAlertDialog(cx, base.AlertDialog{Title: "Discard changes?", Description: "A native alert, answered with the label chosen.",
						Actions: []core.AlertAction{{Label: "Keep", Role: core.AlertCancel}, {Label: "Discard", Role: core.AlertDestructive}}},
						func(choice string) { picked.Set("alert: " + choice) })
				}),
			),
		),
		base.Dialog{Open: dialog.Get, OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) }, BackdropStyle: t.backdrop(),
			PopupStyle: core.Style{Background: t.surface, Color: t.ink, CornerRadius: 14, Padding: core.All(20), Gap: 12,
				Width: core.Pct(80), MaxWidth: core.Pt(420)},
			Label: "Dialog", Children: panel("A dialog", "Escape, back or the backdrop closes it.", func() { dialog.Set(false) })},
		base.Sheet{Open: sheet.Get, OnOpenChange: func(open bool, _ base.Reason) { sheet.Set(open) }, BackdropStyle: t.backdrop(),
			PopupStyle: core.Style{Background: t.surface, Color: t.ink, Padding: core.All(20), Gap: 12, CornerRadius: 16, Height: core.Pct(40)},
			Label:      "Sheet", Children: []core.Node{base.SafeAreaView{Edges: base.Bottom, Style: core.Style{Gap: 12},
				Children: panel("A sheet from the bottom", "Sheets slide in from an edge of the app.", func() { sheet.Set(false) })}}},
		base.Sheet{Open: side.Get, OnOpenChange: func(open bool, _ base.Reason) { side.Set(open) }, BackdropStyle: t.backdrop(),
			Side: core.PlaceRight, Label: "Side sheet",
			PopupStyle: core.Style{Background: t.surface, Color: t.ink, Padding: core.All(20), Gap: 12, Width: core.Pt(300)},
			Children: []core.Node{base.SafeAreaView{Edges: base.Top | base.Bottom, Style: core.Style{Gap: 12},
				Children: panel("A side sheet", "Side picks the edge: this one sits on the right.", func() { side.Set(false) })}}},
	}}
}
