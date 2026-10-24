package main

import (
	"fmt"
	"strings"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"
)

// prefs are the values of the settings dialog's template pages. They show
// what a settings surface holds and stay in memory for the session; the
// gallery's own preferences are kept in storage.
type prefs struct {
	name, email, bio           *core.Signal[string]
	smart                      *core.Signal[bool]
	quiet                      *core.Signal[string]
	channels                   []*core.Signal[string]
	fontSize                   *core.Signal[float64]
	tabWidth                   *core.Signal[string]
	lineNumbers, wrap, minimap *core.Signal[bool]
	cache                      *core.Signal[float64]
	downloads                  *core.Signal[string]
	updates                    *core.Signal[string]
}

// notificationRows are the template's notifications, by group, with the
// channel each starts on.
var notificationRows = []struct{ group, title, about, channel string }{
	{"Account & security", "Security alerts", "Unusual activity or important security updates.", "email"},
	{"Account & security", "New sign-in", "Your account is used on a new device.", "email"},
	{"Workspace", "Role changes", "Your access to a workspace changes.", "app"},
	{"Workspace", "New members", "Someone joins a workspace.", "app"},
	{"Messages", "Mentions", "Someone mentions you in a conversation.", "app"},
	{"Messages", "Shared notes", "A note is shared with you.", "email"},
}

func newPrefs(cx *core.Scope) *prefs {
	p := &prefs{
		name: core.NewSignal(cx, "Alex Morgan"), email: core.NewSignal(cx, "alex@example.com"), bio: core.NewSignal(cx, ""),
		smart: core.NewSignal(cx, true), quiet: core.NewSignal(cx, "night"),
		fontSize: core.NewSignal(cx, 14.0), tabWidth: core.NewSignal(cx, "4"),
		lineNumbers: core.NewSignal(cx, true), wrap: core.NewSignal(cx, true), minimap: core.NewSignal(cx, false),
		cache: core.NewSignal(cx, 2.4), downloads: core.NewSignal(cx, "~/Downloads/NSGo"), updates: core.NewSignal(cx, "stable"),
	}
	for _, r := range notificationRows {
		p.channels = append(p.channels, core.NewSignal(cx, r.channel))
	}
	return p
}

// settingsDialog hosts the settings surface in a dialog over the gallery.
func (g *gallery) settingsDialog() core.Node {
	return ui.Dialog{Open: g.settings.Get, OnOpenChange: func(open bool, _ base.Reason) { g.settings.Set(open) },
		Label: "Settings", TestID: "settings", Bare: true,
		PopupStyle: core.Style{Width: core.Pt(820), Height: core.Pt(600), MaxWidth: core.Pct(92), MaxHeight: core.Pct(88),
			Overflow: core.Hidden, CornerRadius: dialogRadius},
		Children: []core.Node{core.Func(func(cx *core.Scope) core.Node {
			return ui.Settings{Categories: g.settingsCategories(cx), Category: g.category.Get, OnCategoryChange: g.category.Set,
				OnClose: func() { g.settings.Set(false) }, NarrowWidth: 600}
		})},
	}
}

