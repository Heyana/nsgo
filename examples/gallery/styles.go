package main

import (
	"fmt"
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

func stylesPage(cx *core.Scope, t *theme) core.Node {
	swatch := func(label string, style core.Style) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(96), Height: core.Pt(64), CornerRadius: radius, Background: t.sunken,
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.All(4)}.Merge(style),
			Children: []core.Node{core.Text{Content: label, Style: core.Style{FontSize: 12, TextAlign: core.TextAlignCenter}}}}
	}
	red, green, blue, amber := t.danger, t.success, t.accent, t.warning

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
				swatch("40% alpha", core.Style{Background: withAlpha(t.accent, 0.4)}),
				swatch("sunken", core.Style{}),
			),
		),
		t.demo("Corner radius", "CornerRadius rounds every corner; Overflow Hidden clips children to it.", flow(radii...)),
		shapesDemo(t, swatch),
		shadowsDemo(t, swatch),
		gradientsDemo(t, swatch),
		filtersDemo(t, swatch),
		transformsDemo(t, swatch),
		keyframesDemo(t, swatch),
		groupDemo(t),
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
				swatch("glow", core.Style{Background: t.accent, Color: t.onAccent, CornerRadius: 32, ShadowColor: withAlpha(t.accent, 0.6), ShadowRadius: 14}),
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
				base.Button{OnPress: func(core.PressEvent) { pressed.Update(func(n int) int { return n + 1 }) }, Label: "Hover or press me", Style: interactive,
					Children: []core.Node{core.Text{Content: "Hover or press me"}}},
				base.Button{OnPress: func(core.PressEvent) {}, Disabled: disabled.Get, Label: "Disabled", Style: t.button(),
					Children: []core.Node{core.Text{Content: "Disabled"}}},
				t.plain("Toggle disabled", func() { disabled.Update(func(d bool) bool { return !d }) }),
			),
			core.Input{Placeholder: "Focus me: the border turns accent", Style: t.field(), Label: "Focus demo"},
			t.live(func() string { return fmt.Sprintf("Pressed %d times", pressed.Get()) }),
		),
	}}
}

func shapesDemo(t *theme, swatch func(string, core.Style) core.Node) core.Node {
	surface := core.Style{Background: t.accentSoft}
	return t.demo("Corners and borders", "CornerRadii rounds each corner; BorderCurve Continuous eases corners into their edges; BorderStyle dashes, dots or removes the border.",
		flow(
			swatch("top corners", surface.Merge(core.Style{CornerRadii: core.Corners{TopLeft: 20, TopRight: 20}})),
			swatch("leaf", surface.Merge(core.Style{CornerRadii: core.Corners{TopLeft: 32, BottomRight: 32}})),
			swatch("circular 24", surface.Merge(core.Style{CornerRadius: 24})),
			swatch("continuous 24", surface.Merge(core.Style{CornerRadius: 24, BorderCurve: core.BorderCurveContinuous})),
			swatch("dashed", core.Style{Background: t.surface, BorderWidth: 2, BorderColor: t.accent, BorderStyle: core.BorderDashed}),
			swatch("dotted", core.Style{Background: t.surface, BorderWidth: 3, BorderColor: t.accent, BorderStyle: core.BorderDotted, CornerRadius: 16}),
			swatch("text color", core.Style{Background: t.surface, BorderWidth: 2, BorderColor: core.CurrentColor, Color: t.success}),
			swatch("border none", core.Style{Background: t.surface, BorderWidth: 6, BorderColor: t.danger, BorderStyle: core.BorderNone}),
		),
	)
}

func shadowsDemo(t *theme, swatch func(string, core.Style) core.Node) core.Node {
	card := core.Style{Background: t.surface}
	return t.demo("Layered shadows and outlines", "Shadows stacks CSS box shadows, inset ones inside the box; Ring draws Tailwind's rings; Outline* draws a line outside the border box.",
		flow(
			swatch("layered", card.Merge(core.Style{Shadows: []core.BoxShadow{
				{Offset: core.Point{Y: 1}, Blur: 2, Color: core.RGBA(0, 0.08)},
				{Offset: core.Point{Y: 8}, Blur: 24, Spread: -4, Color: core.RGBA(0, 0.18)}}})),
			swatch("inset", core.Style{Background: t.sunken, Shadows: []core.BoxShadow{
				{Offset: core.Point{Y: 2}, Blur: 6, Color: core.RGBA(0, 0.25), Inset: true}}}),
			swatch("ring", card.Merge(core.Style{Shadows: []core.BoxShadow{core.Ring(2, t.accent)}})),
			swatch("ring offset", card.Merge(core.Style{Shadows: []core.BoxShadow{core.Ring(3, t.background), core.Ring(5, t.accent)}})),
			swatch("outline", card.Merge(core.Style{OutlineWidth: 2, OutlineColor: t.accent, OutlineOffset: 3})),
			swatch("dashed outline", card.Merge(core.Style{OutlineWidth: 2, OutlineColor: t.success, OutlineStyle: core.BorderDashed})),
		),
	)
}

