package main

import (
	"fmt"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// basePage shows the parts the engine owns with no look: a pressable
// view, popups and menus, navigation and safe areas. The app styles them.
func basePage(cx *core.Scope, t *theme) core.Node {
	presses := core.NewSignal(cx, 0)
	press := func() { presses.Update(func(n int) int { return n + 1 }) }
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Button", "base.Button is a pressable view with the button role; its look is the app's.",
			flow(
				t.primary("Primary", press),
				t.plain("Secondary", press),
				t.destructive("Destructive", press),
				base.Button{OnPress: func() {}, Disabled: func() bool { return true }, Label: "Disabled", Style: t.button(),
					Children: []core.Node{core.Text{Content: "Disabled"}}},
			),
			t.live(func() string { return fmt.Sprintf("%d presses", presses.Get()) }),
		),
		t.demo("Safe area", "base.SafeAreaView adds the screen's safe-area insets to its padding on the chosen edges.",
			base.SafeAreaView{Edges: base.Top | base.Bottom, TestID: "base-safe-area",
				Style:    core.Style{Padding: core.All(12), CornerRadius: radius, Background: t.sunken},
				Children: []core.Node{core.Text{Content: "This box keeps clear of the system UI above and below."}}},
		),
		popupsDemo(cx, t),
		navigationDemo(cx, t),
	}}
}
