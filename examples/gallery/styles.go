package main

import (
	"time"

	"github.com/oneo-me/nsgo/core"
)

// specimenStage is the stage every specimen stands on: the card's light,
// without its drop shadow.
func specimenStage() core.Style {
	hl := inner(0, 1, 0, core.RGB(0xffffff))
	if isDark {
		hl = inner(0, 1, 0, white(0.05))
	}
	return core.Style{Height: core.Pt(104), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, CornerRadius: radiusControl,
		BorderCurve: core.BorderCurveContinuous, Background: colors.surface, BorderWidth: 1, BorderColor: colors.line,
		Gradients: fin.card.gradients, Shadows: []core.BoxShadow{hl}}
}

// specimen is a box the styles are tried on.
func specimen(s core.Style) core.Style {
	return core.Style{Width: core.Pt(112), Height: core.Pt(76), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: colors.raised}.Merge(s)
}

func specimenLabel(s string, c core.Color) core.Node { return text(s, 13, semibold, c) }

// tileProps are a specimen tile's texts: a label, the style it shows as
// code and a plain note.
type tileProps struct {
	label, code, note string
	labelFn           func() string
	onPress           func()
}

// tile is one cell of a two-column grid: the specimen on a stage and its
// texts.
func tile(p tileProps, children ...core.Node) core.Node {
	st := core.View{Style: specimenStage(), Children: children}
	if p.onPress != nil {
		st.OnPress = func(core.PressEvent) { p.onPress() }
		st.Role = core.RoleButton
		st.Label = p.label
	}
	lbl := p.labelFn
	if lbl == nil {
		lbl = label(p.label)
	}
	texts := []core.Node{textf(lbl, 13, medium, colors.text)}
	if p.code != "" {
		texts = append(texts, core.Text{Content: p.code, MaxLines: 1, Style: core.Style{FontSize: 12, Color: colors.muted, FontFamily: mono}})
	}
	if p.note != "" {
		texts = append(texts, core.Text{Content: p.note, MaxLines: 1, Style: core.Style{FontSize: 12, Color: colors.faint}})
	}
	return core.View{Style: core.Style{FlexBasis: core.Pct(40), FlexGrow: 1, Gap: 8}, Children: []core.Node{
		st, core.View{Style: core.Style{Gap: 3, Padding: core.Symmetric(0, 2)}, Children: texts},
	}}
}

func grid(children ...core.Node) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, ColumnGap: 12, RowGap: 16}, Children: children}
}

func cornersCard(cx *core.Scope) core.Node {
	shapes := []struct {
		label string
		style core.Style
	}{
		{"Circular", core.Style{Width: core.Pt(60), Height: core.Pt(60), CornerRadius: 20, Background: colors.info}},
		{"Continuous", core.Style{Width: core.Pt(60), Height: core.Pt(60), CornerRadius: 20, BorderCurve: core.BorderCurveContinuous, Background: colors.info}},
		{"One corner", core.Style{Width: core.Pt(60), Height: core.Pt(60), CornerRadii: core.Corners{TopLeft: 4, TopRight: 20, BottomRight: 20, BottomLeft: 20},
			BorderCurve: core.BorderCurveContinuous, BorderWidth: 2.5, BorderColor: colors.accent}},
		{"Pill", core.Style{Width: core.Pt(60), Height: core.Pt(28), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous, Background: colors.up}},
	}
	var row []core.Node
	for _, s := range shapes {
		row = append(row, core.View{Style: core.Style{AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
			core.View{Style: core.Style{Height: core.Pt(64), JustifyContent: core.JustifyCenter}, Children: []core.Node{core.View{Style: s.style}}},
			text(s.label, 12, regular, colors.muted),
		}})
	}
	return card(cardProps{icon: iconStyles, title: "Corners", subtitle: "CornerRadius · BorderCurve · per corner"},
		core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween}, Children: row},
		caption("Continuous corners ease into the edges", label("Radius 20 pt")),
		grid(
			tile(tileProps{label: "Top corners", code: "CornerRadii", note: "28 pt"},
				core.View{Style: specimen(core.Style{CornerRadii: core.Corners{TopLeft: 28, TopRight: 28}, BorderWidth: 1.5, BorderColor: colors.accent}),
					Children: []core.Node{specimenLabel("28 pt", colors.accent)}}),
			tile(tileProps{label: "Mixed radii", note: "44 · 6 · 28 · 0 pt, clockwise"},
				core.View{Style: specimen(core.Style{Background: tints.info, CornerRadii: core.Corners{TopLeft: 44, TopRight: 6, BottomRight: 28}, BorderWidth: 2, BorderColor: colors.info}),
					Children: []core.Node{specimenLabel("Mixed", colors.info)}}),
		),
	)
}

