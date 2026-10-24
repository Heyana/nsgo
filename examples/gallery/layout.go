package main

import (
	"fmt"

	"github.com/oneo-me/nsgo/core"
)

func positionDemo(cx *core.Scope, t *theme, box func(string, core.Style) core.Node, frame func(core.Style, ...core.Node) core.Node) core.Node {
	// Each section holds its rows, so its header stops at the section's end.
	var sections []core.Node
	for _, name := range []string{"Fruit", "Vegetables", "Grains"} {
		section := []core.Node{core.View{Style: core.Style{Position: core.Sticky, Top: core.Pt(0), Background: t.sunken,
			Padding: core.Symmetric(4, 8)}, Children: []core.Node{core.Text{Content: name, Style: core.Style{FontWeight: core.FontWeightSemibold}}}}}
		for i := 1; i <= 4; i++ {
			section = append(section, core.Text{Content: fmt.Sprintf("%s %d", name, i), Style: core.Style{Padding: core.Symmetric(6, 8)}})
		}
		sections = append(sections, core.View{Children: section})
	}
	shown := core.NewSignal(cx, false)
	return t.demo("Static, sticky and fixed", "Static ignores insets and lets absolute views inside reach past it; Sticky pins a header while its section scrolls; Fixed places a view in the window, out of every scroll view.",
		frame(core.Style{Height: core.Pt(80)},
			core.View{Style: core.Style{Position: core.Static, Top: core.Pt(40), Padding: core.All(4)}, Children: []core.Node{
				box("absolute, in the frame", core.Style{Position: core.Absolute, Bottom: core.Pt(4), Right: core.Pt(4)}),
			}},
		),
		core.Scroll{Style: core.Style{Height: core.Pt(160), BorderWidth: 1, BorderColor: t.line, CornerRadius: radius}, Children: sections},
		t.plain("Show a fixed note", func() { shown.Update(func(b bool) bool { return !b }) }),
		core.Show{When: shown.Get, Then: func(*core.Scope) core.Node {
			return core.View{Style: core.Style{Position: core.Fixed, Right: core.Pt(24), Bottom: core.Pt(24), Padding: core.Symmetric(10, 14),
				Background: t.ink, Color: t.background, CornerRadius: radius, ZIndex: 10},
				Children: []core.Node{core.Text{Content: "Fixed to the window"}}}
		}},
	)
}

