package main

import (
	"fmt"
	"strings"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"
)

// commandPalette finds every page and the gallery's commands.
func (g *gallery) commandPalette() core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		return ui.CommandPalette{Open: g.palette.Get, OnOpenChange: func(open bool, _ base.Reason) { g.palette.Set(open) },
			Commands: func() []ui.Command { return g.commands(cx) }, OnSelect: g.run,
			Placeholder: "Search pages and commands", TestID: "palette"}
	})
}

// commands are the palette's: each page under its group, then actions.
func (g *gallery) commands(cx *core.Scope) []ui.Command {
	var out []ui.Command
	for i, s := range sections {
		// The description finds a page by what it shows without crowding
		// the rows.
		c := ui.Command{ID: "page:" + s.name, Title: s.name, Group: s.group, Icon: s.icon(), Keywords: []string{s.about}}
		if i < 9 {
			c.Shortcut = shortcut(cx, fmt.Sprint(i+1))
		}
		out = append(out, c)
	}
	sidebar := "Hide sidebar"
	if !g.sidebar.Get() {
		sidebar = "Show sidebar"
	}
	descriptions := "Hide page descriptions"
	if !g.descriptions.Get() {
		descriptions = "Show page descriptions"
	}
	out = append(out,
		ui.Command{ID: "settings", Title: "Open settings", Group: "Commands", Icon: lucide.Settings(),
			Shortcut: shortcut(cx, ","), Keywords: []string{"preferences", "options"}},
		ui.Command{ID: "sidebar", Title: sidebar, Group: "Commands", Icon: lucide.PanelLeft(),
			Shortcut: modName(cx) + "+Alt+S", Keywords: []string{"toggle", "navigation"}, Disabled: !cx.Env().Desktop()},
		ui.Command{ID: "descriptions", Title: descriptions, Group: "Commands", Icon: lucide.Info(), Keywords: []string{"toggle", "about"}},
	)
	for _, a := range appearanceOptions() {
		out = append(out, ui.Command{ID: "appearance:" + a.Value, Title: "Use the " + a.Label + " appearance", Group: "Appearance",
			Icon: a.Icon, Keywords: []string{"theme", a.Value + " mode"}, Disabled: g.appearance.Get() == a.Value})
	}
	return out
}

// run carries out a palette command.
func (g *gallery) run(id string) {
	switch id {
	case "settings":
		g.openSettings()
	case "sidebar":
		g.toggleSidebar()
	case "descriptions":
		g.descriptions.Update(func(on bool) bool { return !on })
	default:
		if name, ok := strings.CutPrefix(id, "page:"); ok {
			g.open(name)
		} else if a, ok := strings.CutPrefix(id, "appearance:"); ok {
			g.appearance.Set(a)
		}
	}
}
