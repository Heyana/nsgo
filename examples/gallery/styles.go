package main

import (
	"fmt"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

func stylesPage(cx *core.Scope, t *theme) core.Node {
	swatch := func(label string, style core.Style) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(96), Height: core.Pt(64), CornerRadius: radius, Background: t.sunken,
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(4)}.Merge(style),
			Children: []core.Node{core.Text{Content: label, Style: core.Style{FontSize: 12, TextAlign: core.TextAlignCenter}}}}
	}
	red, green, blue, amber := core.RGB(0xe5534b), core.RGB(0x2da44e), core.RGB(0x2f81f7), core.RGB(0xd4a72c)

	var radii, opacities []core.Node
	for _, r := range []core.Points{0, 6, 16, 32} {
		radii = append(radii, swatch(fmt.Sprintf("radius %g", r), core.Style{CornerRadius: r, Background: t.accentSoft}))
	}
	for _, a := range []float32{1, 0.75, 0.5, 0.25} {
		opacities = append(opacities, swatch(fmt.Sprintf("opacity %g", a), core.Style{Background: t.accent, Color: t.onAccent,
			Opacity: core.Alpha(a)}))
	}

	pressed := core.NewSignal(cx, 0)
	disabled := core.NewSignal(cx, true)
	interactive := t.secondary().Merge(core.Style{
		Hover:  &core.Style{Background: t.accentSoft, BorderColor: t.accent},
		Active: &core.Style{Background: t.accent, Color: t.onAccent, Transform: core.Transform{Scale: 0.96}},
	})

	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Backgrounds", "Background takes an sRGB color, with alpha from RGBA.",
			flow(
				swatch("accent", core.Style{Background: t.accent, Color: t.onAccent}),
				swatch("success", core.Style{Background: t.success, Color: t.onAccent}),
				swatch("danger", core.Style{Background: t.danger, Color: t.onAccent}),
				swatch("40% alpha", core.Style{Background: core.RGBA(0x2f81f7, 0.4)}),
				swatch("sunken", core.Style{}),
			),
		),
		t.demo("Corner radius", "CornerRadius rounds every corner; Overflow Hidden clips children to it.", flow(radii...)),
		t.demo("Per-side borders", "Borders gives each side its own width and BorderColors its own color; corners still round.",
			flow(
				swatch("underline", core.Style{CornerRadius: 0,
					Borders: core.Edges{Bottom: 2}, BorderColors: core.EdgeColors{Bottom: t.accent}}),
				swatch("accent bar", core.Style{CornerRadius: 4,
					Borders: core.Edges{Left: 4}, BorderColor: t.accent}),
				swatch("four colors", core.Style{CornerRadius: 0, Background: t.surface,
					Borders: core.All(4), BorderColors: core.EdgeColors{Top: red, Right: green, Bottom: blue, Left: amber}}),
				swatch("four, rounded", core.Style{CornerRadius: 16, Background: t.surface,
					Borders: core.All(4), BorderColors: core.EdgeColors{Top: red, Right: green, Bottom: blue, Left: amber}}),
				swatch("widths 1 2 4 8", core.Style{CornerRadius: 10, Background: t.surface,
					Borders: core.Edges{Top: 1, Right: 2, Bottom: 4, Left: 8}, BorderColors: core.AllColors(t.muted)}),
				swatch("top and bottom", core.Style{CornerRadius: 0, Background: t.surface,
					Borders: core.Edges{Top: 1, Bottom: 1}, BorderColors: core.AllColors(t.line)}),
			),
			core.View{Style: core.Style{Padding: core.Symmetric(10, 14), Background: t.accentSoft, CornerRadius: 6,
				Borders: core.Edges{Left: 4}, BorderColors: core.EdgeColors{Left: t.accent}},
				Children: []core.Node{core.Text{Content: "A callout: a left accent bar over a tinted background, the usual way to set a note apart."}}},
		),
		t.demo("Shadows", "ShadowColor, ShadowRadius and ShadowOffset; a shadow follows the corners.",
			flow(
				swatch("soft", core.Style{Background: t.surface, ShadowColor: core.RGBA(0, 0.12), ShadowRadius: 6, ShadowOffset: core.Point{Y: 2}}),
				swatch("raised", core.Style{Background: t.surface, ShadowColor: core.RGBA(0, 0.2), ShadowRadius: 16, ShadowOffset: core.Point{Y: 8}}),
				swatch("glow", core.Style{Background: t.accent, Color: t.onAccent, CornerRadius: 32, ShadowColor: core.RGBA(0x2f81f7, 0.6), ShadowRadius: 14}),
			),
		),
		t.demo("Opacity", "Opacity fades a view and everything in it.", flow(opacities...)),
		t.demo("Transforms", "Transform moves, rotates and scales around the center without changing layout.",
			flow(
				swatch("rotate 12°", core.Style{Background: t.accentSoft, Transform: core.Transform{Rotate: 12}}),
				swatch("scale 0.8", core.Style{Background: t.accentSoft, Transform: core.Transform{Scale: 0.8}}),
				swatch("scaleX 1.2", core.Style{Background: t.accentSoft, Transform: core.Transform{ScaleX: 1.2}}),
				swatch("up 8", core.Style{Background: t.accentSoft, Transform: core.Transform{TranslateY: -8}}),
			),
		),
		t.demo("State styles", "Hover, Active, Focus and Disabled merge over the style while the view is in that state.",
			flow(
				base.Button{OnPress: func() { pressed.Update(func(n int) int { return n + 1 }) }, Label: "Hover or press me", Style: interactive,
					Children: []core.Node{core.Text{Content: "Hover or press me"}}},
				base.Button{OnPress: func() {}, Disabled: disabled.Get, Label: "Disabled", Style: t.button(),
					Children: []core.Node{core.Text{Content: "Disabled"}}},
				t.plain("Toggle disabled", func() { disabled.Update(func(d bool) bool { return !d }) }),
			),
			core.Input{Placeholder: "Focus me: the border turns accent", Style: t.field(), Label: "Focus demo"},
			t.live(func() string { return fmt.Sprintf("Pressed %d times", pressed.Get()) }),
		),
	}}
}
