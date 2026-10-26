package main

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// hairline is one device pixel, in points.
func hairline(env *core.Env) core.Points {
	r := env.PixelRatio()
	if r == 0 {
		r = 1
	}
	return core.Points(1 / r)
}

// darkField is an input on its own, 48 pt tall, its rim the accent while
// it has the focus.
func darkField(in core.Input) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		focused := core.NewSignal(cx, false)
		onFocus, onBlur := in.OnFocus, in.OnBlur
		in.OnFocus = func() {
			focused.Set(true)
			if onFocus != nil {
				onFocus()
			}
		}
		in.OnBlur = func() {
			focused.Set(false)
			if onBlur != nil {
				onBlur()
			}
		}
		in.StyleFunc = func() core.Style {
			border := colors.fieldBorder
			if focused.Get() {
				border = colors.accent
			}
			return fin.field.over(core.Style{PlaceholderColor: colors.faint, SelectionColor: colors.accent, Height: core.Pt(48),
				Padding: core.Symmetric(0, 14), FontSize: 15, Color: colors.text, Background: colors.field,
				BorderWidth: 1, BorderColor: border, CornerRadius: radiusControl})
		}
		return in
	})
}

// cell is a cell of a row of equal widths.
func cell(n core.Node) core.Node {
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0)}, Children: []core.Node{n}}
}

// linkRow is a row that pushes a screen.
func linkRow(name iconName, title, noteText string, onPress func()) core.Node {
	return press(pressProps{onPress: onPress, scale: 0.98, label: title},
		core.View{Style: core.Style{MinHeight: core.Pt(60), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(8, 10)}, Children: []core.Node{
			iconTile(name, 34),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
				text(title, 15, medium, colors.text),
				core.Text{Content: noteText, MaxLines: 1, Style: core.Style{FontSize: 12, Color: colors.faint, FontFamily: mono}},
			}},
			icon(iconChevron, 14, colors.faint),
		}})
}

var tileTones = []tone{toneAccent, toneInfo, toneWarn, toneUp, toneDanger}

func layoutAnimations(cx *core.Scope) core.Node {
	items := core.NewSignalFunc(cx, []int{1, 2, 3}, slices.Equal)
	preset := core.NewSignal(cx, "spring")
	next := 4
	presets := map[string]core.LayoutAnimation{"spring": core.LayoutSpringPreset, "ease": core.LayoutEaseInEaseOutPreset, "linear": core.LayoutLinearPreset}
	animate := func(change func()) {
		cx.AnimateLayout(presets[preset.GetUntracked()])
		change()
	}
	highlight := inner(0, 1, 0, core.RGB(0xffffff))
	if isDark {
		highlight = inner(0, 1, 0, white(0.06))
	}
	return card(cardProps{icon: iconAnimation, title: "Layout animation", subtitle: "AnimateLayout, then change the tree",
		right: badgef(func() string { return itoa(len(items.Get())) }, toneNeutral, false)},
		segmented([]option[string]{{value: "spring", label: "Spring"}, {value: "ease", label: "Ease"}, {value: "linear", label: "Linear"}}, preset.Get, preset.Set, true),
		core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 8, MinHeight: core.Pt(56)}, Children: []core.Node{
			core.Each(items.Get, func(n int) int { return n }, func(cx *core.Scope, n func() int, _ func() int) core.Node {
				t := tileTones[core.Untracked(cx, n)%len(tileTones)]
				return core.View{Style: core.Style{Width: core.Pt(56), Height: core.Pt(56), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous,
					Background: t.tint(), Shadows: []core.BoxShadow{highlight}, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
					Children: []core.Node{core.Text{ContentFunc: func() string { return itoa(n()) }, Style: core.Style{FontSize: 17, FontWeight: semibold, Color: t.color()}}}}
			}),
		}},
		core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
			cell(button(buttonProps{label: "Add", icon: iconPlus}, func() {
				animate(func() { items.Update(func(s []int) []int { return append([]int{next}, s...) }); next++ })
			})),
			cell(button(buttonProps{label: "Shuffle", variant: secondary}, func() {
				animate(func() {
					items.Update(func(s []int) []int {
						s = slices.Clone(s)
						rand.Shuffle(len(s), func(i, j int) { s[i], s[j] = s[j], s[i] })
						return s
					})
				})
			})),
			cell(button(buttonProps{label: "Remove", variant: secondary, disabled: func() bool { return len(items.Get()) == 0 }}, func() {
				animate(func() { items.Update(func(s []int) []int { return slices.Clone(s[1:]) }) })
			})),
		}},
	)
}

