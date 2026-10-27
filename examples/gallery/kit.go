package main

import (
	"fmt"
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// text is a line in the gallery's type: size, weight and color.
func text(content string, size core.Points, weight core.FontWeight, c core.Color) core.Text {
	return core.Text{Content: content, Style: core.Style{FontSize: size, FontWeight: weight, Color: c}}
}

// textf is a line whose content follows fn.
func textf(fn func() string, size core.Points, weight core.FontWeight, c core.Color) core.Text {
	return core.Text{ContentFunc: fn, Style: core.Style{FontSize: size, FontWeight: weight, Color: c}}
}

// Weights, short.
const (
	regular  = core.FontWeightNormal
	medium   = core.FontWeightMedium
	semibold = core.FontWeightSemibold
	bold     = core.FontWeightBold
)

// note is a quiet line of explanation under a demo.
func note(cx *core.Scope, content string) core.Node {
	colors := colorsOf(cx)
	return core.Text{Content: content, Style: core.Style{FontSize: 13, LineHeight: 18, Color: colors.muted}}
}

// notef is a quiet line that follows fn.
func notef(cx *core.Scope, fn func() string) core.Node {
	colors := colorsOf(cx)
	return core.Text{ContentFunc: fn, Style: core.Style{FontSize: 13, LineHeight: 18, Color: colors.muted}}
}

// code is a monospaced line, for API names and values.
func code(cx *core.Scope, content string) core.Node {
	colors := colorsOf(cx)
	return core.Text{Content: content, Style: core.Style{FontSize: 12, FontFamily: mono, Color: colors.code}}
}

// pressProps are a Press's options.
type pressProps struct {
	onPress, onLongPress func()
	disabled             func() bool
	// scale is how far it shrinks (default 0.97).
	scale    float64
	style    core.Style
	role     core.Role
	label    string
	selected func() bool
	hitSlop  core.Points
	testID   string
}

// press is a pressable that sinks a little under the finger and springs
// back; the spring runs on the platform, only press in and out reach the
// engine.
func press(p pressProps, children ...core.Node) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		s := core.NewAnimatedValue(cx, 1)
		scale := p.scale
		if scale == 0 {
			scale = 0.97
		}
		role := p.role
		if role == 0 {
			role = core.RoleButton
		}
		var onPress, onLongPress func(core.PressEvent)
		if p.onPress != nil {
			onPress = func(core.PressEvent) { p.onPress() }
		}
		if p.onLongPress != nil {
			onLongPress = func(core.PressEvent) { p.onLongPress() }
		}
		style := p.style
		style.Animated = &core.AnimatedStyle{Scale: s}
		style.Disabled = &core.Style{Opacity: core.Alpha(0.4)}
		return core.View{OnPress: onPress, OnLongPress: onLongPress, Disabled: p.disabled, Selected: p.selected,
			OnPressDown: func(core.PressEvent) { spring(cx, s, scale, pressSpring) },
			OnPressUp:   func(core.PressEvent) { spring(cx, s, 1, pressSpring) },
			Role:        role, Label: p.label, HitSlop: core.All(p.hitSlop), TestID: p.testID,
			Style: style, Children: children}
	})
}

// appear fades and lifts its content in once mounted; index staggers
// siblings, 40 ms apart.
func appear(index int, flex bool, children ...core.Node) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		t := core.NewAnimatedValue(cx, 0)
		c := enterTiming
		c.Delay = time.Duration(min(8, index)) * 40 * time.Millisecond
		timing(cx, t, 1, c)
		s := core.Style{Animated: &core.AnimatedStyle{Opacity: t, TranslateY: lerp(t, 14, 0)}}
		if flex {
			s = core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), MinWidth: core.Pt(0)}.Merge(s)
		}
		return core.View{Style: s, Children: children}
	})
}

// iconTile is a rounded tile around an icon.
func iconTile(cx *core.Scope, name iconName, size core.Points) core.Node {
	colors := colorsOf(cx)
	fin := finOf(cx)
	return core.View{Style: fin.raised.over(core.Style{
		Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size * 0.3, BorderCurve: core.BorderCurveContinuous,
		BorderWidth: 1, BorderColor: colors.border, Background: colors.raised,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
	}), Children: []core.Node{icon(name, size*0.5, colors.text)}}
}

