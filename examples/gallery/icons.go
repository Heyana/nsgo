package main

import (
	"fmt"
	"strings"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
)

// iconEntry is one icon of the catalog, which lucidegen writes.
type iconEntry struct {
	name string
	get  func() *core.SVG
}

func iconsPage(cx *core.Scope, t *theme) core.Node {
	query := core.NewSignal(cx, "")
	found := core.NewMemoFunc(cx, func() []iconEntry {
		q := strings.ToLower(strings.TrimSpace(query.Get()))
		if q == "" {
			return iconCatalog
		}
		var out []iconEntry
		for _, e := range iconCatalog {
			if strings.Contains(e.name, q) {
				out = append(out, e)
			}
		}
		return out
	}, nil)
	// The phone layout shows four icons a row, the desktop widths eight.
	cols := 8
	if core.Untracked(cx, func() core.Size { return cx.Env().Window() }).Width < wideWidth {
		cols = 4
	}
	glyph := func(svg *core.SVG, size core.Points, color core.Color, label string) core.Node {
		return core.Icon{SVG: svg, Label: label, Style: core.Style{FontSize: size, Color: color}}
	}
	sizes := func(svg *core.SVG, label string) core.Node {
		var out []core.Node
		for _, size := range []core.Points{14, 18, 24, 32, 48} {
			out = append(out, glyph(svg, size, t.ink, label))
		}
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignEnd, Gap: 16}, Children: out}
	}
	colors := func(svg *core.SVG, label string) core.Node {
		var out []core.Node
		for _, c := range []core.Color{t.ink, t.muted, t.accent, t.success, t.warning, t.danger} {
			out = append(out, glyph(svg, 24, c, label))
		}
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 16}, Children: out}
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Icons", "core.Icon draws an SVG in the text color it inherits: one document, sharp at any size.",
			flow(
				glyph(lucide.Search(), 24, t.ink, "Search"),
				glyph(lucide.Settings(), 24, t.ink, "Settings"),
				glyph(lucide.Heart(), 24, t.ink, "Heart"),
				glyph(lucide.House(), 24, t.ink, "Home"),
				glyph(lucide.Bell(), 24, t.ink, "Bell"),
				glyph(lucide.Camera(), 24, t.ink, "Camera"),
				glyph(lucide.Cloud(), 24, t.ink, "Cloud"),
				glyph(lucide.Rocket(), 24, t.ink, "Rocket"),
			),
		),
		t.demo("Sizes", "An icon is as high as the inherited font size unless its style sets one.",
			sizes(lucide.Sparkles(), "Sparkles"),
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
				glyph(lucide.ShieldCheck(), 20, t.success, "Shield"),
				core.Text{Content: "An icon in a line of text inherits that text's color and size.", Style: core.Style{FontSize: 17, Color: t.ink, FlexShrink: 1}},
				glyph(lucide.ArrowRight(), 17, t.ink, "Arrow"),
			}},
		),
		t.demo("Colors", "The inherited color is text color: a theme colors icons and labels alike.",
			colors(lucide.CircleCheck(), "Check"),
			colors(lucide.TriangleAlert(), "Warning"),
			core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
				iconButton(t, lucide.Copy(), "Copy", func() {}),
				iconButton(t, lucide.Download(), "Download", func() {}),
				iconButton(t, lucide.Trash(), "Delete", func() {}),
			}},
		),
		t.demo("All icons", "Every icon in the pinned Lucide release, filtered by name. Only the visible rows are built.",
			core.Input{Value: query.Get, OnChange: query.Set, Placeholder: "Search icons", Style: t.field(),
				Label: "Search icons", TestID: "icon-search"},
			t.live(func() string {
				n := len(found.Get())
				if n == 0 {
					return "No icon matches."
				}
				return fmt.Sprintf("%d of %d icons", n, len(iconCatalog))
			}),
			iconBrowser(t, found.Get, cols),
		),
	}}
}

// iconButton is a button whose child is an icon: base.Button presses, the
// icon only draws, so the button keeps the accessible label.
func iconButton(t *theme, svg *core.SVG, label string, onPress func()) core.Node {
	return base.Button{OnPress: onPress, Label: label, Children: []core.Node{
		core.Icon{SVG: svg, Style: core.Style{FontSize: 16}},
		core.Text{Content: label, Style: core.Style{FontSize: 14}},
	}, Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Padding: core.Symmetric(6, 12),
		CornerRadius: radius, Background: t.quiet, BorderWidth: 1, BorderColor: t.line, Color: t.ink,
		Hover: &core.Style{Background: t.quietHover}, Active: &core.Style{Background: t.quietPressed}}}
}

// iconBrowser lists the filtered icons in rows of cells, building only the
// rows near the viewport. A cell is built once and reads its row through
// signals, as List's cells expect.
func iconBrowser(t *theme, found func() []iconEntry, cols int) core.Node {
	at := func(row core.ListRow, col int) func() (iconEntry, bool) {
		return func() (iconEntry, bool) {
			n := row.Index()*cols + col
			all := found()
			if n >= len(all) {
				return iconEntry{}, false
			}
			return all[n], true
		}
	}
	return core.List{
		Count: func() int {
			return (len(found()) + cols - 1) / cols
		},
		EstimatedSize: 84,
		Style:         core.Style{Height: core.Pt(460), BorderWidth: 1, BorderColor: t.line, CornerRadius: radius, Overflow: core.Hidden},
		Row: func(cx *core.Scope, row core.ListRow) core.Node {
			cells := make([]core.Node, cols)
			for i := range cells {
				e := at(row, i)
				cells[i] = core.Show{
					When: func() bool {
						_, ok := e()
						return ok
					},
					Then: func(*core.Scope) core.Node {
						return core.View{Style: core.Style{Width: core.Pct(100 / float32(cols)), Height: core.Pt(80), Gap: 6,
							AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
							Children: []core.Node{
								core.Icon{
									SVGFunc: func() *core.SVG {
										entry, _ := e()
										return entry.get()
									},
									Style: core.Style{FontSize: 22, Color: t.ink},
								},
								core.Text{
									ContentFunc: func() string {
										entry, _ := e()
										return entry.name
									},
									MaxLines: 1,
									Style:    core.Style{FontSize: 10, Color: t.muted, TextAlign: core.TextAlignCenter},
								},
							}}
					},
				}
			}
			return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.Symmetric(0, 4)}, Children: cells}
		},
	}
}
