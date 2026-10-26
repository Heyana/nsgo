package main

import (
	"github.com/oneo-me/nsgo/core"
)

// layoutDemo is the layout NSGo ports from Taffy: flexbox, wrapping,
// growing and shrinking, positioning, grid, intrinsic sizes, stacking,
// direction and block layout. NSH's gallery has no page of its own for
// it; this one is in its look.
func layoutDemo(cx *core.Scope) core.Node {
	box := func(label string, style core.Style) core.Node {
		s := fin.raised.over(core.Style{Background: colors.raised, BorderWidth: 1, BorderColor: colors.border, CornerRadius: 8,
			Padding: core.Symmetric(8, 12), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}).Merge(style)
		// A box of another color drops the raised gradient over it.
		if style.Background != 0 {
			s.Gradients = nil
		}
		return core.View{Style: s, Children: []core.Node{text(label, 13, medium, colors.text)}}
	}
	tinted := core.Style{Background: tints.accent, BorderColor: alpha(colors.accent, 0.4)}
	frame := func(style core.Style, children ...core.Node) core.Node {
		return core.View{Style: core.Style{Background: colors.surface, BorderWidth: 1, BorderColor: colors.line, CornerRadius: 10,
			Padding: core.All(8), Gap: 8}.Merge(style), Children: children}
	}

	rows := func(cx *core.Scope) core.Node {
		var justify []core.Node
		for _, j := range []struct {
			name string
			v    core.Justify
		}{{"JustifyStart", core.JustifyStart}, {"JustifyCenter", core.JustifyCenter}, {"JustifySpaceBetween", core.JustifySpaceBetween}, {"JustifySpaceEvenly", core.JustifySpaceEvenly}} {
			justify = append(justify, api(j.name),
				frame(core.Style{Direction: core.Row, JustifyContent: j.v}, box("A", core.Style{}), box("B", core.Style{}), box("C", core.Style{})))
		}
		justify = append(justify, api("Column, AlignCenter"),
			frame(core.Style{AlignItems: core.AlignCenter}, box("One", core.Style{}), box("Two, wider", core.Style{}), box("Three", core.Style{})))
		return section("Row and column", "Direction · JustifyContent", justify...)
	}

	wrapCard := func(cx *core.Scope) core.Node {
		var chips []core.Node
		for i, w := range []string{"flex", "wrap", "lays", "children", "onto", "more", "lines", "when", "they", "run", "out", "of", "room"} {
			s := core.Style{CornerRadius: 14}
			if i%2 == 1 {
				s = tinted.Merge(s)
			}
			chips = append(chips, box(w, s))
		}
		return section("Wrap", "Wrap", frame(core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 6}, chips...))
	}

	grow := func(cx *core.Scope) core.Node {
		return section("Grow and shrink", "FlexGrow · FlexShrink · AutoMargin",
			frame(core.Style{Direction: core.Row}, box("1", core.Style{FlexGrow: 1}), box("2", core.Style{FlexGrow: 2}), box("3", core.Style{FlexGrow: 3})),
			frame(core.Style{Direction: core.Row}, box("fixed 120", core.Style{Width: core.Pt(120)}), box("900 wide, shrinks to fit", core.Style{Width: core.Pt(900), FlexShrink: 1})),
			frame(core.Style{Direction: core.Row}, box("Start", core.Style{}), box("pushed to the end", core.Style{AutoMargin: core.SideLeft})),
		)
	}

	absolute := func(cx *core.Scope) core.Node {
		return section("Absolute positioning", "Position: Absolute",
			frame(core.Style{Height: core.Pt(120)},
				box("Top left", core.Style{Position: core.Absolute, Top: core.Pt(8), Left: core.Pt(8)}),
				box("Bottom right", core.Style{Position: core.Absolute, Bottom: core.Pt(8), Right: core.Pt(8)}),
				box("Centered", tinted.Merge(core.Style{Position: core.Absolute, Top: core.Pct(50), Left: core.Pct(50),
					Transform: core.Transform{TranslateXPercent: -50, TranslateYPercent: -50}})),
				core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(-6), Right: core.Pt(-6), Width: core.Pt(20), Height: core.Pt(20),
					CornerRadius: 10, Background: colors.danger, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
					Children: []core.Node{text("3", 11, bold, core.RGB(0xffffff))}},
			))
	}

	grid := func(cx *core.Scope) core.Node {
		var cells []core.Node
		for i := 1; i <= 7; i++ {
			cells = append(cells, box(itoa(i), core.Style{Height: core.Pt(44)}))
		}
		return section("Grid", "DisplayGrid",
			api(`GridTemplateColumns: "repeat(3, 1fr)"`),
			core.View{Style: core.Style{Display: core.DisplayGrid, GridTemplateColumns: "repeat(3, 1fr)", GridAutoRows: "44px", Gap: 8}, Children: []core.Node{
				box("span 2", tinted.Merge(core.Style{GridColumn: core.GridLines{Start: core.GridSpan(2)}})),
				box("B", core.Style{}), box("C", core.Style{}),
				box("2 rows", tinted.Merge(core.Style{GridRow: core.GridLines{Start: core.GridSpan(2)}})),
				box("E", core.Style{}), box("F", core.Style{}), box("G", core.Style{}), box("H", core.Style{}),
			}},
			api(`"repeat(auto-fill, minmax(80px, 1fr))"`),
			core.View{Style: core.Style{Display: core.DisplayGrid, GridTemplateColumns: "repeat(auto-fill, minmax(80px, 1fr))", Gap: 8}, Children: cells},
		)
	}

	positions := func(cx *core.Scope) core.Node {
		// Each group holds its rows, so its header stops at the group's
		// end.
		var groups []core.Node
		for _, name := range []string{"Fruit", "Vegetables", "Grains"} {
			g := []core.Node{core.View{Style: core.Style{Position: core.Sticky, Top: core.Pt(0), Background: colors.raised, Padding: core.Symmetric(6, 10),
				Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: colors.line}},
				Children: []core.Node{text(name, 13, semibold, colors.accent)}}}
			for i := 1; i <= 4; i++ {
				g = append(g, core.Text{Content: name + " " + itoa(i), Style: core.Style{Padding: core.Symmetric(7, 10), FontSize: 14, Color: colors.text}})
			}
			groups = append(groups, core.View{Children: g})
		}
		shown := core.NewSignal(cx, false)
		return section("Static, sticky and fixed", "Position",
			note("Static ignores insets and lets absolute views inside reach past it; Sticky pins a header while its group scrolls; Fixed places a view in the window, out of every scroll view."),
			frame(core.Style{Height: core.Pt(80)},
				core.View{Style: core.Style{Position: core.Static, Top: core.Pt(40), Padding: core.All(4)}, Children: []core.Node{
					box("absolute, in the frame", core.Style{Position: core.Absolute, Bottom: core.Pt(4), Right: core.Pt(4)}),
				}},
			),
			core.Scroll{Style: surfaceBox(160).Merge(core.Style{Overflow: core.Hidden}), Children: groups},
			button(buttonProps{label: "Show a fixed note", variant: secondary}, func() { toggled(shown) }),
			core.Show{When: shown.Get, Then: func(*core.Scope) core.Node {
				return core.View{Style: core.Style{Position: core.Fixed, Right: core.Pt(24), Bottom: core.Pt(24), Padding: core.Symmetric(10, 14),
					Background: colors.light, CornerRadius: radiusControl, ZIndex: 10, Shadows: fin.light.shadows},
					Children: []core.Node{text("Fixed to the window", 14, medium, colors.onLight)}}
			}},
		)
	}

	intrinsic := func(cx *core.Scope) core.Node {
		return section("Intrinsic sizes", "MinContent · MaxContent · FitContent",
			frame(core.Style{AlignItems: core.AlignStart},
				box("min-content: each word on a line", tinted.Merge(core.Style{Width: core.MinContent})),
				box("max-content: one line", tinted.Merge(core.Style{Width: core.MaxContent})),
				box("fit-content: one line up to the width, then it wraps as the space runs out", tinted.Merge(core.Style{Width: core.FitContent})),
			))
	}

	stacking := func(cx *core.Scope) core.Node {
		return section("Stacking", "ZIndex",
			frame(core.Style{Direction: core.Row, Height: core.Pt(90)},
				box("ZIndex 2", tinted.Merge(core.Style{Width: core.Pt(100), Height: core.Pt(60), ZIndex: 2})),
				box("0", core.Style{Width: core.Pt(100), Height: core.Pt(60), Margin: core.Edges{Left: -30, Top: 20}}),
				box("ZIndex 1", core.Style{Width: core.Pt(100), Height: core.Pt(60), Margin: core.Edges{Left: -30}, ZIndex: 1, Background: colors.inset}),
			))
	}

	direction := func(cx *core.Scope) core.Node {
		return section("Direction", "LayoutDirection · WritingDirection",
			frame(core.Style{Direction: core.Row, LayoutDirection: core.DirectionRTL}, box("first", core.Style{}), box("second", core.Style{}), box("third", core.Style{})),
			frame(core.Style{},
				core.Text{Content: "שלום עולם, hello world", Style: core.Style{WritingDirection: core.WritingAuto, FontSize: 15, Color: colors.text}},
				core.Text{Content: "Hello world, שלום עולם", Style: core.Style{WritingDirection: core.WritingAuto, FontSize: 15, Color: colors.text}},
			))
	}

	block := func(cx *core.Scope) core.Node {
		return section("Block", "DisplayBlock",
			frame(core.Style{Display: core.DisplayBlock},
				box("margin bottom 16", core.Style{Margin: core.Edges{Bottom: 16}}),
				box("margin top 16: the gap stays 16", core.Style{Margin: core.Edges{Top: 16}}),
				box("width 60%, centered by auto margins", core.Style{Width: core.Pct(60), AutoMargin: core.SidesHorizontal, Margin: core.Edges{Top: 8}}),
			))
	}

	return demoPage(cx, pageProps{}, rows, wrapCard, grow, absolute, grid, positions, intrinsic, stacking, direction, block)
}