// cardProps are a card's header: an optional icon, a title, a subtitle
// and something on the right (a badge, an action).
type cardProps struct {
	icon     iconName
	title    string
	subtitle string
	right    core.Node
	// flush shows the content bare instead of on an inset.
	flush bool
	style core.Style
}

func cardStyle(cx *core.Scope) core.Style {
	colors := colorsOf(cx)
	fin := finOf(cx)
	return fin.card.over(core.Style{Background: colors.surface, CornerRadius: radiusCard, BorderCurve: core.BorderCurveContinuous,
		BorderWidth: 1, BorderColor: colors.border, Padding: core.All(5)})
}

// card is a card: its header, then the content on an inset surface.
func card(cx *core.Scope, p cardProps, children ...core.Node) core.Node {
	colors := colorsOf(cx)
	var nodes []core.Node
	if p.title != "" {
		var head []core.Node
		if p.icon != "" {
			head = append(head, iconTile(cx, p.icon, 40))
		}
		titles := []core.Node{text(p.title, 17, semibold, colors.text)}
		if p.subtitle != "" {
			titles = append(titles, core.Text{Content: p.subtitle, MaxLines: 2, Style: core.Style{FontSize: 13, Color: colors.muted}})
		}
		head = append(head, core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Gap: 2}, Children: titles})
		if p.right != nil {
			head = append(head, p.right)
		}
		nodes = append(nodes, core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12,
			Padding: core.Edges{Top: 11, Right: 11, Bottom: 12, Left: 11}}, Children: head})
	}
	if p.flush {
		nodes = append(nodes, children...)
	} else {
		nodes = append(nodes, inset(cx, insetProps{}, children...))
	}
	return core.View{Style: cardStyle(cx).Merge(p.style), Children: nodes}
}

// insetProps are an inset's spacing; zero values take 12 pt gaps and
// 14 pt padding.
type insetProps struct {
	gap, padding core.Points
	// exact takes gap and padding as given, zeros included.
	exact bool
}

// inset is the darker surface inside a card.
func inset(cx *core.Scope, p insetProps, children ...core.Node) core.Node {
	colors := colorsOf(cx)
	fin := finOf(cx)
	gap, padding := p.gap, p.padding
	if !p.exact {
		if gap == 0 {
			gap = 12
		}
		if padding == 0 {
			padding = 14
		}
	}
	return core.View{Style: fin.inset.over(core.Style{Background: colors.inset, CornerRadius: radiusInset, BorderCurve: core.BorderCurveContinuous,
		BorderWidth: 1, BorderColor: colors.line, Padding: core.All(padding), Gap: gap}), Children: children}
}

// section is a titled card of controls: the demos' sections.
func section(cx *core.Scope, title, noteText string, children ...core.Node) core.Node {
	return sectionf(cx, title, func() string { return noteText }, children...)
}

// sectionf is a section whose note follows fn.
func sectionf(cx *core.Scope, title string, noteFn func() string, children ...core.Node) core.Node {
	colors := colorsOf(cx)
	return core.View{TestID: "section-" + title, Style: cardStyle(cx), Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8, Padding: core.Edges{Top: 10, Right: 11, Bottom: 10, Left: 11}},
			Children: []core.Node{
				core.Text{Content: title, Role: core.RoleHeader, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 15, FontWeight: semibold, Color: colors.text}},
				core.Text{ContentFunc: noteFn, Style: core.Style{FontSize: 13, Color: colors.faint}},
			}},
		inset(cx, insetProps{}, children...),
	}}
}

// caption is a label on the left and a quieter note on the right.
func caption(cx *core.Scope, label string, noteFn func() string) core.Node {
	colors := colorsOf(cx)
	children := []core.Node{core.Text{Content: label, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 14, Color: colors.muted}}}
	if noteFn != nil {
		children = append(children, core.Text{ContentFunc: noteFn, Style: core.Style{FontSize: 13, Color: colors.faint}})
	}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8}, Children: children}
}