func gradientsDemo(t *theme, swatch func(string, core.Style) core.Node) core.Node {
	white := core.RGB(0xffffff)
	blue, yellow, pink := core.RGB(0x2563eb), core.RGB(0xfacc15), core.RGB(0xec4899)
	on := core.Style{Color: white}
	return t.demo("Gradients", "Gradients draws linear, radial and conic gradients over Background, mixed in Oklab unless a stop's Space says otherwise; a state's Background replaces them.",
		flow(
			swatch("linear 90°", on.Merge(core.Style{Gradients: []core.Gradient{core.LinearGradient(90, blue, pink)}})),
			swatch("to top right", on.Merge(core.Style{Gradients: []core.Gradient{{ToCorner: core.ToTopRight,
				Stops: []core.ColorStop{{Color: blue}, {Color: yellow}}}}})),
			swatch("Oklab", core.Style{Gradients: []core.Gradient{core.LinearGradient(90, blue, yellow)}}),
			swatch("sRGB", core.Style{Gradients: []core.Gradient{{Angle: 90, Space: core.ColorSpaceSRGB,
				Stops: []core.ColorStop{{Color: blue}, {Color: yellow}}}}}),
			swatch("radial", core.Style{Gradients: []core.Gradient{core.RadialGradient(yellow, pink, blue)}}),
			swatch("conic", on.Merge(core.Style{CornerRadius: 32, Gradients: []core.Gradient{core.ConicGradient(0, pink, yellow, blue, pink)}})),
			swatch("stripes", core.Style{Gradients: []core.Gradient{{Angle: 45, Repeating: true, Space: core.ColorSpaceSRGB,
				Stops: []core.ColorStop{{Color: t.sunken, At: core.Pt(0)}, {Color: t.sunken, At: core.Pt(8)},
					{Color: t.accentSoft, At: core.Pt(8)}, {Color: t.accentSoft, At: core.Pt(16)}}}}}),
			base.Button{Label: "Gradient button", OnPress: func(core.PressEvent) {},
				Style: core.Style{Padding: core.Symmetric(8, 16), CornerRadius: radius, Color: white, FontWeight: core.FontWeightMedium,
					Gradients: []core.Gradient{core.LinearGradient(180, core.RGB(0x3b82f6), core.RGB(0x1d4ed8))},
					Active:    &core.Style{Background: core.RGB(0x1e3a8a)}},
				Children: []core.Node{core.Text{Content: "Press me"}}},
		),
	)
}

func filtersDemo(t *theme, swatch func(string, core.Style) core.Node) core.Node {
	paint := []core.Gradient{core.LinearGradient(135, core.RGB(0xf43f5e), core.RGB(0xf59e0b), core.RGB(0x10b981), core.RGB(0x3b82f6))}
	art := func(label string, filters ...core.Filter) core.Node {
		return swatch(label, core.Style{Gradients: paint, Color: core.RGB(0xffffff), Filters: filters})
	}
	stripes := []core.Gradient{{Angle: 90, Repeating: true, Space: core.ColorSpaceSRGB, Stops: []core.ColorStop{
		{Color: t.accent, At: core.Pt(0)}, {Color: t.accent, At: core.Pt(6)}, {Color: t.surface, At: core.Pt(6)}, {Color: t.surface, At: core.Pt(12)}}}}
	return t.demo("Filters, backdrops and masks", "Filters change what a view draws; BackdropFilters what is behind it; Mask fades it by a gradient's alpha; BlendMode mixes it with what is behind.",
		flow(
			art("none"),
			art("grayscale", core.Grayscale(1)),
			art("sepia", core.Sepia(1)),
			art("hue 120°", core.HueRotate(120)),
			art("invert", core.Invert(1)),
			art("blur 4", core.Blur(4)),
			swatch("drop shadow", core.Style{Background: 0, Filters: []core.Filter{core.DropShadow(core.Point{X: 2, Y: 4}, 6, core.RGBA(0, 0.35))}}),
		),
		core.View{Style: wrap.Merge(core.Style{Padding: core.All(10), CornerRadius: radius, Gradients: stripes}),
			Children: []core.Node{
				swatch("frosted", core.Style{Background: withAlpha(t.surface, 0.4), BackdropFilters: []core.Filter{core.Blur(8)}}),
				swatch("glass", core.Style{Background: withAlpha(t.surface, 0.3), BackdropFilters: []core.Filter{core.Blur(12)},
					GlassEffect: core.GlassRegular, CornerRadius: 24}),
				swatch("masked", core.Style{Background: t.accent, Color: core.RGB(0xffffff),
					Mask: &core.Gradient{Angle: 90, Stops: []core.ColorStop{{Color: core.RGB(0)}, {Color: 0}}}}),
				swatch("multiply", core.Style{Background: core.RGB(0x60a5fa), BlendMode: core.BlendMultiply}),
				swatch("difference", core.Style{Background: core.RGB(0xffffff), BlendMode: core.BlendDifference}),
			}},
	)
}

