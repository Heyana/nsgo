package ui_test

import (
	"slices"
	"testing"
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/nsgotest"
	"github.com/oneo-me/nsgo/ui"
)

func TestThemeViewChoosesTheAppearance(t *testing.T) {
	theme := ui.DefaultTheme()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		dark := core.NewSignal(cx, true)
		return ui.ThemeView{Dark: dark.Get, TestID: "root", Children: []core.Node{
			ui.Button{Label: "Light", OnPress: func(core.PressEvent) { dark.Set(false) }, TestID: "light"},
		}}
	})
	if got := app.Find("id:root").Style.Background; got != theme.Dark.Background {
		t.Fatalf("background = %x, want the dark one the view chose over the light system", got)
	}
	app.Press("id:light")
	if got := app.Find("id:light").Style.Background; got != theme.Light.Primary {
		t.Fatalf("button = %x, want the light primary", got)
	}
}

func TestSelectChoosesAnOption(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		size := core.NewSignal(cx, "")
		return ui.ThemeView{Children: []core.Node{
			ui.Select{Value: size.Get, OnChange: size.Set, Placeholder: "Pick a size", Label: "Size", TestID: "size",
				Options: []ui.SelectOption{{Value: "s", Label: "Small"}, {Value: "l", Label: "Large"}}},
		}}
	})
	app.Find("text:Pick a size")
	app.Press("id:size")
	app.Press("label:Large")
	if app.Exists("label:Small") {
		t.Fatal("choosing did not close the menu")
	}
	app.Find("id:size >> text:Large")
	app.Press("id:size")
	if opacity := func(label string) float32 {
		return app.Find("label:" + label).Children[0].Style.Opacity
	}; opacity("Large") != 1 || opacity("Small") != 0 {
		t.Fatal("the chosen option is not the one checked")
	}
}

func TestSliderFollowsADrag(t *testing.T) {
	var got []float64
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		v := core.NewSignal(cx, 20.0)
		return ui.ThemeView{Style: core.Style{Padding: core.All(10)}, Children: []core.Node{
			ui.Slider{Value: v.Get, Min: 0, Max: 100, Step: 5, Label: "Volume", TestID: "volume",
				Style:    core.Style{Width: core.Pt(200)},
				OnChange: func(x float64) { got = append(got, x); v.Set(x) }},
		}}
	}, nsgotest.Size(400, 200))
	app.Pan("id:volume", core.Point{X: 3}, core.Point{X: 51}, core.Point{X: 500})
	if want := []float64{20, 45, 100}; !slices.Equal(got, want) && !slices.Equal(got, want[1:]) {
		t.Fatalf("values = %v, want the start, snapped to 5 and clamped: %v", got, want)
	}
	if fill := app.Find("id:volume").Children[0].Children[0].Frame.Size.Width; fill != 200 {
		t.Fatalf("fill = %v wide, want the whole track at the maximum", fill)
	}
}

func TestToastShowsForItsDuration(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return ui.ThemeView{Children: []core.Node{ui.ToastView{Children: []core.Node{core.Func(func(cx *core.Scope) core.Node {
			toast := ui.UseToast(cx)
			return ui.Button{Label: "Save", OnPress: func(core.PressEvent) { toast(ui.Toast{Title: "Saved", Duration: time.Second}) }}
		})}}}}
	})
	app.Press("role:button Save")
	app.Find("id:toast >> text:Saved")
	app.Advance(time.Second)
	app.Advance(200 * time.Millisecond)
	if app.Exists("id:toast") {
		t.Fatal("the toast outlived its duration")
	}
	app.Press("role:button Save")
	app.Press("id:toast")
	app.Advance(200 * time.Millisecond)
	if app.Exists("id:toast") {
		t.Fatal("a press did not dismiss the toast")
	}
}

func settingsCategories(dark *core.Signal[bool]) []ui.SettingsCategory {
	return []ui.SettingsCategory{
		{ID: "general", Title: "General", Section: "App", Groups: []ui.SettingsGroup{{ID: "look", Items: []ui.SettingsItem{
			{ID: "dark", Title: "Dark mode", Description: "Use dark colors", Keywords: []string{"theme"},
				Control: ui.Switch{Checked: dark.Get, OnChange: dark.Set, Label: "Dark mode"}},
		}}}},
		{ID: "editor", Title: "Editor", Section: "App", Groups: []ui.SettingsGroup{{ID: "text", Title: "Text", Items: []ui.SettingsItem{
			{ID: "wrap", Title: "Word wrap", Description: "Wrap long lines"},
			{ID: "numbers", Title: "Line numbers", Keywords: []string{"gutter"}},
		}}}},
	}
}