// figure is a large figure with a unit and a tinted note after it.
func figure(cx *core.Scope, value func() string, unit string, noteFn func() string, t tone) core.Node {
	colors := colorsOf(cx)
	spans := []core.Span{{ContentFunc: value}}
	if unit != "" {
		spans = append(spans, core.Span{Content: " " + unit, Style: core.Style{FontSize: 17, FontWeight: regular, Color: colors.muted}})
	}
	if noteFn != nil {
		spans = append(spans, core.Span{ContentFunc: func() string { return " " + noteFn() }, Style: core.Style{FontSize: 15, FontWeight: medium, Color: t.color(cx)}})
	}
	return core.Text{Spans: spans, Style: core.Style{FontSize: 32, FontWeight: semibold, Color: colors.text, LetterSpacing: -0.5}}
}

// badge is a small label in a tinted capsule.
func badge(cx *core.Scope, label string, t tone) core.Node {
	return badgef(cx, func() string { return label }, t, false)
}

// badgef is a badge whose label follows fn; outline draws only its rim.
func badgef(cx *core.Scope, label func() string, t tone, outline bool) core.Node {
	colors := colorsOf(cx)
	fin := finOf(cx)
	s := core.Style{Padding: core.Symmetric(0, 10), Height: core.Pt(26), CornerRadius: 13, JustifyContent: core.JustifyCenter, BorderWidth: 1}
	fg := colors.text
	if t != toneNeutral {
		fg = t.color(cx)
	}
	if outline {
		s.BorderColor = t.color(cx)
	} else {
		s.Background, s.Shadows, s.BorderColor = t.tint(cx), fin.chip.shadows, alpha(fg, 0.1)
	}
	return core.View{Style: s, Children: []core.Node{core.Text{ContentFunc: label, Style: core.Style{FontSize: 12, FontWeight: medium, Color: fg}}}}
}

// stat is a label above a value, for grids of figures.
func stat(cx *core.Scope, label string, value func() string, t tone, width core.Length) core.Node {
	colors := colorsOf(cx)
	fg := colors.text
	if t != toneNeutral {
		fg = t.color(cx)
	}
	return core.View{Style: core.Style{Width: width, Gap: 3, Padding: core.Symmetric(4, 0)}, Children: []core.Node{
		text(label, 12, regular, colors.muted),
		textf(value, 15, regular, fg),
	}}
}

// legend is a colored square and a label, for chart legends.
func legend(cx *core.Scope, c core.Color, label string, value func() string) core.Node {
	colors := colorsOf(cx)
	spans := []core.Span{{Content: label}}
	if value != nil {
		spans = append(spans, core.Span{ContentFunc: func() string { return " " + value() }, Style: core.Style{Color: colors.text}})
	}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
		core.View{Style: core.Style{Width: core.Pt(9), Height: core.Pt(9), CornerRadius: 2.5, Background: c}},
		core.Text{Spans: spans, Style: core.Style{FontSize: 13, Color: colors.muted}},
	}}
}

// listRow is a row inside an inset, with a hairline above all but the
// first.
func listRow(cx *core.Scope, first bool, onPress func(), children ...core.Node) core.Node {
	colors := colorsOf(cx)
	s := core.Style{MinHeight: core.Pt(48), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}
	if !first {
		s.Borders, s.BorderColors = core.Edges{Top: 1}, core.EdgeColors{Top: colors.line}
	}
	v := core.View{Style: s, Children: children}
	if onPress != nil {
		v.OnPress = func(core.PressEvent) { onPress() }
		v.Style.Active = &core.Style{Background: colors.raised}
	}
	return v
}

// buttonVariant is a button's look: primary is light (the main action),
// secondary dark, accent orange, ghost only a rim.
type buttonVariant uint8

const (
	primary buttonVariant = iota
	secondary
	accentButton
	ghost
)

