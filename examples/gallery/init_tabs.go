package main

import (
	"slices"

	"github.com/nsgo-dev/nsgo/examples/gallery/locales"
	"github.com/nsgo-dev/nsgo/icons/lucide"
)

// The tab assembly runs after the group files' init, which file order
// puts first, so every group that compiled is registered.
func init() {
	type m = locales.Messages
	slices.SortStableFunc(groupRegistrations, func(a, b groupRegistration) int {
		if a.tab != b.tab {
			if a.tab == "components" {
				return -1
			}
			return 1
		}
		return a.order - b.order
	})
	groupsFor := func(tab string) []group {
		var out []group
		for _, g := range groupRegistrations {
			if g.tab == tab {
				out = append(out, group{name: g.name, title: g.title, demos: g.demos})
			}
		}
		return out
	}
	tabs = []tab{
		{"components", m.TabComponents, lucide.Component, "puzzlepiece.extension", "android:ic_menu_agenda", groupsFor("components")},
		{"basics", m.TabBasics, lucide.Shapes, "square.on.circle", "android:ic_menu_sort_by_size", groupsFor("basics")},
	}
	for _, t := range tabs {
		for i := range t.groups {
			t.groups[i].demos = slices.DeleteFunc(t.groups[i].demos, func(d demo) bool { return omitted[d.name] })
		}
	}
}