func bordersCard(cx *core.Scope) core.Node {
	return card(cardProps{icon: iconComponents, title: "Borders", subtitle: "Per side · dashed · dotted · outlines"},
		grid(
			tile(tileProps{label: "Per-side borders", note: "6 · 2 · 4 · 1 pt, clockwise"},
				core.View{Style: specimen(core.Style{Borders: core.Edges{Top: 6, Right: 2, Bottom: 4, Left: 1},
					BorderColors: core.EdgeColors{Top: colors.danger, Right: colors.up, Bottom: colors.info, Left: colors.warn}})}),
			tile(tileProps{label: "Dashed / dotted", code: "BorderStyle"},
				core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					core.View{Style: core.Style{Width: core.Pt(112), Height: core.Pt(28), CornerRadius: 8, BorderWidth: 1.5, BorderColor: colors.info, BorderStyle: core.BorderDashed}},
					core.View{Style: core.Style{Width: core.Pt(112), Height: core.Pt(28), CornerRadius: 8, BorderWidth: 3, BorderColor: colors.warn, BorderStyle: core.BorderDotted}},
				}}),
			tile(tileProps{label: "Rings", code: "Ring", note: "Box shadows of no blur"},
				core.View{Style: specimen(core.Style{CornerRadius: 14, Shadows: []core.BoxShadow{core.Ring(3, colors.surface), core.Ring(5, colors.accent)}})}),
			tile(tileProps{label: "Outline", code: "OutlineOffset", note: "Outside the border box"},
				core.View{Style: specimen(core.Style{CornerRadius: 14, OutlineWidth: 2, OutlineColor: colors.info, OutlineOffset: 4, OutlineStyle: core.BorderDashed})}),
		),
	)
}

// shadowsCard is shadows that read on a dark page: colored glows rather
// than black.
func shadowsCard(cx *core.Scope) core.Node {
	offsetBg := core.RGB(0xe7edfb)
	shade := float32(0.22)
	if isDark {
		offsetBg, shade = core.RGB(0x1d2740), 0.6
	}
	return card(cardProps{icon: iconSpark, title: "Shadows and glow", subtitle: "ShadowColor · box shadows · text shadow"},
		grid(
			tile(tileProps{label: "Glow", code: "ShadowColor", note: "A colored shadow"},
				core.View{Style: specimen(core.Style{CornerRadius: 16, BorderCurve: core.BorderCurveContinuous, Background: colors.accentFill,
					ShadowColor: alpha(colors.accentFill, 0.55), ShadowRadius: 9, ShadowOffset: core.Point{Y: 6}}), Children: []core.Node{specimenLabel("Glow", colors.onAccent)}}),
			tile(tileProps{label: "Offset shadow", code: "ShadowOffset", note: "4, 8 pt · radius 10"},
				core.View{Style: specimen(core.Style{Background: offsetBg, CornerRadii: core.Corners{TopLeft: 30, BottomRight: 30}, BorderWidth: 1, BorderColor: colors.info,
					ShadowColor: alpha(colors.info, 0.45), ShadowRadius: 5, ShadowOffset: core.Point{X: 4, Y: 8}}), Children: []core.Node{specimenLabel("Offset", colors.info)}}),
			tile(tileProps{label: "Image corners", note: "Two radii and a shadow"},
				core.Image{Source: core.Src("https://picsum.photos/seed/nsh/300/220"), Style: core.Style{Width: core.Pt(112), Height: core.Pt(76),
					CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28}, Background: colors.raised, ShadowColor: black(shade), ShadowRadius: 6, ShadowOffset: core.Point{Y: 8}}}),
			tile(tileProps{label: "Text shadow", code: "TextShadowColor"},
				core.Text{Content: "Shadow", Style: core.Style{FontSize: 24, FontWeight: core.FontWeight(800), Color: colors.text, LetterSpacing: -0.5,
					TextShadowColor: alpha(colors.danger, 0.9), TextShadowRadius: 4}}),
			tile(tileProps{label: "Text corners", note: "A span's background and radius"},
				core.Text{Spans: []core.Span{{Content: " Text corners ", Style: core.Style{Background: tints.up, CornerRadius: 10}}},
					Style: core.Style{FontSize: 15, FontWeight: semibold, Color: colors.up, LineHeight: 30}}),
			tile(tileProps{label: "Layered", code: "Shadows", note: "Two box shadows and an inset one"},
				core.View{Style: specimen(core.Style{CornerRadius: 16, Background: colors.surface, Shadows: []core.BoxShadow{
					inner(0, -2, 6, alpha(colors.accent, 0.35)), shadow(0, 1, 2, 0, black(0.08)), shadow(0, 10, 24, -4, black(shade))}}),
					Children: []core.Node{specimenLabel("Layered", colors.text)}}),
		),
	)
}

