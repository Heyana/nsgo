package ui

import (
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

func TestControlTextFuncsKeepFocus(t *testing.T) {
	headless(t)
	var scope *core.Scope
	var busy *core.Signal[bool]
	var text, override *core.Signal[string]
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		scope, busy = cx, core.NewSignal(cx, false)
		text, override = core.NewSignal(cx, "Copy"), core.NewSignal(cx, "")
		titles := func() string {
			if cx.Env().Locale() == "fr" {
				return "Titres"
			}
			return "Titles"
		}
		return core.View{Children: []core.Node{
			Button{TextFunc: text.Get, LabelFunc: override.Get, Busy: busy.Get, BusyLabel: "Copying", TestID: "copy",
				OnPress: func() { text.Set("Copied") }},
			Checkbox{TextFunc: titles, HintFunc: titles, TestID: "check"},
			base.Button{HintFunc: titles, TestID: "hinted"},
			Progress{LabelFunc: titles, TestID: "progress"},
			Popover{Trigger: Button{TextFunc: titles}, TestID: "popover"},
			Tabs[int]{LabelFunc: titles, TestID: "tabs", Options: []Option[int]{{LabelFunc: titles, TestID: "tab"}}},
			Select[int]{Label: "Options", TestID: "select", Value: func() int { return 1 },
				Options: []Option[int]{{Value: 1, LabelFunc: titles}}},
		}}
	})
	app.Focus("copy")
	button := app.Find("copy")
	buttonID, textID := button.ID, button.Children[0].Children[0].Children[0].Children[0].ID
	app.Key("space")
	button = app.Find("copy")
	if button.ID != buttonID || button.Props.Label != "Copied" || !button.Focused {
		t.Fatalf("button after activation = %+v, want its original focused view labeled Copied", button.Props)
	}
	restingText := button.Children[0].Children[0].Children[0].Children[0]
	if restingText.ID != textID || restingText.Text != "Copied" {
		t.Fatalf("resting text = %q on %v, want Copied on %v", restingText.Text, restingText.ID, textID)
	}
	app.Post(func() { busy.Set(true); override.Set("Copy to clipboard") })
	app.Settle()
	if app.Find("copy").Props.Label != "Copying" {
		t.Fatal("busy label did not override the resting label")
	}
	app.Post(func() { busy.Set(false) })
	app.Settle()
	if button = app.Find("copy"); button.ID != buttonID || button.Props.Label != "Copy to clipboard" || !button.Focused {
		t.Fatalf("button after busy state = %+v, want its original focused view with explicit label", button.Props)
	}

	app.Focus("tab")
	tab, list := app.Find("tab"), app.Find("tabs")
	tabID, listID, indicatorID := tab.ID, list.ID, list.Children[0].ID
	checkID, popoverID := app.Find("check").ID, app.Find("popover").ID
	app.Post(func() { scope.SetLocale("fr") })
	app.Settle()
	if tab = app.Find("tab"); tab.ID != tabID || tab.Props.Label != "Titres" || tab.Children[0].Text != "Titres" || !tab.Props.Selected || !tab.Focused {
		t.Fatalf("tab after translation = %+v, want its original focused and selected view", tab.Props)
	}
	if list = app.Find("tabs"); list.ID != listID || list.Props.Label != "Titres" || list.Children[0].ID != indicatorID {
		t.Fatal("translating the tab remounted the list or its indicator")
	}
	check := app.Find("check")
	if check.ID != checkID || check.Props.Label != "Titres" || check.Props.Hint != "Titres" || check.Children[1].Text != "Titres" {
		t.Fatalf("checkbox after translation = %+v", check.Props)
	}
	if pop := app.Find("popover"); pop.ID != popoverID || pop.Props.Label != "Titres" {
		t.Fatalf("popover after translation = %+v", pop.Props)
	}
	if app.Find("progress").Props.Label != "Titres" || app.FindMounted("id:hinted").Props.Hint != "Titres" {
		t.Fatal("the progress label or base button hint did not follow the locale")
	}
	app.Press("select")
	if row := app.Find("role:option Titres"); row.Props.Label != "Titres" {
		t.Fatalf("option after translation = %+v", row.Props)
	}
	app.Key("Escape")
	app.Focus("check")
	app.Post(func() { scope.SetLocale("en") })
	app.Settle()
	if check = app.Find("check"); check.ID != checkID || !check.Focused || check.Props.Label != "Titles" {
		t.Fatalf("locale change lost the focused checkbox or its label: %+v", check.Props)
	}
}

func TestSwitchLabelFuncOnNativeAndDrawnViews(t *testing.T) {
	headless(t)
	for _, os := range []core.OS{core.OSMacOS, core.OSWindows} {
		var label *core.Signal[string]
		app, _ := start(t, core.Environment{OS: os, Idiom: core.IdiomDesktop}, func(cx *core.Scope) core.Node {
			label = core.NewSignal(cx, "Wi-Fi")
			return Switch{Variant: SwitchSystem, Checked: func() bool { return false }, LabelFunc: label.Get, TestID: "radio"}
		})
		before := app.FindMounted("id:radio").ID
		app.Post(func() { label.Set("Réseau sans fil") })
		app.Settle()
		if after := app.FindMounted("id:radio"); after.ID != before || after.Props.Label != "Réseau sans fil" {
			t.Fatalf("%v: switch label = %q on %v, want new label on %v", os, after.Props.Label, after.ID, before)
		}
	}
}

func TestBaseSliderStartLabelFuncFallsBack(t *testing.T) {
	headless(t)
	var startLabel *core.Signal[string]
	app, _ := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		startLabel = core.NewSignal(cx, "Start")
		return base.Slider{Value: func() base.SliderValue { return base.RangeValue(20, 60) }, Label: "Range", StartLabelFunc: startLabel.Get,
			StartThumbTestID: "start", ThumbTestID: "end",
			Style:      core.Style{Width: core.Pt(210), Height: core.Pt(40)},
			TrackStyle: core.Style{Height: core.Pct(100)}, ThumbStyle: core.Style{Width: core.Pt(16), Height: core.Pt(16)}}
	})
	app.Focus("start")
	before := app.Find("start").ID
	if initial := app.Find("start").Props.Label; initial != "Start" {
		t.Fatalf("start name = %q, want Start", initial)
	}
	app.Post(func() { startLabel.Set("") })
	app.Settle()
	if after := app.Find("start"); after.ID != before || !after.Focused || after.Props.Label != "Range" {
		t.Fatalf("start after an empty label = %+v, focused %v", after.Props, after.Focused)
	}
}
