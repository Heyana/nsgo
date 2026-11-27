package main

import "testing"

// TestIsApp checks that isApp matches only demos registered under the
// "Apps" group, regardless of which groups are compiled into tabs.
func TestIsApp(t *testing.T) {
	if isApp("") || isApp("NonexistentDemo") {
		t.Fatal("isApp returned true for unknown demo")
	}
	for _, g := range allGroups() {
		want := g.name == "Apps"
		for _, d := range g.demos {
			if got := isApp(d.name); got != want {
				t.Errorf("isApp(%q) in group %q = %v, want %v", d.name, g.name, got, want)
			}
		}
	}

	saved := tabs
	t.Cleanup(func() { tabs = saved })
	tabs = []tab{{
		id: "components",
		groups: []group{{
			name:  "Actions",
			demos: []demo{{name: "Buttons"}},
		}},
	}}
	if isApp("Buttons") {
		t.Fatal(`isApp("Buttons") = true when Actions is tabs[0].groups[0], want false`)
	}
	if isApp("Stocks") {
		t.Fatal(`isApp("Stocks") = true when Apps group is not compiled, want false`)
	}

	tabs = nil
	if isApp("Stocks") {
		t.Fatal(`isApp("Stocks") = true when tabs is empty, want false`)
	}
}

// TestPerGroupStartup verifies that init_tabs omits empty tabs and that
// Root renders in both desktop and phone layouts for any compiled subset
// of groups.
func TestPerGroupStartup(t *testing.T) {
	for _, tab := range tabs {
		if len(tab.groups) == 0 {
			t.Fatalf("tab %q has no groups; empty tabs must be omitted", tab.id)
		}
		for _, g := range tab.groups {
			if len(g.demos) == 0 {
				t.Fatalf("group %q in tab %q has no demos", g.name, tab.id)
			}
		}
	}

	t.Run("desktop", func(t *testing.T) {
		app := startDesktop(t)
		demos := allDemos()
		if len(demos) > 0 && app.Find("id:demo-"+demos[0].name) == nil {
			t.Fatalf("initial demo %q did not mount", demos[0].name)
		}
	})

	t.Run("phone", func(t *testing.T) {
		app := startPhone(t)
		if len(tabs) > 0 && app.Find("id:index") == nil {
			t.Fatal("phone index did not mount")
		}
	})
}