// layersCard is clipping, stacking order and opacity, each a press away.
func layersCard(cx *core.Scope) core.Node {
	clip := core.NewSignal(cx, true)
	front := core.NewSignal(cx, 0)
	opacity := core.NewAnimatedValue(cx, 0.45)
	level := core.NewSignal(cx, "0.45")
	stack := []core.Color{colors.danger, colors.info, colors.up}
	var cards []core.Node
	shade := float32(0.18)
	if isDark {
		shade = 0.5
	}
	for i, c := range stack {
		cards = append(cards, core.View{StyleFunc: func() core.Style {
			s := core.Style{Position: core.Absolute, Left: core.Pt(4 + core.Points(i)*28), Top: core.Pt(2 + core.Points(i)*14), Width: core.Pt(64), Height: core.Pt(52),
				CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, Background: c, ShadowColor: black(shade), ShadowRadius: 3, ShadowOffset: core.Point{Y: 3}}
			if front.Get() == i {
				s.BorderWidth, s.BorderColor, s.ZIndex = 2, colors.text, 10
			}
			return s
		}})
	}
	onBlue := core.RGB(0xffffff)
	if isDark {
		onBlue = core.RGB(0x0c1020)
	}
	return card(cardProps{icon: iconLists, title: "Clipping and layers", subtitle: "Overflow · ZIndex · Opacity", right: toggle(clip.Get, clip.Set, "Clip")},
		grid(
			tile(tileProps{labelFn: func() string {
				if clip.Get() {
					return "Clipped"
				}
				return "Visible"
			}, code: "Overflow: Hidden / Visible", note: hint(cx, "Tap to switch", "Click to switch"), onPress: func() { toggled(clip) }},
				core.View{StyleFunc: func() core.Style {
					o := core.Hidden
					if !clip.Get() {
						o = core.Visible
					}
					return specimen(core.Style{Background: tints.warn, CornerRadius: 22, BorderWidth: 1.5, BorderColor: colors.warn, Overflow: o})
				}, Children: []core.Node{
					core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(-26), Top: core.Pt(-26), Width: core.Pt(72), Height: core.Pt(72), CornerRadius: 36, Background: colors.danger}},
					specimenLabel(hint(cx, "Tap", "Click"), colors.warn),
				}}),
			tile(tileProps{label: "Stacking order", code: "ZIndex", note: hint(cx, "Tap to raise the next", "Click to raise the next"), onPress: func() { front.Update(func(n int) int { return (n + 1) % 3 }) }},
				core.View{Style: core.Style{Width: core.Pt(130), Height: core.Pt(84)}, Children: cards}),
		),
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(64), Height: core.Pt(64), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous, Background: colors.info,
				AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Animated: &core.AnimatedStyle{Opacity: opacity}},
				Children: []core.Node{textf(level.Get, 13, bold, onBlue)}},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 8}, Children: []core.Node{
				caption("Opacity", label("Animated natively")),
				segmented([]option[string]{{value: "0.2", label: "0.2"}, {value: "0.45", label: "0.45"}, {value: "1", label: "1"}}, level.Get, func(v string) {
					level.Set(v)
					to := map[string]float64{"0.2": 0.2, "0.45": 0.45, "1": 1}[v]
					timing(cx, opacity, to, core.TimingConfig{Duration: ms(260), Easing: easeOutCubic})
				}, false),
			}},
		}},
	)
}