func pressableSheet(cx *core.Scope) core.Node {
	open := core.NewSignal(cx, false)
	env := cx.Env()
	pressedFill := core.RGB(0xb84a15)
	if isDark {
		pressedFill = core.RGB(0xd8825a)
	}
	return card(cardProps{icon: iconComponents, title: "Sheet", subtitle: "A pressed state style opens it"},
		core.View{OnPress: func(core.PressEvent) { open.Set(true) }, Role: core.RoleButton, Label: "Open a sheet",
			Style: core.Style{Shadows: fin.accent.shadows, Height: core.Pt(48), Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
				Gap: 8, CornerRadius: 12, Background: colors.accentFill, Transition: core.Transition{Duration: 150e6},
				Active: &core.Style{Transform: core.Transform{Scale: 0.97}, Background: pressedFill}},
			Children: []core.Node{text("Open a sheet", 15, semibold, colors.onAccent)}},
		caption("Scales and darkens while pressed", func() string { return "Active: scale 0.97" }),
		base.Sheet{Open: open.Get, OnOpenChange: func(o bool, _ base.Reason) { open.Set(o) }, Label: "A sheet",
			BackdropStyle: core.Style{Background: black(0.5)},
			PopupStyleFunc: func() core.Style {
				return core.Style{Gap: 16, Padding: core.Edges{Top: 20, Right: 20, Bottom: env.SafeArea().Bottom + 20, Left: 20},
					CornerRadii: core.Corners{TopLeft: 24, TopRight: 24}, Background: colors.background}
			},
			Children: []core.Node{
				core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{badge("Sheet", toneAccent)}},
					core.Text{Content: "A sheet", Style: core.Style{FontSize: 28, FontWeight: bold, LetterSpacing: -0.5, Color: colors.text}},
					core.Text{Content: hint(cx, "Tap outside it or press back (Android) to close.", "Press Escape or click outside it to close."),
						Style: core.Style{FontSize: 15, LineHeight: 21, Color: colors.muted}},
				}},
				button(buttonProps{label: "Close"}, func() { open.Set(false) }),
			}},
	)
}

// pressTile is a tile to press, with the style it shows.
func pressTile(title func() string, noteText string, right core.Node) core.Node {
	children := []core.Node{core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: []core.Node{
		textf(title, 15, regular, colors.text),
		core.Text{Content: noteText, Style: core.Style{FontSize: 12, Color: colors.faint, FontFamily: mono}},
	}}}
	if right != nil {
		children = append(children, right)
	}
	return core.View{Style: core.Style{MinHeight: core.Pt(60), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(8, 12)}, Children: children}
}

// tileStyle is the raised highlight without its gradient, so an underlay
// color shows.
func tileStyle() core.Style {
	return core.Style{CornerRadius: radiusControl, BorderCurve: core.BorderCurveContinuous, BorderWidth: 1, BorderColor: colors.border,
		Background: colors.surface, Shadows: fin.raised.shadows}
}

// pressFeedback is press feedback drawn by the platform: opacity, an
// underlay; Disabled takes no presses.
func pressFeedback(cx *core.Scope) core.Node {
	last := core.NewSignal(cx, "None yet")
	disabled := core.NewSignal(cx, false)
	fade, under, third := tileStyle(), tileStyle(), tileStyle()
	fade.Active = &core.Style{Opacity: core.Alpha(0.4)}
	under.Active = &core.Style{Background: colors.raised}
	third.Active = &core.Style{Background: tints.danger}
	third.Disabled = &core.Style{Opacity: core.Alpha(0.4)}
	return card(cardProps{icon: iconGestures, title: "Press feedback", subtitle: hint(cx, "Drawn natively while the finger is down", "Drawn natively while the button is down")},
		core.View{Style: fade, Role: core.RoleButton, OnPress: func(core.PressEvent) { last.Set("Fading tile") },
			Children: []core.Node{pressTile(label("Fades while pressed"), "Active: Opacity 0.4", nil)}},
		core.View{Style: under, Role: core.RoleButton, OnPress: func(core.PressEvent) { last.Set("Underlay tile") },
			Children: []core.Node{pressTile(label("Lights up while pressed"), "Active: Background", nil)}},
		core.View{Style: third, Role: core.RoleButton, Disabled: disabled.Get, OnPress: func(core.PressEvent) { last.Set("Enabled tile") },
			Children: []core.Node{pressTile(func() string {
				if disabled.Get() {
					return "Takes no presses"
				}
				return "Enabled"
			}, "Disabled + Disabled: Opacity 0.4", core.Dynamic(func(*core.Scope) core.Node {
				if disabled.Get() {
					return badge("Off", toneNeutral)
				}
				return badge("On", toneUp)
			}))}},
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
			core.View{Style: fill, Children: []core.Node{caption("Last pressed", last.Get)}},
			toggle(disabled.Get, disabled.Set, "Disable the third tile"),
		}},
	)
}

