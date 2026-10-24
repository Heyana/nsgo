package main

import (
	"strconv"
	"strings"

	"github.com/oneo-me/nsgo/core"
)

func storagePage(cx *core.Scope, t *theme) core.Node {
	s := cx.Storage()
	// Untracked: the section must not rebuild on its own count.
	cx.Untrack(func() {
		opened, _ := s.GetItem("gallery.opened")
		n, _ := strconv.Atoi(opened)
		s.SetItem("gallery.opened", strconv.Itoa(n+1))
	})
	draft := func() string { v, _ := s.GetItem("gallery.draft"); return v }
	dir := func(d string) string {
		if d == "" {
			return "none on this target"
		}
		return d
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Storage", "Scope.Storage keeps strings by key on the device, in a file in the data directory or in localStorage on the web. Reads subscribe to changes.",
			core.Input{Value: draft, OnChange: func(v string) { s.SetItem("gallery.draft", v) },
				Placeholder: "Type, then restart the gallery", Style: t.field(), Label: "Draft", TestID: "draft"},
			t.live(func() string { return "Kept: " + strconv.Quote(draft()) }),
			t.live(func() string {
				v, _ := s.GetItem("gallery.opened")
				return "This section was opened " + v + " times."
			}),
			t.live(func() string { return "Keys: " + strings.Join(s.Keys(), ", ") }),
			flow(t.destructive("Clear storage", s.Clear)),
		),
		t.demo("App directories", "Scope.DataDir and Scope.CacheDir are the app's own, named by its ID.",
			t.note("Data: "+dir(cx.DataDir())),
			t.note("Cache: "+dir(cx.CacheDir())),
		),
	}}
}