// buttonProps are a button's options.
type buttonProps struct {
	label   string
	variant buttonVariant
	// Color fills it with that color instead.
	color    core.Color
	icon     iconName
	disabled func() bool
	// flex grows it to share a row with its siblings.
	flex   float32
	small  bool
	testID string
}

// button is a button, 44 pt tall (32 pt small, for pointer-sized
// surfaces). base.Button gives the behavior; the look is the gallery's.
func button(p buttonProps, onPress func()) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		colors := colorsOf(cx)
		fin := finOf(cx)
		scale := core.NewAnimatedValue(cx, 1)
		r := radiusControl
		height, padding, gap, font, iconSize := core.Points(44), core.Points(18), core.Points(8), core.Points(15), core.Points(16)
		if p.small {
			r, height, padding, gap, font, iconSize = 8, 32, 12, 6, 13, 14
		}
		bg, fg := colors.raised, colors.text
		var f finish
		switch {
		case p.color != 0:
			bg, fg = p.color, core.RGB(0xffffff)
		case p.variant == primary:
			bg, fg, f = colors.light, colors.onLight, fin.light
		case p.variant == accentButton:
			bg, fg, f = colors.accentFill, colors.onAccent, fin.accent
		case p.variant == ghost:
			bg = 0
		default:
			f = fin.raised
		}
		if p.color != 0 {
			f = finish{}
		}
		face := f.over(core.Style{Height: core.Pt(height), Padding: core.Symmetric(0, padding), CornerRadius: r, BorderCurve: core.BorderCurveContinuous,
			Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: gap, Background: bg})
		if p.variant == secondary || p.variant == ghost {
			face.BorderWidth, face.BorderColor = 1, colors.border
		}
		var children []core.Node
		if p.icon != "" {
			children = append(children, icon(p.icon, iconSize, fg))
		}
		children = append(children, text(p.label, font, semibold, fg))
		return base.Button{
			OnPress:     func(core.PressEvent) { onPress() },
			OnPressDown: func(core.PressEvent) { spring(cx, scale, 0.97, pressSpring) },
			OnPressUp:   func(core.PressEvent) { spring(cx, scale, 1, pressSpring) },
			Disabled:    p.disabled, Label: p.label, TestID: p.testID,
			Style: core.Style{FlexGrow: p.flex, FlexShrink: p.flex, CornerRadius: r, Animated: &core.AnimatedStyle{Scale: scale},
				Hover: &core.Style{Opacity: core.Alpha(0.88)}, Focus: focusRing(cx, 2), Disabled: &core.Style{Opacity: core.Alpha(0.4)}},
			Children: []core.Node{core.View{Style: face, Children: children}},
		}
	})
}

// roundButton is a round icon button (44 pt): light by default,
// secondary dark.
func roundButton(cx *core.Scope, name iconName, label string, variant buttonVariant, size core.Points, onPress func()) core.Node {
	colors := colorsOf(cx)
	fin := finOf(cx)
	if size == 0 {
		size = 44
	}
	isPrimary := variant == primary
	s := core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size / 2, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}
	fg := colors.text
	if isPrimary {
		s, fg = fin.light.over(s), colors.onLight
		s.Background = colors.light
	} else {
		s = fin.pill.over(s)
		s.BorderWidth = 1
	}
	return press(pressProps{onPress: onPress, scale: 0.92, label: label},
		core.View{Style: s, Children: []core.Node{icon(name, size*0.42, fg)}})
}

// toggle is an on and off switch in the gallery's look: base.Switch, the
// knob springing across and the track changing color on the platform.
func toggle(value func() bool, onChange func(bool), label string) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		look := newSwitchLook(cx, value, false)
		// 8 pt more to press on every side, without taking more room.
		return base.Switch{Checked: value, OnChange: onChange, Label: label,
			Style:      core.Style{Margin: core.All(-8), Padding: core.All(8)},
			TrackStyle: look.track, ThumbStyleFunc: look.thumb}
	})
}

// switchLook is the gallery's switch as the styles of base.Switch's
// parts: a track that turns orange and a knob that springs across, both
// on the platform. on is the value the app owns; the knob follows it.
type switchLook struct {
	track core.Style
	thumb func() core.Style
}