func accessory(cx *core.Scope) core.Node {
	value := core.NewSignal(cx, "")
	input := &core.Ref{}
	env := cx.Env()
	captionText := "The bar counts what you type"
	switch {
	case env.Desktop():
		captionText = "Desktops have no software keyboard"
	}
	return card(cardProps{icon: iconText, title: "Input accessory", subtitle: "A bar over the software keyboard", right: badge("iOS · Android", toneNeutral)},
		darkField(core.Input{Ref: input, Value: value.Get, OnChange: value.Set, Placeholder: "Focus me", InputAccessoryViewID: "counter", TestID: "accessory-field"}),
		caption(captionText, func() string { return `InputAccessoryViewID: "counter"` }),
		core.InputAccessoryView{ID: "counter", TestID: "accessory-bar", Style: core.Style{Background: colors.surface}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(8, 16),
				Borders: core.Edges{Top: hairline(env)}, BorderColors: core.EdgeColors{Top: colors.border}}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return fmt.Sprintf("%d characters", len([]rune(value.Get()))) }, Style: core.Style{FlexGrow: 1, FontSize: 13, Color: colors.muted}},
				button(buttonProps{label: "Clear", variant: secondary}, func() { value.Set("") }),
				button(buttonProps{label: "Done"}, input.Blur),
			}},
		}},
	)
}

// avoidingPage is a screen whose field at the bottom stays above the
// keyboard.
func avoidingPage(cx *core.Scope) core.Screen {
	behavior := core.NewSignal(cx, base.AvoidPadding)
	names := map[base.AvoidBehavior]string{base.AvoidPadding: "padding", base.AvoidHeight: "height"}
	notes := map[base.AvoidBehavior]string{
		base.AvoidPadding: "The bottom pads: the tinted background stays under the keyboard.",
		base.AvoidHeight:  "The view shrinks: its background ends at the keyboard.",
	}
	return screen("KeyboardAvoidingView", nil, core.Dynamic(func(cx *core.Scope) core.Node {
		b := behavior.Get()
		return base.KeyboardAvoidingView{Behavior: b, Style: core.Style{FlexGrow: 1, Background: tints.info}, Children: []core.Node{
			core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(16), Gap: 16}, Children: []core.Node{
				card(cardProps{icon: iconForm, title: fmt.Sprintf("Behavior: %s", names[b]), subtitle: "Focus the field at the bottom"},
					segmented([]option[base.AvoidBehavior]{{value: base.AvoidPadding, label: "padding"}, {value: base.AvoidHeight, label: "height"}}, behavior.Get, behavior.Set, false),
					core.Text{Content: notes[b], Style: core.Style{FontSize: 14, LineHeight: 20, Color: colors.muted}},
				),
				spacer(),
				darkField(core.Input{Placeholder: "Type here"}),
			}},
		}}
	}))
}

func modalOptions(cx *core.Scope) core.Node {
	visible, translucent := core.NewSignal(cx, false), core.NewSignal(cx, false)
	bg := core.RGB(0xf8ece3)
	if isDark {
		bg = core.RGB(0x2a1d15)
	}
	return card(cardProps{icon: iconSystem, title: "Dialog options", subtitle: "StatusBarTranslucent, orientations",
		right: toggle(translucent.Get, translucent.Set, "StatusBarTranslucent")},
		core.Text{Content: "Android: translucent content reaches under the status bar. iOS: this dialog allows portrait and landscape.",
			Style: core.Style{FontSize: 14, LineHeight: 20, Color: colors.muted}},
		caption("StatusBarTranslucent", func() string { return fmt.Sprint(translucent.Get()) }),
		button(buttonProps{label: "Open the dialog", variant: secondary}, func() { visible.Set(true) }),
		core.Dynamic(func(*core.Scope) core.Node {
			t := translucent.Get()
			return base.Dialog{Open: visible.Get, Animation: core.ModalSlide, StatusBarTranslucent: t, Label: "A dialog",
				SupportedOrientations: core.OrientationPortrait | core.OrientationLandscape,
				OnOpenChange:          func(o bool, _ base.Reason) { visible.Set(o) },
				PopupStyle:            core.Style{FlexGrow: 1, AlignSelf: core.AlignStretch},
				Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, Background: bg}, Children: []core.Node{
					base.SafeAreaView{Edges: base.Top | base.Bottom | base.Left | base.Right, Style: core.Style{FlexGrow: 1, Padding: core.All(16), Gap: 12}, Children: []core.Node{
						core.View{Style: core.Style{Direction: core.Row, Padding: core.Edges{Top: 8}}, Children: []core.Node{
							badge(map[bool]string{true: "translucent", false: "opaque bar"}[t], toneAccent)}},
						core.Text{Content: "A dialog", Style: core.Style{FontSize: 28, FontWeight: bold, LetterSpacing: -0.5, Color: colors.text}},
						core.Text{Content: fmt.Sprintf("The olive background starts %s the status bar on Android.", map[bool]string{true: "under", false: "below"}[t]),
							Style: core.Style{FontSize: 15, LineHeight: 21, Color: colors.muted}},
						spacer(),
						button(buttonProps{label: "Close"}, func() { visible.Set(false) }),
					}},
				}}}}
		}),
	)
}