func TestSettingsNavigateAndSearch(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		dark := core.NewSignal(cx, false)
		return ui.ThemeView{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			ui.Settings{Categories: settingsCategories(dark), OnClose: func() {}},
		}}
	}, nsgotest.Size(800, 600))
	app.Find("id:setting-dark")
	if app.Exists("id:setting-wrap") {
		t.Fatal("another category's items show")
	}
	app.Press("role:switch Dark mode")
	if !app.Find("role:switch Dark mode").Props.Checked {
		t.Fatal("the item's control does not work")
	}
	app.Press("id:settings-category-editor")
	app.Find("id:setting-wrap")
	if !app.Find("id:settings-category-editor").Props.Selected {
		t.Fatal("the sidebar does not show the chosen category")
	}

	app.Type("id:settings-search", "gutter")
	app.Find("role:header Search results")
	if !app.Exists("id:setting-numbers") || app.Exists("id:setting-wrap") {
		t.Fatal("search did not find the item by its keyword alone")
	}
	app.Type("id:settings-search", "nothing like it")
	app.Find("text:No matching settings")
	app.Press("id:settings-category-general")
	if got := app.Find("id:settings-search").Text; got != "" {
		t.Fatalf("choosing a category kept the search %q", got)
	}

	app.Resize(420, 600)
	if app.Exists("id:settings-category-general") {
		t.Fatal("a narrow surface keeps the sidebar")
	}
	app.Press("id:settings-category")
	app.Press("label:Editor")
	app.Find("id:setting-wrap")
}

func TestSearchSettingsMatchesEveryTerm(t *testing.T) {
	cats := settingsCategories(nil)
	if got := ui.SearchSettings(cats, "EDITOR wrap"); len(got) != 1 || got[0] != (ui.SettingsResult{Category: 1, Group: 0, Item: 0}) {
		t.Fatalf("results = %v, want the one item every term finds", got)
	}
	if got := ui.SearchSettings(cats, ""); len(got) != 3 {
		t.Fatalf("an empty query found %d items, want all 3", len(got))
	}
}

func TestCommandPaletteFiltersMovesAndRuns(t *testing.T) {
	var ran []string
	var reasons []base.Reason
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		open := core.NewSignal(cx, true)
		return ui.ThemeView{Style: core.Style{FlexGrow: 1}, Children: []core.Node{
			ui.Button{Label: "Open", OnPress: func(core.PressEvent) { open.Set(true) }},
			ui.CommandPalette{Open: open.Get,
				OnOpenChange: func(o bool, r base.Reason) { reasons = append(reasons, r); open.Set(o) },
				OnSelect:     func(id string) { ran = append(ran, id) },
				Commands: func() []ui.Command {
					return []ui.Command{
						{ID: "new", Title: "New file", Group: "File"},
						{ID: "open", Title: "Open file", Group: "File", Disabled: true},
						{ID: "save", Title: "Save file", Group: "File", Keywords: []string{"write"}},
						{ID: "theme", Title: "Toggle theme", Group: "View"},
					}
				}},
		}}
	}, nsgotest.Size(800, 600))
	if n := len(app.Query("role:header File")); n != 1 {
		t.Fatalf("%d File headings, want one over the group", n)
	}
	app.Find("role:header View")
	if !app.Find("id:command-new").Props.Selected {
		t.Fatal("the first command is not active")
	}
	app.Key("down")
	if !app.Find("id:command-save").Props.Selected {
		t.Fatal("down did not skip the disabled command")
	}
	app.Submit("id:command-search")
	if !slices.Equal(ran, []string{"save"}) || reasons[len(reasons)-1] != base.ReasonSelect {
		t.Fatalf("ran %v closing with %v, want save closing for the selection", ran, reasons)
	}
	if app.Exists("id:command-search") {
		t.Fatal("running a command did not close the palette")
	}

	app.Press("role:button Open")
	if got := app.Find("id:command-search").Text; got != "" {
		t.Fatalf("reopening kept the search %q", got)
	}
	app.Type("id:command-search", "write")
	if app.Exists("id:command-new") || !app.Find("id:command-save").Props.Selected {
		t.Fatal("the search did not narrow to the keyword's command")
	}
	app.Type("id:command-search", "zzz")
	app.Find("text:No matching commands")
	app.Back()
	if app.Exists("id:command-search") {
		t.Fatal("Escape did not close the palette")
	}
}