// newSwitchLook makes a switch's look, 50 × 30 or, small, 38 × 22 for
// pointer-sized rows.
func newSwitchLook(cx *core.Scope, on func() bool, small bool) switchLook {
	colors := colorsOf(cx)
	fin := finOf(cx)
	isDark := cx.Dark()
	width, height, pad, knob := core.Points(50), core.Points(30), core.Points(3), core.Points(22)
	if small {
		width, height, pad, knob = 38, 22, 2, 16
	}
	start := 0.0
	shown := core.Untracked(cx, on)
	if shown {
		start = 1
	}
	t := core.NewAnimatedValue(cx, start)
	x := lerp(t, 0, float64(width-2-2*pad-knob))
	env := cx.Env()
	cx.Effect(func() {
		next := on()
		if next == shown {
			return
		}
		shown = next
		to := 0.0
		if next {
			to = 1
		}
		// Reduced motion: the knob and the track change in place.
		if env.ReduceMotion() {
			t.Set(to)
		} else {
			spring(cx, t, to, settleSpring)
		}
	})
	track := fin.track.over(core.Style{Width: core.Pt(width), Height: core.Pt(height), CornerRadius: height / 2, Padding: core.All(pad),
		BorderWidth: 1, BorderColor: colors.border, Background: colors.track,
		Animated: &core.AnimatedStyle{Background: core.Interpolate(t, core.Interpolation{Input: []float64{0, 1}, Colors: []core.Color{colors.track, colors.accentFill}})}})
	knobShadows := fin.knob.shadows
	if isDark {
		knobShadows = []core.BoxShadow{inner(0, 1, 0, white(0.2)), shadow(0, 1, 3, 0, black(0.4))}
	}
	return switchLook{track: track, thumb: func() core.Style {
		bg := colors.knob
		if on() {
			bg = colors.onAccent
		}
		return core.Style{Width: core.Pt(knob), Height: core.Pt(knob), CornerRadius: knob / 2, Background: bg,
			// A drop of glass: a specular top over the knob's color.
			Gradients: gradA(white(0.202), white(0.098)), Shadows: knobShadows,
			Animated: &core.AnimatedStyle{TranslateX: x}}
	}}
}

// option is one segment of a segmented control; count shows a quiet
// number after the label when set.
type option[T comparable] struct {
	value T
	label string
	count func() int
}

// segmented is equal segments with an indicator that springs to the
// selected one: base.Tabs, a tab per segment, in the gallery's look.
func segmented[T comparable](options []option[T], value func() T, onChange func(T), small bool) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		colors := colorsOf(cx)
		fin := finOf(cx)
		width := core.NewSignal(cx, core.Points(0))
		x := core.NewAnimatedValue(cx, 0)
		index := func() int {
			v := value()
			for i, o := range options {
				if o.value == v {
					return i
				}
			}
			return 0
		}
		// The track's border and padding: the indicator is placed inside
		// the border, so the segments share what is left of the measured
		// width after both.
		const border, pad = 1, 3
		segment := func() core.Points { return (width.Get() - 2*(border+pad)) / core.Points(max(1, len(options))) }
		height := core.Points(40)
		if small {
			height = 32
		}
		// The first placement jumps; later ones spring.
		placed := false
		cx.Effect(func() {
			to := float64(core.Points(index()) * segment())
			if width.Get() <= 0 {
				return
			}
			if placed {
				spring(cx, x, to, settleSpring)
			} else {
				x.Set(to)
			}
			placed = true
		})
		font := core.Points(14)
		if small {
			font = 13
		}
		children := []core.Node{core.View{PointerEvents: core.PointerEventsNone, StyleFunc: func() core.Style {
			s := fin.pill.over(core.Style{Position: core.Absolute, Top: core.Pt(pad), Left: core.Pt(pad), Bottom: core.Pt(pad),
				Width: core.Pt(max(0, segment())), CornerRadius: (height - 2*(border+pad)) / 2, BorderWidth: 1,
				Animated: &core.AnimatedStyle{TranslateX: x}})
			return s
		}}}
		for _, o := range options {
			selected := func() bool { return value() == o.value }
			label := []core.Node{core.Text{Content: o.label, MaxLines: 1, StyleFunc: func() core.Style {
				c := colors.muted
				if selected() {
					c = colors.text
				}
				return core.Style{FontSize: font, FontWeight: medium, Color: c}
			}}}
			if o.count != nil {
				label = append(label, core.Text{ContentFunc: func() string { return fmt.Sprint(o.count()) }, Style: core.Style{FontSize: 12, Color: colors.faint}})
			}
			children = append(children, base.Tab{Selected: selected, OnPress: func(core.PressEvent) { onChange(o.value) }, Label: o.label,
				Style:    core.Style{FlexGrow: 1, FlexBasis: core.Pt(0), Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 5},
				Children: label})
		}
		return core.View{OnLayout: func(b core.Bounds) { width.Set(b.Size.Width) }, Children: []core.Node{base.Tabs{
			Style: core.Style{Height: core.Pt(height), Direction: core.Row, Padding: core.All(pad), CornerRadius: height / 2,
				Background: colors.inset, Shadows: fin.track.shadows, BorderWidth: border, BorderColor: colors.border},
			Children: children,
		}}}
	})
}