func platformInfo(cx *core.Scope) core.Node {
	env := cx.Env()
	rows := func() [][2]string {
		r := env.PixelRatio()
		if r == 0 {
			r = 1
		}
		return [][2]string{
			{"Env.OS", osName(env.OS())},
			{"Env.OSVersion", env.OSVersion().String()},
			{"Env.Idiom", idiomName(env.Idiom())},
			{"Env.PixelRatio", fmt.Sprint(r)},
			{"10.3 on whole pixels", fmt.Sprint(math.Round(10.3*float64(r)) / float64(r))},
			{"Env.ScreenReader", fmt.Sprint(env.ScreenReader())},
			{"Env.ReduceMotion", fmt.Sprint(env.ReduceMotion())},
			{"Env.Locale", env.Locale()},
		}
	}
	announced := 0
	target := &core.Ref{}
	return card(cardProps{icon: iconSpark, title: "Platform", subtitle: "The environment, PixelRatio, Announce", right: badge(osName(env.OS()), toneAccent)},
		core.Dynamic(func(*core.Scope) core.Node {
			var nodes []core.Node
			for i, r := range rows() {
				nodes = append(nodes, listRow(i == 0, nil,
					core.Text{Content: r[0], MaxLines: 1, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 12, Color: colors.muted, FontFamily: mono}},
					core.Text{Content: r[1], MaxLines: 1, Style: core.Style{FontSize: 14, Color: colors.text}},
				))
			}
			return core.View{Children: nodes}
		}),
		core.View{StyleFunc: func() core.Style { return core.Style{Height: core.Pt(hairline(env)), Background: colors.accent} }},
		caption("The line above is one device pixel tall", func() string { return fmt.Sprintf("%.3f pt", hairline(env)) }),
		core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
			cell(button(buttonProps{label: "Announce"}, func() {
				announced++
				cx.Announce(fmt.Sprintf("Announcement %d", announced))
			})),
			cell(button(buttonProps{label: "Focus the note", variant: secondary}, target.SetAccessibilityFocus)),
		}},
		core.Text{Ref: target, Content: "With VoiceOver or TalkBack on, the buttons above speak and move the reader here.",
			Style: core.Style{FontSize: 13, LineHeight: 18, Color: colors.faint}},
	)
}

// idiomName is a device idiom in words.
func idiomName(i core.Idiom) string {
	switch i {
	case core.IdiomPhone:
		return "phone"
	case core.IdiomTablet:
		return "tablet"
	case core.IdiomDesktop:
		return "desktop"
	}
	return "unknown"
}

// componentsDemo is press feedback, a sheet, layout animation, an input
// accessory, keyboard avoidance, a dialog's options and the platform.
func componentsDemo(cx *core.Scope) core.Node {
	return demoPage(cx, pageProps{behavior: core.ScrollBehavior{KeyboardDismissMode: core.KeyboardDismissInteractive}},
		pressFeedback, pressableSheet, layoutAnimations, accessory,
		func(cx *core.Scope) core.Node {
			return card(cardProps{title: "Keyboard avoidance", subtitle: "Content outside scroll views", flush: true},
				inset(insetProps{exact: true}, linkRow(iconForm, "KeyboardAvoidingView", "padding / height", func() { shell.push(avoidingPage) })))
		},
		modalOptions, platformInfo)
}
