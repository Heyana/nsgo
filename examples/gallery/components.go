package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"
)

// The pages of the ui package's components, which take the gallery's
// theme from the ThemeView at its root.

func always() bool { return true }

func fieldsPage(cx *core.Scope, t *theme) core.Node {
	name := core.NewSignal(cx, "")
	query := core.NewSignal(cx, "")
	password := core.NewSignal(cx, "")
	size := core.NewSignal(cx, "medium")
	city := core.NewSignal(cx, "")
	volume := core.NewSignal(cx, 40.0)
	rating := core.NewSignal(cx, 3.0)
	likes := core.NewSignal(cx, 0)
	last := core.NewSignal(cx, "nothing yet")
	fields := func(children ...core.Node) core.Node {
		return core.View{Style: core.Style{Gap: 10, MaxWidth: core.Pt(380)}, Children: children}
	}
	slider := func(s ui.Slider, value func() string) core.Node {
		s.Style = core.Style{Width: core.Pt(240)}
		return hstack(s, core.Text{ContentFunc: value, Style: core.Style{FontFamily: core.FontMonospace, FontSize: 13, Color: t.muted}})
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Input", "ui.Input is core.Input in the theme's field, marking focus with the ring color; Icon leads it.",
			fields(
				ui.Input{Value: name.Get, OnChange: name.Set, Placeholder: "Your name", Label: "Name", TestID: "field-name"},
				ui.Input{Value: query.Get, OnChange: query.Set, Placeholder: "Search", Icon: lucide.Search(), Label: "Search", TestID: "field-search"},
				ui.Input{Value: password.Get, OnChange: password.Set, Placeholder: "Password", Masked: true, Label: "Password"},
				ui.Input{DefaultValue: "Not editable", Disabled: always, Label: "Disabled"},
			),
			t.live(func() string {
				if n := strings.TrimSpace(name.Get()); n != "" {
					return "Hello, " + n + "."
				}
				return "Type your name."
			}),
		),
		t.demo("Select", "ui.Select shows the chosen option and opens a menu of them, the chosen one checked.",
			flow(
				ui.Select{Value: size.Get, OnChange: size.Set, Label: "Size", TestID: "select-size", Options: []ui.SelectOption{
					{Value: "small", Label: "Small"}, {Value: "medium", Label: "Medium"}, {Value: "large", Label: "Large"},
				}},
				ui.Select{Value: city.Get, OnChange: city.Set, Placeholder: "Choose a city", Label: "City", Options: []ui.SelectOption{
					{Value: "lisbon", Label: "Lisbon", Icon: lucide.MapPin()},
					{Value: "kyoto", Label: "Kyoto", Icon: lucide.MapPin()},
					{Value: "oslo", Label: "Oslo", Icon: lucide.MapPin()},
				}},
			),
			t.live(func() string {
				if city.Get() == "" {
					return fmt.Sprintf("Size %s, no city yet.", size.Get())
				}
				return fmt.Sprintf("Size %s in %s.", size.Get(), city.Get())
			}),
		),
		t.demo("Slider", "ui.Slider: drag anywhere along it; Step snaps the value and Min and Max bound it.",
			slider(ui.Slider{Value: volume.Get, OnChange: volume.Set, Max: 100, Label: "Volume", TestID: "slider-volume"},
				func() string { return fmt.Sprintf("%.0f%%", volume.Get()) }),
			slider(ui.Slider{Value: rating.Get, OnChange: rating.Set, Min: 1, Max: 5, Step: 1, Label: "Rating"},
				func() string { return fmt.Sprintf("%.0f of 5", rating.Get()) }),
			slider(ui.Slider{Value: func() float64 { return 0.6 }, Disabled: always, Label: "Disabled"},
				func() string { return "disabled" }),
		),
		t.demo("Icon buttons", "ui.IconButton is an icon its label names, in a tooltip on desktops; with Menu it opens a menu.",
			hstack(
				ui.IconButton{Icon: lucide.Heart(), Label: "Like", OnPress: func(core.PressEvent) { likes.Update(func(n int) int { return n + 1 }) }},
				ui.IconButton{Icon: lucide.Copy(), Label: "Copy", OnPress: func(core.PressEvent) { last.Set("Copy") }},
				ui.IconButton{Icon: lucide.Share2(), Label: "Share", OnPress: func(core.PressEvent) { last.Set("Share") }},
				ui.IconButton{Icon: lucide.Trash(), Label: "Delete", Disabled: always},
				ui.IconButton{Icon: lucide.Ellipsis(), Label: "More", Menu: []core.Node{
					ui.MenuItem{Label: "Rename", OnSelect: func() { last.Set("Rename") }},
					ui.MenuItem{Label: "Duplicate", OnSelect: func() { last.Set("Duplicate") }},
					ui.MenuSeparator{},
					ui.MenuItem{Label: "Archive", OnSelect: func() { last.Set("Archive") }},
				}},
			),
			t.live(func() string { return fmt.Sprintf("Liked %d times; last chose %s.", likes.Get(), last.Get()) }),
		),
	}}
}