func layoutPage(cx *core.Scope, t *theme) core.Node {
	box := func(label string, style core.Style) core.Node {
		return core.View{Style: t.tile().Merge(style), Children: []core.Node{core.Text{Content: label, Style: core.Style{FontSize: 13}}}}
	}
	frame := func(style core.Style, children ...core.Node) core.Node {
		return core.View{Style: core.Style{Background: t.background, BorderWidth: 1, BorderColor: t.line, CornerRadius: radius,
			Padding: core.All(8), Gap: 8}.Merge(style), Children: children}
	}

	var justify []core.Node
	for _, j := range []struct {
		name string
		v    core.Justify
	}{{"Start", core.JustifyStart}, {"Center", core.JustifyCenter}, {"SpaceBetween", core.JustifySpaceBetween}, {"SpaceEvenly", core.JustifySpaceEvenly}} {
		justify = append(justify, t.note(j.name),
			frame(core.Style{Direction: core.Row, JustifyContent: j.v}, box("A", core.Style{}), box("B", core.Style{}), box("C", core.Style{})))
	}

	var chips []core.Node
	for i, w := range []string{"flex", "wrap", "lays", "children", "onto", "more", "lines", "when", "they", "run", "out", "of", "room"} {
		chips = append(chips, box(w, core.Style{CornerRadius: 14, Background: []core.Color{t.sunken, t.accentSoft}[i%2]}))
	}

	var cells []core.Node
	for i := 1; i <= 7; i++ {
		cells = append(cells, box(fmt.Sprint(i), core.Style{Height: core.Pt(44)}))
	}

	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Row and column", "Direction sets the main axis; JustifyContent spaces children along it.",
			append(justify,
				t.note("Column, AlignItems center"),
				frame(core.Style{AlignItems: core.AlignCenter}, box("One", core.Style{}), box("Two, wider", core.Style{}), box("Three", core.Style{})),
			)...,
		),
		t.demo("Wrap", "Wrap moves children that do not fit onto another line.",
			frame(core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 6}, chips...),
		),
		t.demo("Grow and shrink", "FlexGrow shares free space by weight, FlexShrink gives up space when there is too little, and auto margins take what is left.",
			frame(core.Style{Direction: core.Row},
				box("1", core.Style{FlexGrow: 1}), box("2", core.Style{FlexGrow: 2}), box("3", core.Style{FlexGrow: 3})),
			frame(core.Style{Direction: core.Row},
				box("fixed 120", core.Style{Width: core.Pt(120)}),
				box("900 wide, shrinks to fit", core.Style{Width: core.Pt(900), FlexShrink: 1})),
			frame(core.Style{Direction: core.Row},
				box("Start", core.Style{}), box("pushed to the end by an auto margin", core.Style{AutoMargin: core.SideLeft})),
		),
		t.demo("Absolute positioning", "Position Absolute takes a node out of the flow, placed by Top, Right, Bottom and Left.",
			frame(core.Style{Height: core.Pt(120)},
				box("Top left", core.Style{Position: core.Absolute, Top: core.Pt(8), Left: core.Pt(8)}),
				box("Bottom right", core.Style{Position: core.Absolute, Bottom: core.Pt(8), Right: core.Pt(8)}),
				box("Centered", core.Style{Position: core.Absolute, Top: core.Pct(50), Left: core.Pct(50),
					Transform: core.Transform{TranslateX: -40, TranslateY: -16}, Width: core.Pt(80), Height: core.Pt(32)}),
				core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(-6), Right: core.Pt(-6), Width: core.Pt(20), Height: core.Pt(20),
					CornerRadius: 10, Background: t.danger, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
					Children: []core.Node{core.Text{Content: "3", Style: core.Style{Color: t.onAccent, FontSize: 11, FontWeight: core.FontWeightBold}}}},
			),
		),
		t.demo("Grid", "DisplayGrid with GridTemplateColumns; GridColumn and GridRow place and span items.",
			t.note(`GridTemplateColumns: "repeat(3, 1fr)", the first item spanning two columns and the fourth two rows`),
			core.View{Style: core.Style{Display: core.DisplayGrid, GridTemplateColumns: "repeat(3, 1fr)", GridAutoRows: "44px", Gap: 8},
				Children: []core.Node{
					box("span 2", core.Style{GridColumn: core.GridLines{Start: core.GridSpan(2)}, Background: t.accentSoft}),
					box("B", core.Style{}), box("C", core.Style{}),
					box("2 rows", core.Style{GridRow: core.GridLines{Start: core.GridSpan(2)}, Background: t.accentSoft}),
					box("E", core.Style{}), box("F", core.Style{}), box("G", core.Style{}), box("H", core.Style{}),
				}},
			t.note(`"repeat(auto-fill, minmax(80px, 1fr))" fits as many columns as the width allows`),
			core.View{Style: core.Style{Display: core.DisplayGrid, GridTemplateColumns: "repeat(auto-fill, minmax(80px, 1fr))", Gap: 8},
				Children: cells},
		),
		positionDemo(cx, t, box, frame),
		t.demo("Intrinsic sizes", "MinContent fits the longest word, MaxContent the whole line, FitContent the line up to the space there is.",
			frame(core.Style{AlignItems: core.AlignStart},
				box("min-content: each word on a line", core.Style{Width: core.MinContent, Background: t.accentSoft}),
				box("max-content: one line", core.Style{Width: core.MaxContent, Background: t.accentSoft}),
				box("fit-content: one line up to the width, then it wraps as the space runs out", core.Style{Width: core.FitContent, Background: t.accentSoft}),
			),
		),
		t.demo("Stacking", "ZIndex raises a view above its siblings without moving it in layout.",
			frame(core.Style{Direction: core.Row, Height: core.Pt(90)},
				box("ZIndex 2", core.Style{Width: core.Pt(100), Height: core.Pt(60), ZIndex: 2, Background: t.accentSoft, BorderWidth: 1, BorderColor: t.accent}),
				box("0", core.Style{Width: core.Pt(100), Height: core.Pt(60), Margin: core.Edges{Left: -30, Top: 20}, Background: t.surface, BorderWidth: 1, BorderColor: t.line}),
				box("ZIndex 1", core.Style{Width: core.Pt(100), Height: core.Pt(60), Margin: core.Edges{Left: -30}, ZIndex: 1, Background: t.sunken, BorderWidth: 1, BorderColor: t.borderStrong}),
			),
		),
		t.demo("Direction", "LayoutDirection lays a subtree out right to left; WritingDirection Auto takes a text's direction from its first letter.",
			frame(core.Style{Direction: core.Row, LayoutDirection: core.DirectionRTL},
				box("first", core.Style{}), box("second", core.Style{}), box("third", core.Style{})),
			frame(core.Style{},
				core.Text{Content: "שלום עולם, hello world", Style: core.Style{WritingDirection: core.WritingAuto}},
				core.Text{Content: "Hello world, שלום עולם", Style: core.Style{WritingDirection: core.WritingAuto}},
			),
		),
		t.demo("Block", "DisplayBlock stacks children as CSS block boxes; vertical margins between them collapse.",
			frame(core.Style{Display: core.DisplayBlock},
				box("margin bottom 16", core.Style{Margin: core.Edges{Bottom: 16}}),
				box("margin top 16: the gap stays 16", core.Style{Margin: core.Edges{Top: 16}}),
				box("width 60%, centered by auto margins", core.Style{Width: core.Pct(60), AutoMargin: core.SidesHorizontal, Margin: core.Edges{Top: 8}}),
			),
		),
	}}
}