// flipCard is two faces back to back: each hides while turned away.
func flipCard(cx *core.Scope) core.Node {
	angle := core.NewAnimatedValue(cx, 0)
	flipped := false
	flip := func() {
		flipped = !flipped
		to := 0.0
		if flipped {
			to = 180
		}
		spring(cx, angle, to, core.SpringConfig{Stiffness: 140, Damping: 16})
	}
	face := func(bg, from, to, border core.Color, rotate core.AnimatedNode, badgeText string, t tone, degrees, title, body string) core.Node {
		sh := []core.BoxShadow{inner(0, 1, 0, core.RGB(0xffffff)), shadow(0, 8, 20, 0, paper(0.10))}
		if isDark {
			sh = []core.BoxShadow{inner(0, 1, 0, white(0.07)), shadow(0, 10, 24, 0, black(0.4))}
		}
		return core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), CornerRadius: radiusCard,
			BorderCurve: core.BorderCurveContinuous, BorderWidth: 1, BorderColor: border, Padding: core.All(16), JustifyContent: core.JustifySpaceBetween, BackfaceHidden: true,
			Background: bg, Gradients: []core.Gradient{core.LinearGradient(180, from, to)}, Shadows: sh,
			Transform: core.Transform{Perspective: 800}, Animated: &core.AnimatedStyle{RotateY: rotate}}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween}, Children: []core.Node{badge(badgeText, t), text(degrees, 13, regular, colors.muted)}},
			core.View{Style: core.Style{Gap: 2}, Children: []core.Node{text(title, 20, semibold, colors.text), text(body, 13, regular, colors.muted)}},
		}}
	}
	front := face(core.RGB(0xeef2fc), core.RGB(0xf0f4fd), core.RGB(0xebf0fb), core.RGB(0xcdd8f3), angle, "front", toneInfo, "0°", hint(cx, "Tap to flip", "Click to flip"), "A spring turns it on the UI thread")
	back := face(core.RGB(0xfbefe6), core.RGB(0xfcf2ea), core.RGB(0xfaebe0), core.RGB(0xf0cdb3), core.Add(angle, core.Constant(180)), "back", toneAccent, "180°", "BackfaceHidden", "Each face hides while turned away")
	if isDark {
		front = face(core.RGB(0x1a2134), core.RGB(0x1e263b), core.RGB(0x1a2134), core.RGB(0x2b3a5c), angle, "front", toneInfo, "0°", hint(cx, "Tap to flip", "Click to flip"), "A spring turns it on the UI thread")
		back = face(core.RGB(0x2a1d14), core.RGB(0x2f2116), core.RGB(0x291c13), core.RGB(0x4e3423), core.Add(angle, core.Constant(180)), "back", toneAccent, "180°", "BackfaceHidden", "Each face hides while turned away")
	}
	return press(pressProps{onPress: flip, scale: 0.98, label: "Flip the card"}, core.View{Style: core.Style{Height: core.Pt(132)}, Children: []core.Node{front, back}})
}