func feedbackPage(cx *core.Scope, t *theme) core.Node {
	toast := ui.UseToast(cx)
	progress := core.NewSignal(cx, 0.4)
	var stop func()
	upload := func() {
		if stop != nil {
			stop()
		}
		progress.Set(0)
		stop = cx.Animate(2*time.Second, core.EaseInOut, func(p float64) {
			progress.Set(p)
			if p == 1 {
				toast(ui.Toast{Title: "Upload finished", Description: "report.pdf is in your files.",
					Icon: lucide.CircleCheck(), Tone: ui.ToneSuccess})
			}
		})
	}
	nudge := func(by float64) func(core.PressEvent) {
		return func(core.PressEvent) { progress.Update(func(v float64) float64 { return min(max(v+by, 0), 1) }) }
	}
	toasts := []struct {
		label string
		toast ui.Toast
	}{
		{"Saved", ui.Toast{Title: "Changes saved", Icon: lucide.CircleCheck(), Tone: ui.ToneSuccess}},
		{"Warning", ui.Toast{Title: "Storage almost full", Description: "Free some space to keep syncing.",
			Icon: lucide.TriangleAlert(), Tone: ui.ToneWarning}},
		{"Error", ui.Toast{Title: "Could not connect", Description: "Check the network and try again.",
			Icon: lucide.CircleX(), Tone: ui.ToneDanger}},
		{"Plain", ui.Toast{Title: "A plain notice"}},
	}
	var toastButtons []core.Node
	for _, b := range toasts {
		toastButtons = append(toastButtons, ui.Button{Label: b.label, Variant: ui.Secondary, OnPress: func(core.PressEvent) { toast(b.toast) }})
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Badge", "ui.Badge is a pill of text in a tone, with an optional icon.",
			flow(
				ui.Badge{Text: "Draft"},
				ui.Badge{Text: "New", Tone: ui.ToneAccent},
				ui.Badge{Text: "Passed", Tone: ui.ToneSuccess, Icon: lucide.CircleCheck()},
				ui.Badge{Text: "Pending", Tone: ui.ToneWarning, Icon: lucide.Clock()},
				ui.Badge{Text: "Failed", Tone: ui.ToneDanger, Icon: lucide.CircleX()},
				ui.Badge{Text: "Beta", Tone: ui.ToneInfo, Icon: lucide.Info()},
			),
		),
		t.demo("Progress", "ui.Progress fills as far as its value goes from 0 to 1.",
			ui.Progress{Value: progress.Get, Label: "Upload", TestID: "progress"},
			flow(
				ui.Button{Label: "Upload", OnPress: func(core.PressEvent) { upload() }, Children: []core.Node{
					core.Icon{SVG: lucide.Upload(), Style: core.Style{Width: core.Pt(16), Height: core.Pt(16)}},
					core.Text{Content: "Upload"},
				}},
				ui.Button{Label: "−10%", Variant: ui.Secondary, OnPress: nudge(-0.1)},
				ui.Button{Label: "+10%", Variant: ui.Secondary, OnPress: nudge(0.1)},
			),
			t.live(func() string { return fmt.Sprintf("%.0f%%", progress.Get()*100) }),
		),
		t.demo("Toast", "ui.UseToast shows a notice in the nearest ToastView, which leaves after a while or when pressed.",
			flow(toastButtons...),
		),
	}}
}