func TestFilterCommandsPutsTitleMatchesFirst(t *testing.T) {
	cmds := []ui.Command{{ID: "a", Title: "Open recent", Description: "Theme files"}, {ID: "b", Title: "Theme"}}
	got := ui.FilterCommands(cmds, "the")
	if len(got) != 2 || got[0].ID != "b" {
		t.Fatalf("order = %v, want the title match first", got)
	}
}

func TestSidebarItemShowsSelection(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		at := core.NewSignal(cx, "inbox")
		item := func(id string) core.Node {
			return ui.SidebarItem{Label: id, TestID: id, Selected: func() bool { return at.Get() == id }, OnPress: func(core.PressEvent) { at.Set(id) }}
		}
		return ui.ThemeView{Children: []core.Node{ui.Sidebar{Children: []core.Node{
			ui.SidebarSection{Title: "Mail", Children: []core.Node{item("inbox"), item("sent")}},
		}}}}
	})
	app.Find("role:header Mail")
	app.Press("id:sent")
	if !app.Find("id:sent").Props.Selected || app.Find("id:inbox").Props.Selected {
		t.Fatal("the selection did not follow the press")
	}
	if app.Find("id:sent").Style.Background == app.Find("id:inbox").Style.Background {
		t.Fatal("the selected item has no face of its own")
	}
}

func TestInputWithIconShowsFocus(t *testing.T) {
	theme := ui.DefaultTheme().Light
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		q := core.NewSignal(cx, "")
		ref := &core.Ref{}
		return ui.ThemeView{Children: []core.Node{
			ui.Input{Value: q.Get, OnChange: q.Set, Icon: lucide.Search(), Ref: ref, Label: "Find", TestID: "find"},
			ui.Button{Label: "Focus", OnPress: func(core.PressEvent) { ref.Focus() }},
		}}
	})
	frame := func() core.ViewStyle { return app.Find("id:find").Parent.Style }
	if frame().BorderColor != theme.Border {
		t.Fatalf("border = %x, want the border color at rest", frame().BorderColor)
	}
	app.Press("role:button Focus")
	if frame().BorderColor != theme.Ring {
		t.Fatalf("focused border = %x, want the ring color", frame().BorderColor)
	}
}

func TestControlsReportTheNextValue(t *testing.T) {
	theme := ui.DefaultTheme().Light
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		mixed := core.NewSignal(cx, true)
		tab := core.NewSignal(cx, 0)
		return ui.ThemeView{Children: []core.Node{
			ui.Checkbox{Checked: on.Get, Indeterminate: mixed.Get, Label: "Select all", TestID: "all",
				OnChange: func(v bool) { on.Set(v); mixed.Set(false) }},
			ui.Switch{Checked: on.Get, OnChange: on.Set, Label: "Wi-Fi", TestID: "wifi"},
			ui.Tabs{Children: []core.Node{
				ui.Tab{Selected: func() bool { return tab.Get() == 0 }, OnPress: func(core.PressEvent) { tab.Set(0) }, Label: "One", TestID: "one"},
				ui.Tab{Selected: func() bool { return tab.Get() == 1 }, OnPress: func(core.PressEvent) { tab.Set(1) }, Label: "Two", TestID: "two"},
			}},
		}}
	})
	all := app.Find("role:checkbox Select all")
	if !all.Props.Indeterminate || all.Props.Checked {
		t.Fatalf("checkbox starts %+v, want mixed", all.Props)
	}
	app.Press("id:all")
	if all := app.Find("id:all"); !all.Props.Checked || all.Props.Indeterminate {
		t.Fatalf("after press %+v, want checked", all.Props)
	}
	if indicator := app.Find("id:all").Children[0]; indicator.Style.Background != theme.Primary {
		t.Errorf("indicator background = %x, want the primary", indicator.Style.Background)
	}
	if !app.Find("role:switch Wi-Fi").Props.Checked {
		t.Error("the switch does not follow the shared value")
	}
	app.Press("id:wifi")
	if app.Find("id:all").Props.Checked {
		t.Error("turning the switch off did not uncheck the checkbox")
	}
	app.Press("label:Two")
	if !app.Find("id:two").Props.Selected || app.Find("id:one").Props.Selected {
		t.Error("pressing a tab did not select it")
	}
}