// meter is a horizontal meter with an optional marker (a limit, negative
// for none).
func meter(cx *core.Scope, value func() float64, t tone, marker float64) core.Node {
	colors := colorsOf(cx)
	children := []core.Node{core.View{StyleFunc: func() core.Style {
		return core.Style{Width: core.Pct(float32(min(max(value(), 0), 1) * 100)), Height: core.Pt(6), CornerRadius: 3, Background: t.color(cx)}
	}}}
	if marker >= 0 {
		children = append(children, core.View{Style: core.Style{Position: core.Absolute, Left: core.Pct(float32(marker * 100)), Top: core.Pt(-3),
			Width: core.Pt(2), Height: core.Pt(12), CornerRadius: 1, Background: colors.text}})
	}
	return core.View{Style: core.Style{Height: core.Pt(6), CornerRadius: 3, Background: colors.raised}, Children: children}
}

// sheet is a panel at the bottom over a dimmed backdrop while open
// (base.Sheet in the gallery's look); a press on the backdrop, Escape or
// the back button closes it.
func sheet(cx *core.Scope, open func() bool, onClose func(), children ...core.Node) core.Node {
	colors := colorsOf(cx)
	env := cx.Env()
	return base.Sheet{
		Open: open,
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				onClose()
			}
		},
		BackdropStyle: core.Style{Background: black(0.4)},
		PopupStyleFunc: func() core.Style {
			return core.Style{Gap: 12, Padding: core.Edges{Top: 8, Right: 16, Bottom: env.SafeArea().Bottom + 16, Left: 16},
				CornerRadii: core.Corners{TopLeft: 16, TopRight: 16}, Background: colors.surface}
		},
		Children: append([]core.Node{core.View{Style: core.Style{Width: core.Pt(36), Height: core.Pt(5), CornerRadius: 3,
			AlignSelf: core.AlignCenter, Background: colors.raised}}}, children...),
	}
}

// field is an input in its well: the field's color, a rim and an inner
// shadow; the input's own style sets its size and padding.
func field(cx *core.Scope, in base.Input) core.Node {
	colors := colorsOf(cx)
	fin := finOf(cx)
	in.Style = core.Style{FontSize: 15, Color: colors.text, PlaceholderColor: colors.faint, SelectionColor: colors.accent}.Merge(in.Style)
	return core.View{Style: fin.field.over(core.Style{CornerRadius: radiusControl, BorderCurve: core.BorderCurveContinuous,
		BorderWidth: 1, BorderColor: colors.fieldBorder, Background: colors.field}), Children: []core.Node{in}}
}

// captionf is a caption whose label follows fn.
func captionf(cx *core.Scope, fn func() string) core.Node {
	colors := colorsOf(cx)
	return core.Text{ContentFunc: fn, Style: core.Style{FontSize: 14, Color: colors.muted}}
}