func settingsPage(cx *core.Scope, t *theme) core.Node {
	g := galleryKey.Use(cx)
	hideSearch := core.NewSignal(cx, false)
	hideSidebar := core.NewSignal(cx, false)
	wifi := core.NewSignal(cx, true)
	network := core.NewSignal(cx, "home")
	bluetooth := core.NewSignal(cx, false)
	brightness := core.NewSignal(cx, 70.0)
	night := core.NewSignal(cx, true)
	volume := core.NewSignal(cx, 50.0)
	output := core.NewSignal(cx, "speakers")
	sw := func(v *core.Signal[bool], label string) core.Node {
		return ui.Switch{Checked: v.Get, OnChange: v.Set, Label: label, HideLabel: true}
	}
	level := func(v *core.Signal[float64], label string) core.Node {
		return ui.Slider{Value: v.Get, OnChange: v.Set, Max: 100, Label: label, Style: core.Style{Width: core.Pt(180)}}
	}
	categories := []ui.SettingsCategory{
		{ID: "wifi", Title: "Wi-Fi", Section: "Network", Icon: lucide.Wifi(), Groups: []ui.SettingsGroup{{ID: "wifi", Items: []ui.SettingsItem{
			{ID: "on", Title: "Wi-Fi", Description: "Join networks nearby.", Control: sw(wifi, "Wi-Fi")},
			{ID: "network", Title: "Network", Keywords: []string{"ssid"}, Control: ui.Select{Value: network.Get, OnChange: network.Set,
				Label: "Network", Options: []ui.SelectOption{{Value: "home", Label: "Home"}, {Value: "office", Label: "Office"}, {Value: "cafe", Label: "Café"}}}},
		}}}},
		{ID: "bluetooth", Title: "Bluetooth", Section: "Network", Icon: lucide.Bluetooth(), Groups: []ui.SettingsGroup{{ID: "bt", Items: []ui.SettingsItem{
			{ID: "on", Title: "Bluetooth", Description: "Connect headphones, keyboards and more.", Control: sw(bluetooth, "Bluetooth")},
		}}}},
		{ID: "display", Title: "Display", Section: "Device", Icon: lucide.Monitor(), Groups: []ui.SettingsGroup{{ID: "display", Items: []ui.SettingsItem{
			{ID: "brightness", Title: "Brightness", Control: level(brightness, "Brightness")},
			{ID: "night", Title: "Night Shift", Description: "Warmer colors after dark.", Keywords: []string{"warm"}, Control: sw(night, "Night Shift")},
		}}}},
		{ID: "sound", Title: "Sound", Section: "Device", Icon: lucide.Volume2(), Groups: []ui.SettingsGroup{{ID: "sound", Items: []ui.SettingsItem{
			{ID: "volume", Title: "Volume", Control: level(volume, "Volume")},
			{ID: "output", Title: "Output", Control: ui.Select{Value: output.Get, OnChange: output.Set, Label: "Output",
				Options: []ui.SelectOption{{Value: "speakers", Label: "Speakers"}, {Value: "headphones", Label: "Headphones"}}}},
		}}}},
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("The gallery's settings", "ui.Settings in a bare ui.Dialog: the gallery's preferences, then template pages.",
			flow(
				ui.Button{Label: "Open settings", OnPress: func(core.PressEvent) { g.openSettings() }, TestID: "page-open-settings"},
				keycap(cx.KeyLabel("mod+,")),
			),
		),
		t.demo("In a page", "ui.Settings fills its parent, here a card. Search finds items by any word; narrow, a selector replaces the sidebar.",
			flow(
				ui.Switch{Label: "Hide search", Checked: hideSearch.Get, OnChange: hideSearch.Set},
				ui.Switch{Label: "Hide sidebar", Checked: hideSidebar.Get, OnChange: hideSidebar.Set},
			),
			core.View{Style: core.Style{Height: core.Pt(400), CornerRadius: cardRadius, BorderWidth: 1, BorderColor: t.line,
				Overflow: core.Hidden, Background: t.background}, Children: []core.Node{
				core.Dynamic(func(cx *core.Scope) core.Node {
					return ui.Settings{Categories: categories, Title: "Preferences", HideSearch: hideSearch.Get(),
						HideSidebar: hideSidebar.Get(), TestID: "embedded-settings"}
				}),
			}},
		),
	}}
}

func commandPage(cx *core.Scope, t *theme) core.Node {
	g := galleryKey.Use(cx)
	open := core.NewSignal(cx, false)
	last := core.NewSignal(cx, "")
	files := []ui.Command{
		{ID: "new", Title: "New file", Group: "File", Icon: lucide.FilePlus(), Shortcut: cx.KeyLabel("mod+n")},
		{ID: "open", Title: "Open…", Group: "File", Icon: lucide.FolderOpen(), Shortcut: cx.KeyLabel("mod+o")},
		{ID: "save", Title: "Save", Group: "File", Icon: lucide.Save(), Shortcut: cx.KeyLabel("mod+s"), Disabled: true},
		{ID: "rename", Title: "Rename", Group: "Edit", Icon: lucide.Pencil(), Keywords: []string{"move"}},
		{ID: "duplicate", Title: "Duplicate", Group: "Edit", Icon: lucide.CopyPlus()},
		{ID: "tag", Title: "Add a tag", Group: "Edit", Icon: lucide.Tag(), Description: "Group files by what they are about."},
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("The gallery's palette", "Every page and the gallery's commands. Up and Down choose, Return runs, Escape closes.",
			flow(
				ui.Button{Label: "Search everything", OnPress: func(core.PressEvent) { g.openPalette() }, TestID: "page-open-palette"},
				keycap(cx.KeyLabel("mod+k")),
			),
		),
		t.demo("Your own commands", "ui.CommandPalette takes the app's commands, grouped, and reports the one run; disabled ones are skipped.",
			flow(ui.Button{Label: "Open file commands", Variant: ui.Secondary, OnPress: func(core.PressEvent) { open.Set(true) }, TestID: "open-files"}),
			t.live(func() string {
				if last.Get() == "" {
					return "No command run yet."
				}
				return "Ran " + last.Get() + "."
			}),
			ui.CommandPalette{Open: open.Get, OnOpenChange: func(o bool, _ base.Reason) { open.Set(o) },
				Commands: func() []ui.Command { return files }, OnSelect: last.Set, Placeholder: "Search file commands",
				Label: "File commands"},
		),
	}}
}