func TestSliderIsAdjustable(t *testing.T) {
	var value *core.Signal[float64]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		value = core.NewSignal(cx, 0.5)
		return ui.ThemeView{Children: []core.Node{
			ui.Slider{Value: value.Get, OnChange: value.Set, Step: 0.25, Label: "Volume", TestID: "s"},
		}}
	})
	s := app.Find("role:adjustable Volume")
	if v := s.Props.Value; v != (core.AccessibilityValue{Min: 0, Max: 1, Now: 0.5}) {
		t.Fatalf("slider value = %+v", v)
	}
	for range 3 {
		app.AccessibilityAction("id:s", core.ActionIncrement)
	}
	if value.GetUntracked() != 1 || app.Find("id:s").Props.Value.Now != 1 {
		t.Errorf("after three increments the value is %v, want the maximum", value.GetUntracked())
	}
	app.AccessibilityAction("id:s", core.ActionDecrement)
	if value.GetUntracked() != 0.75 {
		t.Errorf("after a decrement the value is %v", value.GetUntracked())
	}
}

func TestSwitchHidesItsLabel(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return ui.ThemeView{Children: []core.Node{
			ui.Switch{Label: "Wi-Fi", HideLabel: true, TestID: "wifi"},
		}}
	})
	if app.Exists("text:Wi-Fi") {
		t.Fatal("the hidden label shows")
	}
	if app.Find("role:switch").Props.Label != "Wi-Fi" {
		t.Fatal("assistive technology lost the label")
	}
}

func TestSplitDrawsTheThemeBorder(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return ui.ThemeView{Children: []core.Node{
			ui.Split{Direction: base.SplitHorizontal, TestID: "split", DefaultSize: core.Pt(120),
				Style:  core.Style{Height: core.Pt(160)},
				First:  core.View{TestID: "first", Style: core.Style{Height: core.Pct(100)}},
				Second: core.View{TestID: "second", Style: core.Style{Height: core.Pct(100)}}},
		}}
	}, nsgotest.Size(400, 300))
	app.Find("id:split")
	line := app.Find("id:split-divider").Children[0]
	if line.Style.Background != ui.DefaultTheme().Light.Border {
		t.Errorf("divider line = %x, want the theme's border", line.Style.Background)
	}
	if line.Frame.Size != (core.Size{Width: 1, Height: 160}) {
		t.Errorf("divider line = %v, want one point wide and the split's height", line.Frame.Size)
	}
	if first := app.Find("id:first").Frame.Size.Width; first != 120 {
		t.Errorf("first pane = %v wide, want the default 120", first)
	}
	if c := app.Find("id:split-divider").Style.Cursor; c != core.CursorColResize {
		t.Errorf("divider cursor = %v, want the horizontal resize cursor", c)
	}
}

func TestWindowControlsCommandTheWindow(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return ui.ThemeView{Children: []core.Node{ui.WindowControls{TestID: "controls"}}}
	})
	env := core.Environment{OS: core.OSLinux, Idiom: core.IdiomDesktop}
	app.SetEnvironment(env)
	app.Press("id:controls-minimize")
	app.Press("id:controls-maximize")
	app.Press("id:controls-close")
	want := []core.WindowCommand{{Action: core.WindowMinimize}, {Action: core.WindowToggleMaximize}, {Action: core.WindowClose}}
	if !slices.Equal(app.Window, want) {
		t.Fatalf("window commands = %+v, want %+v", app.Window, want)
	}
	// The close button is Onevra's red under the pointer.
	app.Hover("id:controls-close", true)
	if got := app.Find("id:controls-close").Style.Background; got != core.RGB(0xe81123) {
		t.Fatalf("hovered close = %x, want Onevra's red", got)
	}
	// A maximized window shows restore.
	env.Maximized = true
	app.SetEnvironment(env)
	app.Find("id:controls-restore")
	if app.Exists("id:controls-maximize") {
		t.Fatal("the maximize button stays while the window is maximized")
	}
}