func transformsDemo(t *theme, swatch func(string, core.Style) core.Node) core.Node {
	face := core.Style{Background: t.accentSoft}
	return t.demo("3D transforms", "RotateX and RotateY turn a view out of its plane, seen through a Perspective; SkewX slants it; TransformOrigin moves the point it turns around.",
		core.View{Style: wrap.Merge(core.Style{Perspective: 400}), Children: []core.Node{
			swatch("rotateY 45°", face.Merge(core.Style{Transform: core.Transform{RotateY: 45}})),
			swatch("rotateX 50°", face.Merge(core.Style{Transform: core.Transform{RotateX: 50}})),
			swatch("skewX 15°", face.Merge(core.Style{Transform: core.Transform{SkewX: 15}})),
			swatch("from top left", face.Merge(core.Style{Transform: core.Transform{Rotate: 12},
				TransformOrigin: core.ObjectPosition{X: core.PositionAxis{Offset: core.Pct(0)}, Y: core.PositionAxis{Offset: core.Pct(0)}}})),
			swatch("up 25%", face.Merge(core.Style{Transform: core.Transform{TranslateYPercent: -25}})),
			swatch("back hidden", face.Merge(core.Style{Transform: core.Transform{RotateY: 180}, BackfaceHidden: true})),
		}},
	)
}

func keyframesDemo(t *theme, swatch func(string, core.Style) core.Node) core.Node {
	dot := func(label string, a core.Keyframes, style core.Style) core.Node {
		return core.View{Style: core.Style{AlignItems: core.AlignCenter, Gap: 6, Width: core.Pt(72)}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(32), Height: core.Pt(32), Margin: core.Edges{Top: 8},
				Animation: core.KeyframeAnimation{Keyframes: a}}.Merge(style)},
			core.Text{Content: label, Style: t.caption()},
		}}
	}
	return t.demo("Keyframe animations", "Animation runs Tailwind's spin, ping, pulse and bounce keyframes on the platform, on top of the transform and opacity.",
		flow(
			dot("spin", core.KeyframesSpin, core.Style{BorderWidth: 4, BorderColor: t.sunken,
				Borders: core.All(4), BorderColors: core.EdgeColors{Top: t.accent, Right: t.sunken, Bottom: t.sunken, Left: t.sunken}, CornerRadius: 16}),
			dot("ping", core.KeyframesPing, core.Style{Background: t.accent, CornerRadius: 16}),
			dot("pulse", core.KeyframesPulse, core.Style{Background: t.accent, CornerRadius: 6}),
			dot("bounce", core.KeyframesBounce, core.Style{Background: t.accent, CornerRadius: 16}),
		),
	)
}

func groupDemo(t *theme) core.Node {
	card := func(title string) core.Node {
		return core.View{Style: core.Style{Group: "card", Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10,
			Padding: core.All(12), CornerRadius: radius, BorderWidth: 1, BorderColor: t.line, Width: core.Pt(220),
			Hover: &core.Style{BorderColor: t.accent}}, OnPress: func(core.PressEvent) {},
			Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(28), Height: core.Pt(28), CornerRadius: 14, Background: t.sunken,
					Transition: core.Transition{Duration: 150 * time.Millisecond},
					GroupStyle: &core.GroupStyle{Name: "card", Hover: &core.Style{Background: t.accent}, Active: &core.Style{Transform: core.Transform{Scale: 0.85}}}}},
				core.Text{Content: title, Style: core.Style{Color: t.muted,
					GroupStyle: &core.GroupStyle{Name: "card", Hover: &core.Style{Color: t.ink}}}},
			}}
	}
	return t.demo("Group styles", "GroupStyle styles a view by the states of its nearest Group: hover a card to light its dot and title.",
		flow(card("Hover this card"), card("Or this one")),
	)
}