func transformsCard(cx *core.Scope) core.Node {
	// A slow sway on the platform, so the perspective reads as depth.
	sway := core.NewAnimatedValue(cx, -30)
	sine := ease(core.EasingSin, core.EaseModeInOut)
	run := cx.StartAnimation(core.Loop(core.Sequence(
		core.Timing(sway, 30, core.TimingConfig{Duration: ms(2200), Easing: sine}),
		core.Timing(sway, -30, core.TimingConfig{Duration: ms(2200), Easing: sine}),
	), 0), nil)
	cx.Cleanup(run.Stop)
	return card(cardProps{icon: iconPhysics, title: "3D transforms", subtitle: "Perspective · RotateX/Y · Skew · origin"},
		flipCard(cx),
		grid(
			tile(tileProps{label: "Perspective", code: "RotateY", note: "Swaying, driven natively"},
				core.View{Style: specimen(core.Style{CornerRadius: 14, Background: tints.info, BorderWidth: 1, BorderColor: colors.info,
					Transform: core.Transform{Perspective: 400}, Animated: &core.AnimatedStyle{RotateY: sway}}), Children: []core.Node{specimenLabel("Y axis", colors.info)}}),
			tile(tileProps{label: "Tilt", code: "RotateX: 40"},
				core.View{Style: specimen(core.Style{CornerRadius: 14, Background: tints.accent, BorderWidth: 1, BorderColor: colors.accent,
					Transform: core.Transform{Perspective: 400, RotateX: 40}}), Children: []core.Node{specimenLabel("X axis", colors.accent)}}),
			tile(tileProps{label: "Skew", code: "SkewX: -12"},
				core.View{Style: specimen(core.Style{CornerRadius: 10, Background: tints.warn, BorderWidth: 1, BorderColor: colors.warn,
					Transform: core.Transform{SkewX: -12}}), Children: []core.Node{specimenLabel("Skew", colors.warn)}}),
			tile(tileProps{label: "Origin", code: "TransformOrigin", note: "Top left · rotated 30°"},
				core.View{Style: core.Style{Width: core.Pt(112), Height: core.Pt(84)}, Children: []core.Node{
					core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(30), Top: core.Pt(14), Width: core.Pt(54), Height: core.Pt(54), CornerRadius: 6,
						BorderWidth: 1, BorderColor: colors.faint, BorderStyle: core.BorderDashed}},
					core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(30), Top: core.Pt(14), Width: core.Pt(54), Height: core.Pt(54), CornerRadius: 6,
						Background: alpha(colors.danger, 0.55), Transform: core.Transform{Rotate: 30},
						TransformOrigin: core.ObjectPosition{X: core.PositionAxis{Offset: core.Pct(0)}, Y: core.PositionAxis{Offset: core.Pct(0)}}}},
					core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(26), Top: core.Pt(10), Width: core.Pt(8), Height: core.Pt(8), CornerRadius: 4, Background: colors.text}},
				}}),
		),
	)
}

// rightToLeft is rows, start margins and text under a right-to-left
// layout: the app's language decides it, and LayoutDirection sets a
// view's own.
func rightToLeft(cx *core.Scope) core.Node {
	rtl := core.NewSignal(cx, false)
	tones := []core.Color{colors.danger, colors.info, colors.up}
	onTile := core.RGB(0xffffff)
	if isDark {
		onTile = core.RGB(0x151412)
	}
	var boxes []core.Node
	for i, c := range tones {
		s := core.Style{Width: core.Pt(44), Height: core.Pt(44), CornerRadius: 10, CornerRadii: core.Corners{TopLeft: 22, TopRight: 10, BottomRight: 10, BottomLeft: 10},
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: c}
		if i == 0 {
			s.Margin = core.Edges{Left: 16}
		}
		boxes = append(boxes, core.View{Style: s, Children: []core.Node{text(itoa(i+1), 15, bold, onTile)}})
	}
	chip := func(c, bg core.Color, n int) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(40), Height: core.Pt(40), CornerRadius: 10, BorderCurve: core.BorderCurveContinuous,
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: bg}, Children: []core.Node{text(itoa(n), 13, regular, c)}}
	}
	var chips []core.Node
	for n := 1; n <= 12; n++ {
		chips = append(chips, chip(colors.info, tints.info, n))
	}
	monoNote := func(s string) func() string { return label(s) }
	return card(cardProps{icon: iconNav, title: "Right to left", subtitle: "", right: button(buttonProps{label: "Flip", variant: secondary, small: true}, func() { toggled(rtl) })},
		core.View{StyleFunc: func() core.Style {
			d := core.DirectionLTR
			if rtl.Get() {
				d = core.DirectionRTL
			}
			return core.Style{LayoutDirection: d, Gap: 12}
		}, Children: []core.Node{
			captionf(func() string {
				if rtl.Get() {
					return "LayoutDirection: RTL"
				}
				return "LayoutDirection: LTR"
			}),
			core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: boxes},
			core.Text{Content: "Text aligns to the start side; the first box has a start margin of 16 and a round top-start corner.",
				Style: core.Style{FontSize: 15, LineHeight: 21, Color: colors.text}},
			caption("Horizontal scroll view", monoNote("Scroll")),
			core.Scroll{Horizontal: true, HideIndicator: true, Style: core.Style{Height: core.Pt(40)}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, Gap: 6}, Children: chips}}},
			caption("Horizontal list", monoNote("List")),
			core.List{Count: func() int { return 30 }, Horizontal: true, EstimatedSize: 46, HideIndicator: true, Gap: 6, Style: core.Style{Height: core.Pt(40)},
				Row: func(cx *core.Scope, r core.ListRow) core.Node {
					return core.View{Style: core.Style{Width: core.Pt(40), Height: core.Pt(40), CornerRadius: 10, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: tints.accent},
						Children: []core.Node{textf(func() string { return itoa(r.Index() + 1) }, 13, regular, colors.accent)}}
				}},
			core.View{Style: core.Style{Height: core.Pt(1), Background: colors.line}},
			core.Text{Content: "TextAlignStart: the start side, right to left", Style: core.Style{FontSize: 13, TextAlign: core.TextAlignStart, Color: colors.muted}},
			core.Text{Content: "مرحبا بالعالم — WritingDirection RTL", Style: core.Style{FontSize: 13, WritingDirection: core.WritingRTL, Color: colors.muted}},
		}},
	)
}

