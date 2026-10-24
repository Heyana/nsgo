package main

import (
	"fmt"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// The pages of base's components, which own behavior and accessibility
// and take their look from the gallery's theme.

func buttonPage(cx *core.Scope, t *theme) core.Node {
	presses := core.NewSignal(cx, 0)
	press := func() { presses.Update(func(n int) int { return n + 1 }) }
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Button", "Each look here is a style the app gives base.Button, its states included.",
			flow(
				t.primary("Primary", press),
				t.plain("Secondary", press),
				t.destructive("Destructive", press),
				base.Button{OnPress: func(core.PressEvent) {}, Disabled: func() bool { return true }, Label: "Disabled", Style: t.button(),
					Children: []core.Node{core.Text{Content: "Disabled"}}},
			),
			t.live(func() string { return fmt.Sprintf("%d presses", presses.Get()) }),
		),
	}}
}

func safeAreaPage(cx *core.Scope, t *theme) core.Node {
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Safe area", "base.SafeAreaView adds the screen's safe-area insets to its padding on the chosen edges.",
			base.SafeAreaView{Edges: base.Top | base.Bottom, TestID: "base-safe-area",
				Style:    core.Style{Padding: core.All(12), CornerRadius: radius, Background: t.sunken},
				Children: []core.Node{core.Text{Content: "This box keeps clear of the system UI above and below."}}},
		),
	}}
}

func splitPage(cx *core.Scope, t *theme) core.Node {
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Split", "base.Split resizes its first pane with a drag; a double click restores its default. Sizes are points or percentages.",
			base.Split{Direction: base.SplitHorizontal, TestID: "base-split", Label: "Resize the example split",
				DefaultSize: core.Pct(35), Min: core.Pt(120), Max: core.Pct(70),
				Style:        core.Style{Height: core.Pt(170), CornerRadius: radius, BorderWidth: 1, BorderColor: t.line, Overflow: core.Hidden},
				DividerStyle: core.Style{Hover: &core.Style{Background: t.hover}, Active: &core.Style{Background: t.accentSoft}},
				LineStyle:    core.Style{Background: t.line},
				First: core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), Padding: core.All(12), Gap: 4,
					Background: t.quiet}, Children: []core.Node{
					core.Text{Content: "First pane", Style: core.Style{FontWeight: core.FontWeightSemibold}},
					t.note("35%, at least 120 points, at most 70%."),
				}},
				Second: core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), Padding: core.All(12), Gap: 4,
					Background: t.surface}, Children: []core.Node{
					core.Text{Content: "Second pane", Style: core.Style{FontWeight: core.FontWeightSemibold}},
					t.note("It takes the rest of the split."),
				}},
			},
		),
	}}
}