// settingsCategories are the gallery's settings, then template pages.
func (g *gallery) settingsCategories(cx *core.Scope) []ui.SettingsCategory {
	toast := ui.UseToast(cx)
	p := g.prefs
	sw := func(value *core.Signal[bool], label string) core.Node {
		return ui.Switch{Checked: value.Get, OnChange: value.Set, Label: label, HideLabel: true}
	}
	choose := func(value *core.Signal[string], label string, options ...ui.SelectOption) core.Node {
		return ui.Select{Value: value.Get, OnChange: value.Set, Options: options, Label: label}
	}
	gallery := []ui.SettingsCategory{
		{ID: "general", Title: "General", Section: "Gallery", Icon: lucide.Settings(), Groups: []ui.SettingsGroup{
			{ID: "pages", Items: []ui.SettingsItem{
				{ID: "start", Title: "Open on", Description: "The page the gallery shows when it starts.",
					Keywords: []string{"launch", "start"}, Control: choose(g.start, "Open on",
						ui.SelectOption{Value: "welcome", Label: "Welcome", Icon: lucide.House()},
						ui.SelectOption{Value: "last", Label: "The last page shown", Icon: lucide.Clock()})},
				{ID: "descriptions", Title: "Page descriptions", Description: "A line under each page's title saying what it shows.",
					Keywords: []string{"about", "help"}, Control: sw(g.descriptions, "Page descriptions")},
				{ID: "sidebar", Title: "Sidebar", Description: "The list of pages beside them on wide windows.",
					Keywords: []string{"navigation"}, Control: sw(g.sidebar, "Sidebar")},
			}},
		}},
		{ID: "appearance", Title: "Appearance", Section: "Gallery", Icon: lucide.Palette(), Groups: []ui.SettingsGroup{
			{ID: "theme", Items: []ui.SettingsItem{
				{ID: "appearance", Title: "Appearance", Description: "Light or dark, or as the system is. The window's own controls follow the system.",
					Keywords: []string{"theme", "dark mode", "light mode"},
					Control:  choose(g.appearance, "Appearance", appearanceOptions()...)},
			}},
		}},
		{ID: "shortcuts", Title: "Shortcuts", Section: "Gallery", Icon: lucide.Keyboard(), Groups: []ui.SettingsGroup{
			{ID: "keys", Items: []ui.SettingsItem{
				{ID: "search", Title: "Search", Description: "Find a page or a command.", Control: keycap(cx.KeyLabel("mod+k"))},
				{ID: "settings", Title: "Settings", Description: "Open these settings.", Control: keycap(cx.KeyLabel("mod+,"))},
				{ID: "toggle-sidebar", Title: "Toggle the sidebar", Control: keycap(cx.KeyLabel("mod+alt+s"))},
				{ID: "pages", Title: "Pages", Description: "The first nine pages, in order.", Control: keycap(cx.KeyLabel("mod+1") + "…9")},
				{ID: "step", Title: "Next and previous page", Control: keycap("Ctrl+Tab")},
			}},
		}},
	}

	notifications := ui.SettingsCategory{ID: "notifications", Title: "Notifications", Section: "Templates", Icon: lucide.Bell(),
		Description: "A template: changes stay in this session and send nothing.",
		Groups: []ui.SettingsGroup{{ID: "delivery", Items: []ui.SettingsItem{
			{ID: "smart", Title: "Smart notifications", Description: "Pause phone alerts while you are active on a computer.",
				Control: sw(p.smart, "Smart notifications")},
			{ID: "quiet", Title: "Quiet hours", Description: "Hold alerts while you rest.", Keywords: []string{"do not disturb"},
				Leading: core.Icon{SVG: lucide.Moon(), Style: core.Style{Width: core.Pt(18), Height: core.Pt(18)}},
				Control: choose(p.quiet, "Quiet hours",
					ui.SelectOption{Value: "off", Label: "Off"},
					ui.SelectOption{Value: "night", Label: "22:00 – 07:00"},
					ui.SelectOption{Value: "late", Label: "23:00 – 08:00"})},
		}}},
	}
	channels := []ui.SelectOption{{Value: "email", Label: "Email"}, {Value: "app", Label: "In the app"}, {Value: "off", Label: "Off"}}
	for i, r := range notificationRows {
		groups := &notifications.Groups
		if last := (*groups)[len(*groups)-1]; last.ID != r.group {
			group := r.group
			*groups = append(*groups, ui.SettingsGroup{ID: group, Title: group, Actions: []core.Node{
				ui.Button{Label: "Reset", Variant: ui.Ghost, Style: core.Style{Padding: core.Symmetric(4, 10)}, OnPress: func(core.PressEvent) {
					for j, r := range notificationRows {
						if r.group == group {
							p.channels[j].Set(r.channel)
						}
					}
				}},
			}})
		}
		group := &(*groups)[len(*groups)-1]
		group.Items = append(group.Items, ui.SettingsItem{ID: r.title, Title: r.title, Description: r.about,
			Control: choose(p.channels[i], r.title, channels...)})
	}

	templates := []ui.SettingsCategory{
		{ID: "account", Title: "Account", Section: "Templates", Icon: lucide.CircleUser(), Groups: []ui.SettingsGroup{
			{ID: "profile", Items: []ui.SettingsItem{
				{ID: "card", Title: "Profile", Keywords: []string{"avatar"}, Content: profileCard(p)},
				{ID: "name", Title: "Display name", Description: "How others see you.", Stacked: true,
					Control: ui.Input{Value: p.name.Get, OnChange: p.name.Set, Label: "Display name"}},
				{ID: "email", Title: "Email", Stacked: true,
					Control: ui.Input{Value: p.email.Get, OnChange: p.email.Set, Label: "Email", KeyboardType: core.KeyboardEmail}},
				{ID: "bio", Title: "About you", Description: "A line for your profile.", Stacked: true,
					Control: ui.Input{Value: p.bio.Get, OnChange: p.bio.Set, Placeholder: "Say something", Label: "About you"}},
			}},
		}},
		notifications,
		{ID: "editor", Title: "Editor", Section: "Templates", Icon: lucide.Code(), Groups: []ui.SettingsGroup{
			{ID: "text", Title: "Text", Items: []ui.SettingsItem{
				{ID: "font-size", Title: "Font size", Keywords: []string{"zoom"},
					Description: "Points of the editor's text.",
					Control: core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
						ui.Slider{Value: p.fontSize.Get, OnChange: p.fontSize.Set, Min: 10, Max: 24, Step: 1, Label: "Font size",
							Style: core.Style{Width: core.Pt(160)}},
						core.Text{ContentFunc: func() string { return fmt.Sprintf("%.0f pt", p.fontSize.Get()) },
							Style: core.Style{Width: core.Pt(44), FontFamily: core.FontMonospace, FontSize: 13}},
					}}},
				{ID: "tab-width", Title: "Tab width", Control: choose(p.tabWidth, "Tab width",
					ui.SelectOption{Value: "2", Label: "2 spaces"}, ui.SelectOption{Value: "4", Label: "4 spaces"},
					ui.SelectOption{Value: "8", Label: "8 spaces"})},
			}},
			{ID: "view", Title: "View", Items: []ui.SettingsItem{
				{ID: "line-numbers", Title: "Line numbers", Keywords: []string{"gutter"}, Control: sw(p.lineNumbers, "Line numbers")},
				{ID: "wrap", Title: "Word wrap", Description: "Wrap long lines at the window's edge.", Control: sw(p.wrap, "Word wrap")},
				{ID: "minimap", Title: "Minimap", Description: "A narrow overview of the whole file.", Control: sw(p.minimap, "Minimap")},
			}},
		}},
		{ID: "storage", Title: "Storage", Section: "Templates", Icon: lucide.HardDrive(), Groups: []ui.SettingsGroup{
			{ID: "cache", Title: "Cache", Items: []ui.SettingsItem{
				{ID: "usage", Title: "Cache", Stacked: true,
					Description: "Previews and downloads kept to open them faster.",
					Control: core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
						ui.Progress{Value: func() float64 { return p.cache.Get() / 10 }, Label: "Cache used"},
						core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
							core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB of 10 GB", p.cache.Get()) },
								Style: core.Style{FlexGrow: 1, FontSize: 13}},
							ui.Button{Label: "Clear cache", Variant: ui.Secondary, Disabled: func() bool { return p.cache.Get() == 0 },
								OnPress: func(core.PressEvent) {
									p.cache.Set(0)
									toast(ui.Toast{Title: "Cache cleared", Icon: lucide.CircleCheck(), Tone: ui.ToneSuccess})
								}},
						}},
					}}},
				{ID: "downloads", Title: "Downloads", Description: "Where downloaded files go.", Stacked: true,
					Control: ui.Input{Value: p.downloads.Get, OnChange: p.downloads.Set, Icon: lucide.Folder(), Label: "Downloads"}},
			}},
		}},
		{ID: "about", Title: "About", Section: "Templates", Icon: lucide.Info(), Groups: []ui.SettingsGroup{
			{ID: "app", Items: []ui.SettingsItem{
				{ID: "card", Title: "NSGo Gallery", Description: "Version " + version, Content: aboutCard(cx)},
				{ID: "updates", Title: "Updates", Description: "Which releases to offer.", Control: choose(p.updates, "Updates",
					ui.SelectOption{Value: "stable", Label: "Stable"}, ui.SelectOption{Value: "beta", Label: "Beta"})},
			}},
		}},
	}
	return append(gallery, templates...)
}