func gradientsCard(cx *core.Scope) core.Node {
	blue, yellow, pink := core.RGB(0x2563eb), core.RGB(0xfacc15), core.RGB(0xec4899)
	return card(cardProps{icon: iconStyles, title: "Gradients", subtitle: "Linear, radial and conic, mixed in Oklab", right: badge("NSGo", toneNeutral)},
		grid(
			tile(tileProps{label: "Linear", code: "LinearGradient(90, …)"}, core.View{Style: specimen(core.Style{CornerRadius: 14, Gradients: []core.Gradient{core.LinearGradient(90, blue, pink)}})}),
			tile(tileProps{label: "To a corner", code: "ToTopRight"}, core.View{Style: specimen(core.Style{CornerRadius: 14,
				Gradients: []core.Gradient{{ToCorner: core.ToTopRight, Stops: []core.ColorStop{{Color: blue}, {Color: yellow}}}}})}),
			tile(tileProps{label: "Oklab", note: "No gray in the middle"}, core.View{Style: specimen(core.Style{CornerRadius: 14, Gradients: []core.Gradient{core.LinearGradient(90, blue, yellow)}})}),
			tile(tileProps{label: "sRGB", code: "ColorSpaceSRGB"}, core.View{Style: specimen(core.Style{CornerRadius: 14,
				Gradients: []core.Gradient{{Angle: 90, Space: core.ColorSpaceSRGB, Stops: []core.ColorStop{{Color: blue}, {Color: yellow}}}}})}),
			tile(tileProps{label: "Radial", code: "RadialGradient"}, core.View{Style: specimen(core.Style{CornerRadius: 14, Gradients: []core.Gradient{core.RadialGradient(yellow, pink, blue)}})}),
			tile(tileProps{label: "Conic, repeating", code: "ConicGradient · Repeating"}, core.View{Style: core.Style{Gap: 8, Direction: core.Row}, Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(64), Height: core.Pt(64), CornerRadius: 32, Gradients: []core.Gradient{core.ConicGradient(0, pink, yellow, blue, pink)}}},
				core.View{Style: core.Style{Width: core.Pt(64), Height: core.Pt(64), CornerRadius: 12, Gradients: []core.Gradient{{Angle: 45, Repeating: true, Space: core.ColorSpaceSRGB,
					Stops: []core.ColorStop{{Color: colors.raised, At: core.Pt(0)}, {Color: colors.raised, At: core.Pt(8)}, {Color: tints.accent, At: core.Pt(8)}, {Color: tints.accent, At: core.Pt(16)}}}}}},
			}}),
		),
	)
}

func filtersCard(cx *core.Scope) core.Node {
	paint := []core.Gradient{core.LinearGradient(135, core.RGB(0xf43f5e), core.RGB(0xf59e0b), core.RGB(0x10b981), core.RGB(0x3b82f6))}
	art := func(name string, f ...core.Filter) core.Node {
		return core.View{Style: core.Style{AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(56), Height: core.Pt(56), CornerRadius: 12, Gradients: paint, Filters: f}},
			text(name, 12, regular, colors.muted),
		}}
	}
	stripes := []core.Gradient{{Angle: 90, Repeating: true, Space: core.ColorSpaceSRGB, Stops: []core.ColorStop{
		{Color: colors.accent, At: core.Pt(0)}, {Color: colors.accent, At: core.Pt(6)}, {Color: colors.surface, At: core.Pt(6)}, {Color: colors.surface, At: core.Pt(12)}}}}
	pane := func(name string, s core.Style) core.Node {
		return core.View{Style: core.Style{Width: core.Pt(88), Height: core.Pt(60), CornerRadius: 12, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}.Merge(s),
			Children: []core.Node{text(name, 12, semibold, colors.text)}}
	}
	return card(cardProps{icon: iconImage, title: "Filters, backdrops and masks", subtitle: "Filters · BackdropFilters · Mask · BlendMode", right: badge("NSGo", toneNeutral)},
		core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 10}, Children: []core.Node{
			art("none"), art("grayscale", core.Grayscale(1)), art("sepia", core.Sepia(1)), art("hue 120°", core.HueRotate(120)),
			art("invert", core.Invert(1)), art("blur 4", core.Blur(4)),
		}},
		core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 10, Padding: core.All(12), CornerRadius: radiusControl, Gradients: stripes}, Children: []core.Node{
			pane("frosted", core.Style{Background: alpha(colors.surface, 0.4), BackdropFilters: []core.Filter{core.Blur(8)}}),
			pane("glass", core.Style{Background: alpha(colors.surface, 0.3), BackdropFilters: []core.Filter{core.Blur(12)}, GlassEffect: core.GlassRegular, CornerRadius: 24}),
			pane("masked", core.Style{Background: colors.info, Mask: &core.Gradient{Angle: 90, Stops: []core.ColorStop{{Color: core.RGB(0)}, {Color: 0}}}}),
			pane("multiply", core.Style{Background: core.RGB(0x60a5fa), BlendMode: core.BlendMultiply}),
		}},
	)
}

func keyframesCard(cx *core.Scope) core.Node {
	dot := func(name string, k core.Keyframes, s core.Style) core.Node {
		return core.View{Style: core.Style{AlignItems: core.AlignCenter, Gap: 8, Width: core.Pt(64)}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(32), Height: core.Pt(32), Margin: core.Edges{Top: 8}, Animation: core.KeyframeAnimation{Keyframes: k}}.Merge(s)},
			text(name, 12, regular, colors.muted),
		}}
	}
	return card(cardProps{icon: iconAnimation, title: "Keyframes and group styles", subtitle: "Animation · GroupStyle", right: badge("NSGo", toneNeutral)},
		core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceAround}, Children: []core.Node{
			dot("spin", core.KeyframesSpin, core.Style{CornerRadius: 16, Borders: core.All(4), BorderColors: core.EdgeColors{Top: colors.accent, Right: colors.raised, Bottom: colors.raised, Left: colors.raised}}),
			dot("ping", core.KeyframesPing, core.Style{Background: colors.accent, CornerRadius: 16}),
			dot("pulse", core.KeyframesPulse, core.Style{Background: colors.accent, CornerRadius: 6}),
			dot("bounce", core.KeyframesBounce, core.Style{Background: colors.accent, CornerRadius: 16}),
		}},
		core.View{OnPress: func(core.PressEvent) {}, Role: core.RoleButton, Label: "Hover this row",
			Style: core.Style{Group: "row", Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.All(12), CornerRadius: radiusControl,
				BorderWidth: 1, BorderColor: colors.line, Hover: &core.Style{BorderColor: colors.accent}},
			Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(28), Height: core.Pt(28), CornerRadius: 14, Background: colors.raised, Transition: core.Transition{Duration: 150 * time.Millisecond},
					GroupStyle: &core.GroupStyle{Name: "row", Hover: &core.Style{Background: colors.accent}, Active: &core.Style{Transform: core.Transform{Scale: 0.85}}}}},
				core.Text{Content: hint(cx, "Press this row: its dot follows", "Hover this row: its dot and title follow"), Style: core.Style{FontSize: 14, Color: colors.muted,
					GroupStyle: &core.GroupStyle{Name: "row", Hover: &core.Style{Color: colors.text}}}},
			}},
	)
}

// stylesDemo is how views draw: corners, borders, shadows, clipping,
// stacking, 3D transforms and direction, then NSGo's gradients, filters,
// keyframes and group styles.
func stylesDemo(cx *core.Scope) core.Node {
	return demoPage(cx, pageProps{}, cornersCard, bordersCard, shadowsCard, layersCard, transformsCard, rightToLeft, gradientsCard, filtersCard, keyframesCard)
}