// keycap shows a shortcut as a key.
func keycap(keys string) core.Node {
	return ui.Badge{Text: keys, Style: core.Style{FontFamily: core.FontMonospace, CornerRadius: 6}}
}

// profileCard is the account's card: initials on a tinted disc, the name
// and the email, following their fields.
func profileCard(p *prefs) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		theme := ui.UseTheme(cx)
		return core.View{
			StyleFunc: func() core.Style {
				t := theme()
				return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 14, Padding: core.All(14),
					CornerRadius: t.RadiusLarge, BorderWidth: 1, BorderColor: t.Border, Background: t.Background}
			},
			Children: []core.Node{
				core.View{StyleFunc: func() core.Style {
					t := theme()
					return core.Style{Width: core.Pt(48), Height: core.Pt(48), CornerRadius: 24, AlignItems: core.AlignCenter,
						JustifyContent: core.JustifyCenter, Background: ui.Mix(t.Background, t.Primary, 0.18), Color: t.Primary,
						FontSize: 18, FontWeight: core.FontWeightSemibold}
				}, Children: []core.Node{core.Text{ContentFunc: func() string { return initials(p.name.Get()) }}}},
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
					core.Text{ContentFunc: p.name.Get, MaxLines: 1, Style: core.Style{FontWeight: core.FontWeightSemibold}},
					core.Text{ContentFunc: p.email.Get, MaxLines: 1, StyleFunc: func() core.Style {
						return core.Style{Color: theme().MutedForeground, FontSize: 13}
					}},
				}},
				ui.Badge{Text: "Owner", Tone: ui.ToneAccent},
			},
		}
	})
}

func initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		out = append(out, []rune(strings.ToUpper(w))[0])
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		return "?"
	}
	return string(out)
}

// aboutCard is the gallery's name, version and license beside its mark.
func aboutCard(cx *core.Scope) core.Node {
	g := galleryKey.Use(cx)
	return core.Dynamic(func(cx *core.Scope) core.Node {
		t := newTheme(g.dark())
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 16}, Children: []core.Node{
			brandMark(t, 56),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 6}, Children: []core.Node{
				core.Text{Content: "NSGo Gallery", Style: core.Style{FontSize: 17, FontWeight: core.FontWeightSemibold}},
				core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 6}, Children: []core.Node{
					ui.Badge{Text: "Version " + version}, ui.Badge{Text: "Apache-2.0"}, ui.Badge{Text: repository},
				}},
			}},
		}}
	})
}
