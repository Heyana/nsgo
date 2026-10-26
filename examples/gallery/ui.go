package main

import (
	_ "embed"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// The gallery's look, Onevra's palette: a warm charcoal page (or a warm
// paper one in light appearance), cards with an inset surface and one
// orange accent, the logo's. Sizes are for phones: 16 pt gutters, 44 pt
// touch targets, 15 pt body text. The desktop layout keeps them and puts
// more side by side. base has no look, so every color and finish here is
// the app's own.

// palette is the roles of the gallery's colors in one appearance.
type palette struct {
	// background is the page behind the cards; surface a card; inset a
	// surface inside a card; raised a control on a card.
	background, surface, inset, raised core.Color
	// field is an input's well, below the card and the insets it sits on;
	// fieldBorder its edge.
	field, fieldBorder core.Color
	border             core.Color
	// line is the hairlines inside insets.
	line               core.Color
	text, muted, faint core.Color
	// accent is the accent as a foreground: text, icons, lines, small
	// marks. accentFill is the accent as a fill, with onAccent on it.
	accent, accentFill, onAccent core.Color
	// light is the main action's button, light on the dark page and dark
	// on the light one, with onLight on it.
	light, onLight core.Color
	// track and knob are a switch's when off.
	track, knob core.Color
	// hover is a row under the pointer, pressed one held down.
	hover, pressed core.Color
	// code is code inside insets.
	code                         core.Color
	info, warn, danger, up, down core.Color
}

// tintSet is translucent tints of the tones, for fills behind text.
type tintSet struct{ accent, info, warn, danger, up, down core.Color }

// colors and tints are the appearance shown. The gallery builds its tree
// again when the appearance changes (Root), so components read them when
// they are built.
var (
	colors *palette
	tints  tintSet
	fin    *finishes
	isDark bool
)

var lightPalette = palette{
	background: core.RGB(0xf5f3ef), surface: core.RGB(0xfefefd), inset: core.RGB(0xf2efea), raised: core.RGB(0xfcfbf9),
	field: core.RGB(0xebe8e1), fieldBorder: core.RGB(0xd3cfc6), border: core.RGB(0xdbd7ce), line: core.RGB(0xe5e1d9),
	text: core.RGB(0x2d2b27), muted: core.RGB(0x72726a), faint: core.RGB(0xa29e95),
	accent: core.RGB(0xbd481b), accentFill: core.RGB(0xd4581b), onAccent: core.RGB(0xffffff),
	light: core.RGB(0x2d2b27), onLight: core.RGB(0xffffff), track: core.RGB(0xe5e1d9), knob: core.RGB(0xffffff),
	hover: core.RGB(0xffffff), pressed: core.RGB(0xe8e4dc), code: core.RGB(0x3a3631),
	info: core.RGB(0x3d63d1), warn: core.RGB(0xb45309), danger: core.RGB(0xb4444c), up: core.RGB(0x059669), down: core.RGB(0xb4444c),
}

var darkPalette = palette{
	background: core.RGB(0x1a1816), surface: core.RGB(0x22201d), inset: core.RGB(0x151412), raised: core.RGB(0x2e2b27),
	field: core.RGB(0x151412), fieldBorder: core.RGB(0x3b3732), border: core.RGB(0x36332e), line: core.RGB(0x2e2b27),
	text: core.RGB(0xeeeae2), muted: core.RGB(0xa39c90), faint: core.RGB(0x6a645b),
	accent: core.RGB(0xed986e), accentFill: core.RGB(0xed986e), onAccent: core.RGB(0x291b14),
	light: core.RGB(0xeeeae2), onLight: core.RGB(0x1f1d1a), track: core.RGB(0x2e2b27), knob: core.RGB(0xa39c90),
	hover: core.RGB(0x282622), pressed: core.RGB(0x2e2b27), code: core.RGB(0xe2dcd2),
	info: core.RGB(0x7ea8ff), warn: core.RGB(0xe6b567), danger: core.RGB(0xe58f95), up: core.RGB(0x79bea1), down: core.RGB(0xe58f95),
}

// useAppearance points colors, tints and the finishes at an appearance.
func useAppearance(dark bool) {
	isDark = dark
	if dark {
		colors, fin = &darkPalette, &darkFinishes
		tints = tintSet{alpha(colors.accent, 0.14), alpha(colors.info, 0.16), alpha(colors.warn, 0.16),
			alpha(colors.danger, 0.16), alpha(colors.up, 0.16), alpha(colors.down, 0.16)}
		return
	}
	colors, fin = &lightPalette, &lightFinishes
	tints = tintSet{alpha(colors.accentFill, 0.12), alpha(colors.info, 0.10), alpha(colors.warn, 0.11),
		alpha(colors.danger, 0.10), alpha(colors.up, 0.10), alpha(colors.down, 0.10)}
}

// alpha returns c at opacity a.
func alpha(c core.Color, a float32) core.Color {
	r, g, b, _ := c.Components()
	return core.RGBA(uint32(r*255+0.5)<<16|uint32(g*255+0.5)<<8|uint32(b*255+0.5), a)
}

// finish is a surface's light and depth: a gradient over its color, its
// shadows, a rim and, for glass, a blur of what is behind.
type finish struct {
	background core.Color
	gradients  []core.Gradient
	shadows    []core.BoxShadow
	border     core.Color
	backdrop   []core.Filter
}

// style is the finish as a style to merge into a surface's.
func (f finish) style() core.Style {
	return core.Style{Background: f.background, Gradients: f.gradients, Shadows: f.shadows, BorderColor: f.border, BackdropFilters: f.backdrop}
}

// over merges the finish into s, keeping what s sets.
func (f finish) over(s core.Style) core.Style { return f.style().Merge(s) }

// finishes are gentle top-to-bottom gradients, a highlight along top edges
// and soft shadows, so cards, controls and buttons read as surfaces.
type finishes struct {
	card, inset, field, raised, light, accent, track, knob finish
	// pill is lit glass floating over a track: the selected segment, a
	// small round button.
	pill finish
	// chip is a tinted capsule's rim and highlight (badges).
	chip finish
	// glass is a bar floating over content: translucent, blurred, rimmed.
	glass finish
	// menu is a floating panel (menus, the composer): glass, denser for
	// reading.
	menu finish
}

func grad(from, to uint32) []core.Gradient {
	return []core.Gradient{core.LinearGradient(180, core.RGB(from), core.RGB(to))}
}

func gradA(stops ...core.Color) []core.Gradient {
	return []core.Gradient{core.LinearGradient(180, stops...)}
}

// shadow is one of CSS's box-shadow layers: x, y, blur, spread, color.
func shadow(x, y, blur, spread core.Points, c core.Color) core.BoxShadow {
	return core.BoxShadow{Offset: core.Point{X: x, Y: y}, Blur: blur, Spread: spread, Color: c}
}

// inner is an inset shadow layer.
func inner(x, y, blur core.Points, c core.Color) core.BoxShadow {
	return core.BoxShadow{Offset: core.Point{X: x, Y: y}, Blur: blur, Color: c, Inset: true}
}

func black(a float32) core.Color { return core.RGBA(0x000000, a) }
func white(a float32) core.Color { return core.RGBA(0xffffff, a) }
func paper(a float32) core.Color { return core.RGBA(0x1e1408, a) }

var darkFinishes = finishes{
	card:   finish{gradients: grad(0x24221f, 0x211f1c), shadows: []core.BoxShadow{inner(0, 1, 0, white(0.04)), shadow(0, 16, 36, -20, black(0.75))}},
	inset:  finish{shadows: []core.BoxShadow{inner(0, 1, 3, black(0.55))}},
	field:  finish{shadows: []core.BoxShadow{inner(0, 1, 2, black(0.55))}},
	raised: finish{gradients: grad(0x33302b, 0x2e2b27), shadows: []core.BoxShadow{inner(0, 1, 0, white(0.07)), shadow(0, 1, 1, 0, black(0.4)), shadow(0, 6, 14, -7, black(0.7))}},
	light:  finish{gradients: grad(0xfbf9f5, 0xe6e1d8), shadows: []core.BoxShadow{inner(0, 1, 0, core.RGB(0xffffff)), shadow(0, 1, 1, 0, black(0.4)), shadow(0, 8, 18, -8, black(0.75))}},
	accent: finish{gradients: grad(0xf3a982, 0xe98f62), shadows: []core.BoxShadow{inner(0, 1, 0, white(0.45)), shadow(0, 1, 1, 0, black(0.4)), shadow(0, 8, 20, -9, core.RGBA(0xed986e, 0.4))}},
	track:  finish{shadows: []core.BoxShadow{inner(0, 1, 2, black(0.5))}},
	knob:   finish{shadows: []core.BoxShadow{inner(0, 1, 0, white(0.35)), shadow(0, 2, 6, 0, black(0.45))}},
	pill: finish{background: core.RGB(0x33302b), gradients: grad(0x37342f, 0x322f2a),
		shadows: []core.BoxShadow{inner(0, 1, 0, white(0.08)), shadow(0, 1, 1, 0, black(0.4)), shadow(0, 6, 14, -7, black(0.7))}, border: white(0.08)},
	chip: finish{shadows: []core.BoxShadow{inner(0, 1, 0, white(0.07)), inner(0, -1, 0, black(0.18))}},
	glass: finish{background: core.RGBA(0x1e1c19, 0.62), gradients: gradA(white(0.04), white(0.019)),
		shadows: []core.BoxShadow{inner(0, 1, 0, white(0.09)), shadow(0, -8, 24, 0, black(0.28))}, border: white(0.07), backdrop: []core.Filter{core.Blur(20)}},
	menu: finish{background: core.RGBA(0x282522, 0.82), gradients: gradA(white(0.043), white(0.024), white(0.018)),
		shadows: []core.BoxShadow{inner(0, 1, 0, white(0.11)), inner(0, -1, 0, black(0.25)), shadow(0, 12, 32, 0, black(0.42))}, border: white(0.08), backdrop: []core.Filter{core.Blur(24)}},
}

var lightFinishes = finishes{
	card:   finish{gradients: grad(0xffffff, 0xfdfcfa), shadows: []core.BoxShadow{inner(0, 1, 0, core.RGB(0xffffff)), shadow(0, 1, 1, 0, paper(0.04)), shadow(0, 14, 32, -18, paper(0.2))}},
	inset:  finish{shadows: []core.BoxShadow{inner(0, 1, 2, paper(0.07))}},
	field:  finish{shadows: []core.BoxShadow{inner(0, 1, 2, paper(0.09))}},
	raised: finish{gradients: grad(0xffffff, 0xf7f5f0), shadows: []core.BoxShadow{inner(0, 1, 0, core.RGB(0xffffff)), shadow(0, 1, 1, 0, paper(0.06)), shadow(0, 5, 12, -6, paper(0.18))}},
	light:  finish{gradients: grad(0x3a3834, 0x262420), shadows: []core.BoxShadow{inner(0, 1, 0, white(0.16)), shadow(0, 1, 1, 0, paper(0.2)), shadow(0, 7, 14, -7, paper(0.5))}},
	accent: finish{gradients: grad(0xe06526, 0xc9521a), shadows: []core.BoxShadow{inner(0, 1, 0, white(0.3)), shadow(0, 1, 1, 0, core.RGBA(0x8a340c, 0.2)), shadow(0, 7, 14, -7, core.RGBA(0xc9521a, 0.5))}},
	track:  finish{shadows: []core.BoxShadow{inner(0, 1, 2, paper(0.08))}},
	knob:   finish{shadows: []core.BoxShadow{inner(0, 1, 0, core.RGB(0xffffff)), shadow(0, 1, 4, 0, paper(0.28))}},
	pill: finish{background: core.RGB(0xffffff), gradients: grad(0xffffff, 0xf7f5f0),
		shadows: []core.BoxShadow{inner(0, 1, 0, core.RGB(0xffffff)), shadow(0, 1, 1, 0, paper(0.06)), shadow(0, 5, 12, -6, paper(0.18))}, border: paper(0.14)},
	chip: finish{shadows: []core.BoxShadow{inner(0, 1, 0, white(0.65)), inner(0, -1, 0, paper(0.04))}},
	glass: finish{background: core.RGBA(0xfbfaf7, 0.66), gradients: gradA(white(0.472), white(0.227)),
		shadows: []core.BoxShadow{inner(0, 1, 0, white(0.9)), shadow(0, -6, 20, 0, paper(0.06))}, border: paper(0.07), backdrop: []core.Filter{core.Blur(20)}},
	menu: finish{background: core.RGBA(0xffffff, 0.84), gradients: gradA(white(0.497), white(0.287), white(0.217)),
		shadows: []core.BoxShadow{inner(0, 1, 0, core.RGB(0xffffff)), inner(0, -1, 0, paper(0.05)), shadow(0, 12, 32, 0, paper(0.12))}, border: paper(0.08), backdrop: []core.Filter{core.Blur(24)}},
}

// tone names a color role for badges, figures and meters.
type tone uint8

const (
	toneNeutral tone = iota
	toneAccent
	toneInfo
	toneWarn
	toneDanger
	toneUp
	toneDown
)

// color is the tone's foreground.
func (t tone) color() core.Color {
	switch t {
	case toneAccent:
		return colors.accent
	case toneInfo:
		return colors.info
	case toneWarn:
		return colors.warn
	case toneDanger:
		return colors.danger
	case toneUp:
		return colors.up
	case toneDown:
		return colors.down
	}
	return colors.muted
}

// tint is the tone's translucent fill.
func (t tone) tint() core.Color {
	switch t {
	case toneAccent:
		return tints.accent
	case toneInfo:
		return tints.info
	case toneWarn:
		return tints.warn
	case toneDanger:
		return tints.danger
	case toneUp:
		return tints.up
	case toneDown:
		return tints.down
	}
	return colors.raised
}

// mono is the monospaced family, for code (API names, identifiers).
const mono = core.FontMonospace

// Corner radii.
const (
	radiusCard    core.Points = 16
	radiusInset   core.Points = 12
	radiusControl core.Points = 10
)

// The layouts' sizes.
const (
	// sidebarWidth is the width of the desktop layout's sidebar.
	sidebarWidth core.Points = 264
	// desktopWidth is the narrowest window that takes the desktop layout
	// on desktops; tabletWidth on other devices (an iPad in landscape).
	desktopWidth core.Points = 700
	tabletWidth  core.Points = 900
)

// Shared styles.
var (
	row     = core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}
	wrapRow = core.Style{Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignCenter, Gap: 8}
	fill    = core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}
)

// The motion every control shares: quick, slightly springy.
var (
	pressSpring  = core.SpringConfig{Stiffness: 520, Damping: 34}
	settleSpring = core.SpringConfig{Stiffness: 320, Damping: 30}
	easeOutCubic = core.Easing{Kind: core.EasingCubic, Mode: core.EaseModeOut}
	enterTiming  = core.TimingConfig{Duration: 420 * time.Millisecond, Easing: easeOutCubic}
)

// spring and timing start an animation of v on the platform.
func spring(cx *core.Scope, v *core.AnimatedValue, to float64, c core.SpringConfig) {
	cx.StartAnimation(core.Spring(v, to, c), nil)
}

func timing(cx *core.Scope, v *core.AnimatedValue, to float64, c core.TimingConfig) {
	cx.StartAnimation(core.Timing(v, to, c), nil)
}

// lerp maps v's 0 to 1 onto from to to.
func lerp(v core.AnimatedNode, from, to float64) core.AnimatedNode {
	return core.Interpolate(v, core.Interpolation{Input: []float64{0, 1}, Output: []float64{from, to}})
}

// hovered is a pressable under the pointer: a whisper of the text color
// over its surface, quieter than pressed and than selected.
func hovered() *core.Style { return &core.Style{Background: alpha(colors.text, 0.05)} }

// focusRing is the keyboard focus: a 2-point ring of info inside the
// view's edge (offset 2 puts it outside, around a filled button).
func focusRing(offset core.Points) *core.Style {
	return &core.Style{OutlineWidth: 2, OutlineColor: colors.info, OutlineOffset: offset}
}

// hint is instructions for the device's input: touch on phones and
// tablets, pointer on desktops.
func hint(cx *core.Scope, touch, pointer string) string {
	if cx.Env().Desktop() {
		return pointer
	}
	return touch
}

// bindRefresh is desktops' way to refresh what phones pull down: Ctrl+R /
// Cmd+R calls refresh while the calling component shows. Browsers keep the
// key to reload the page. It returns the key's label, or "" where it is
// not bound.
func bindRefresh(cx *core.Scope, refresh func()) string {
	env := cx.Env()
	if !env.Desktop() || env.OS() == core.OSWeb {
		return ""
	}
	cx.OnKey("mod+r", refresh)
	return cx.KeyLabel("mod+r")
}

// columnsFor is how many columns at least min pt wide fit in width, with
// gap between them and 24 pt gutters: 1 to max.
func columnsFor(min core.Points, max int, width core.Points, gap core.Points) int {
	n := int(math.Floor(float64((width - 48 + gap) / (min + gap))))
	return clampInt(n, 1, max)
}

func clampInt(v, lo, hi int) int {
	return min(max(v, lo), hi)
}

func clamp(v, lo, hi float64) float64 { return math.Min(math.Max(v, lo), hi) }

// count formats n with a unit, plural past one.
func count(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

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
func note(content string) core.Node {
	return core.Text{Content: content, Style: core.Style{FontSize: 13, LineHeight: 18, Color: colors.muted}}
}

// notef is a quiet line that follows fn.
func notef(fn func() string) core.Node {
	return core.Text{ContentFunc: fn, Style: core.Style{FontSize: 13, LineHeight: 18, Color: colors.muted}}
}

// code is a monospaced line, for API names and values.
func code(content string) core.Node {
	return core.Text{Content: content, Style: core.Style{FontSize: 12, FontFamily: mono, Color: colors.code}}
}

// hrow lays nodes out in a row with a gap.
func hrow(gap core.Points, nodes ...core.Node) core.Node {
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: gap}, Children: nodes}
}

// wrap lays nodes out in a row that wraps.
func wrap(nodes ...core.Node) core.Node { return core.View{Style: wrapRow, Children: nodes} }

// spacer takes the free space of a row.
func spacer() core.Node { return core.View{Style: core.Style{FlexGrow: 1}} }

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
			s = fillPage.Merge(s)
		}
		return core.View{Style: s, Children: children}
	})
}

// iconTile is a rounded tile around an icon.
func iconTile(name iconName, size core.Points) core.Node {
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

func cardStyle() core.Style {
	return fin.card.over(core.Style{Background: colors.surface, CornerRadius: radiusCard, BorderCurve: core.BorderCurveContinuous,
		BorderWidth: 1, BorderColor: colors.border, Padding: core.All(5)})
}

// card is a card: its header, then the content on an inset surface.
func card(p cardProps, children ...core.Node) core.Node {
	var nodes []core.Node
	if p.title != "" {
		var head []core.Node
		if p.icon != "" {
			head = append(head, iconTile(p.icon, 40))
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
		nodes = append(nodes, inset(insetProps{}, children...))
	}
	return core.View{Style: cardStyle().Merge(p.style), Children: nodes}
}

// insetProps are an inset's spacing; zero values take 12 pt gaps and
// 14 pt padding.
type insetProps struct {
	gap, padding core.Points
	// exact takes gap and padding as given, zeros included.
	exact bool
}

// inset is the darker surface inside a card.
func inset(p insetProps, children ...core.Node) core.Node {
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
func section(title, noteText string, children ...core.Node) core.Node {
	return sectionf(title, func() string { return noteText }, children...)
}

// sectionf is a section whose note follows fn.
func sectionf(title string, noteFn func() string, children ...core.Node) core.Node {
	return core.View{TestID: "section-" + title, Style: cardStyle(), Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8, Padding: core.Edges{Top: 10, Right: 11, Bottom: 10, Left: 11}},
			Children: []core.Node{
				core.Text{Content: title, Role: core.RoleHeader, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 15, FontWeight: semibold, Color: colors.text}},
				core.Text{ContentFunc: noteFn, Style: core.Style{FontSize: 13, Color: colors.faint}},
			}},
		inset(insetProps{}, children...),
	}}
}

// caption is a label on the left and a quieter note on the right.
func caption(label string, noteFn func() string) core.Node {
	children := []core.Node{core.Text{Content: label, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 14, Color: colors.muted}}}
	if noteFn != nil {
		children = append(children, core.Text{ContentFunc: noteFn, Style: core.Style{FontSize: 13, Color: colors.faint}})
	}
	return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8}, Children: children}
}

// figure is a large figure with a unit and a tinted note after it.
func figure(value func() string, unit string, noteFn func() string, t tone) core.Node {
	spans := []core.Span{{ContentFunc: value}}
	if unit != "" {
		spans = append(spans, core.Span{Content: " " + unit, Style: core.Style{FontSize: 17, FontWeight: regular, Color: colors.muted}})
	}
	if noteFn != nil {
		spans = append(spans, core.Span{ContentFunc: func() string { return " " + noteFn() }, Style: core.Style{FontSize: 15, FontWeight: medium, Color: t.color()}})
	}
	return core.Text{Spans: spans, Style: core.Style{FontSize: 32, FontWeight: semibold, Color: colors.text, LetterSpacing: -0.5}}
}

// badge is a small label in a tinted capsule.
func badge(label string, t tone) core.Node { return badgef(func() string { return label }, t, false) }

// badgef is a badge whose label follows fn; outline draws only its rim.
func badgef(label func() string, t tone, outline bool) core.Node {
	s := core.Style{Padding: core.Symmetric(0, 10), Height: core.Pt(26), CornerRadius: 13, JustifyContent: core.JustifyCenter, BorderWidth: 1}
	fg := colors.text
	if t != toneNeutral {
		fg = t.color()
	}
	if outline {
		s.BorderColor = t.color()
	} else {
		s.Background, s.Shadows, s.BorderColor = t.tint(), fin.chip.shadows, alpha(fg, 0.1)
	}
	return core.View{Style: s, Children: []core.Node{core.Text{ContentFunc: label, Style: core.Style{FontSize: 12, FontWeight: medium, Color: fg}}}}
}

// stat is a label above a value, for grids of figures.
func stat(label string, value func() string, t tone, width core.Length) core.Node {
	fg := colors.text
	if t != toneNeutral {
		fg = t.color()
	}
	return core.View{Style: core.Style{Width: width, Gap: 3, Padding: core.Symmetric(4, 0)}, Children: []core.Node{
		text(label, 12, regular, colors.muted),
		textf(value, 15, regular, fg),
	}}
}

// legend is a colored square and a label, for chart legends.
func legend(c core.Color, label string, value func() string) core.Node {
	spans := []core.Span{{Content: label}}
	if value != nil {
		spans = append(spans, core.Span{ContentFunc: func() string { return " " + value() }, Style: core.Style{Color: colors.text}})
	}
	return hrow(6,
		core.View{Style: core.Style{Width: core.Pt(9), Height: core.Pt(9), CornerRadius: 2.5, Background: c}},
		core.Text{Spans: spans, Style: core.Style{FontSize: 13, Color: colors.muted}},
	)
}

// listRow is a row inside an inset, with a hairline above all but the
// first.
func listRow(first bool, onPress func(), children ...core.Node) core.Node {
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
	// color fills it with that color instead.
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
				Hover: &core.Style{Opacity: core.Alpha(0.88)}, Focus: focusRing(2), Disabled: &core.Style{Opacity: core.Alpha(0.4)}},
			Children: []core.Node{core.View{Style: face, Children: children}},
		}
	})
}

// roundButton is a round icon button (44 pt): light by default,
// secondary dark.
func roundButton(name iconName, label string, variant buttonVariant, size core.Points, onPress func()) core.Node {
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

// themeToggle switches the app between light and dark, whatever the
// system shows.
func themeToggle(cx *core.Scope) core.Node {
	name, label, next := iconSun, "Light appearance", core.AppearanceLight
	if !isDark {
		name, label, next = iconMoon, "Dark appearance", core.AppearanceDark
	}
	return roundButton(name, label, secondary, 34, func() { cx.SetAppearance(next) })
}

//go:embed assets/logo-dark.svg
var logoDark []byte

//go:embed assets/logo-light.svg
var logoLight []byte

// logo is the gallery's mark in the appearance shown (deeper oranges on
// light backgrounds).
func logo(size core.Points) core.Node {
	data := logoLight
	if isDark {
		data = logoDark
	}
	return core.Image{Source: core.ImageSource{Data: data, Size: core.Size{Width: 100, Height: 100}},
		Style: core.Style{Width: core.Pt(size), Height: core.Pt(size)}}
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
func meter(value func() float64, t tone, marker float64) core.Node {
	children := []core.Node{core.View{StyleFunc: func() core.Style {
		return core.Style{Width: core.Pct(float32(clamp(value(), 0, 1) * 100)), Height: core.Pt(6), CornerRadius: 3, Background: t.color()}
	}}}
	if marker >= 0 {
		children = append(children, core.View{Style: core.Style{Position: core.Absolute, Left: core.Pct(float32(marker * 100)), Top: core.Pt(-3),
			Width: core.Pt(2), Height: core.Pt(12), CornerRadius: 1, Background: colors.text}})
	}
	return core.View{Style: core.Style{Height: core.Pt(6), CornerRadius: 3, Background: colors.raised}, Children: children}
}

// card builders take the demo's scope, so a demo page lists its cards as
// functions built when shown.
type cardFunc func(cx *core.Scope) core.Node

// pageProps are a demo page's options.
type pageProps struct {
	// columns is the most columns on a desktop (default 2).
	columns int
	// bottom is the space under the last card on phones (default 40).
	bottom core.Points
	// still keeps the cards from fading in.
	still    bool
	behavior core.ScrollBehavior
	// behaviorFunc, when set, replaces behavior as it changes.
	behaviorFunc func() core.ScrollBehavior
	// after follows the cards inside the scroll view (a demo's modals).
	after []core.Node
	ref   *core.Ref
}

// demoPage is a demo's page of cards, each fading in after the one
// before. On phones one column on 16 pt gutters; in the desktop layout the
// cards are dealt left to right into as many columns as fit the pane (at
// least 380 pt each), on 24 pt gutters.
func demoPage(cx *core.Scope, p pageProps, cards ...cardFunc) core.Node {
	build := func(i int, c cardFunc) core.Node {
		node := core.Func(func(cx *core.Scope) core.Node { return c(cx) })
		if p.still {
			return node
		}
		return appear(i, false, node)
	}
	bottom := p.bottom
	if bottom == 0 {
		bottom = 40
	}
	most := p.columns
	if most == 0 {
		most = 2
	}
	var body core.Node
	if !shell.desktop() {
		var nodes []core.Node
		for i, c := range cards {
			nodes = append(nodes, build(i, c))
		}
		body = core.View{Style: core.Style{Padding: core.Edges{Top: 16, Right: 16, Bottom: bottom, Left: 16}, Gap: 16}, Children: nodes}
	} else {
		columns := core.NewMemo(cx, func() int { return columnsFor(380, most, shell.paneWidth(), 20) })
		body = core.Dynamic(func(cx *core.Scope) core.Node {
			n := columns.Get()
			return core.Func(func(cx *core.Scope) core.Node {
				var cols []core.Node
				for column := range n {
					var nodes []core.Node
					for i, c := range cards {
						if i%n == column {
							nodes = append(nodes, build(i, c))
						}
					}
					cols = append(cols, core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), Gap: 20}, Children: nodes})
				}
				return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignStart, Gap: 20,
					Padding: core.Edges{Top: 24, Right: 24, Bottom: 48, Left: 24}, Width: core.Pct(100),
					MaxWidth: core.Pt(core.Points(n)*560 + 48), AlignSelf: core.AlignCenter}, Children: cols}
			})
		})
	}
	return core.Scroll{Ref: p.ref, TestID: "demo-scroll", Behavior: p.behavior, BehaviorFunc: p.behaviorFunc, Style: core.Style{FlexGrow: 1, FlexShrink: 1, Background: colors.background},
		Children: append([]core.Node{body}, p.after...)}
}

// sheet is a panel at the bottom over a dimmed backdrop while open
// (base.Sheet in the gallery's look); a press on the backdrop, Escape or
// the back button closes it.
func sheet(cx *core.Scope, open func() bool, onClose func(), children ...core.Node) core.Node {
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

// pageHeaderProps are a page header's options.
type pageHeaderProps struct {
	title string
	// root is the bottom page of its stack: no back button.
	root bool
	// onBack is what back does (default: pop the page).
	onBack func()
	// left replaces the back button, center the title.
	left, center, right core.Node
	// background is the bar's fill (default the page's background).
	background core.Color
	// tint is the back button's color (default the accent).
	tint core.Color
	// transparent lays it over the content: no fill, no hairline.
	transparent bool
}

// pageHeader is a page's header where the app draws it (desktops, the
// web): a back button while the page is above another, the title and
// right. Phones show their system header with the screen's title instead.
func pageHeader(p pageHeaderProps) core.Node {
	tint := p.tint
	if tint == 0 {
		tint = colors.accent
	}
	back := func() bool { return p.left == nil && !p.root && shell.nav.Depth() > 0 }
	s := core.Style{Height: core.Pt(52), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(0, 12)}
	switch {
	case p.transparent:
	case p.background != 0:
		s.Background = p.background
	default:
		s.Background, s.Borders, s.BorderColors = colors.background, core.Edges{Bottom: 1}, core.EdgeColors{Bottom: colors.line}
	}
	var children []core.Node
	if p.left != nil {
		children = append(children, p.left)
	}
	children = append(children, core.Show{When: back, Then: func(*core.Scope) core.Node {
		return core.View{
			OnPress: func(core.PressEvent) {
				if p.onBack != nil {
					p.onBack()
				} else {
					shell.nav.Pop(1)
				}
			},
			Role: core.RoleButton, Label: "Back", HitSlop: core.All(6),
			Style: core.Style{Height: core.Pt(32), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 2,
				Padding: core.Edges{Left: 4, Right: 10}, CornerRadius: radiusControl, Cursor: core.CursorPointer,
				Hover: &core.Style{Background: colors.hover}, Active: &core.Style{Background: colors.pressed}},
			Children: []core.Node{icon(iconBack, 18, tint), text("Back", 14, medium, tint)},
		}
	}})
	center := p.center
	if center == nil {
		center = core.Text{Content: p.title, MaxLines: 1, Role: core.RoleHeader, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 17, FontWeight: semibold, Color: colors.text}}
	}
	children = append(children, core.View{StyleFunc: func() core.Style {
		var pad core.Points = 4
		if back() || p.left != nil {
			pad = 0
		}
		return core.Style{FlexGrow: 1, FlexShrink: 1, Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.Symmetric(0, pad)}
	}, Children: []core.Node{center}})
	if p.right != nil {
		children = append(children, p.right)
	}
	return core.View{Style: s, Children: children}
}

// headerStyle is the system header's colors on every page (iOS,
// Android).
func headerStyle() core.Style {
	return core.Style{Background: colors.background, Color: colors.text, TintColor: colors.accent}
}

// screen is a page a demo pushes: the system header on phones, the app's
// own elsewhere.
func screen(title string, right core.Node, children ...core.Node) core.Screen {
	return core.Screen{Title: title, HeaderStyle: headerStyle(), HeaderRight: right,
		Header:   pageHeader(pageHeaderProps{title: title, right: right}),
		Style:    core.Style{Background: colors.background},
		Children: children}
}

// itoa formats an integer.
func itoa(n int) string { return strconv.Itoa(n) }

// field is an input in its well: the field's color, a rim and an inner
// shadow; the input's own style sets its size and padding.
func field(in core.Input) core.Node {
	in.Style = core.Style{FontSize: 15, Color: colors.text, PlaceholderColor: colors.faint, SelectionColor: colors.accent}.Merge(in.Style)
	return core.View{Style: fin.field.over(core.Style{CornerRadius: radiusControl, BorderCurve: core.BorderCurveContinuous,
		BorderWidth: 1, BorderColor: colors.fieldBorder, Background: colors.field}), Children: []core.Node{in}}
}

// every calls fn every d until the scope ends or stop is called.
func every(cx *core.Scope, d time.Duration, fn func()) (stop func()) {
	stopped := false
	var cancel func() bool
	var tick func()
	tick = func() {
		if stopped {
			return
		}
		fn()
		if !stopped {
			cancel = cx.AfterFunc(d, tick)
		}
	}
	cancel = cx.AfterFunc(d, tick)
	stop = func() {
		stopped = true
		if cancel != nil {
			cancel()
		}
	}
	cx.Cleanup(stop)
	return stop
}

// toggled flips a boolean signal.
func toggled(s *core.Signal[bool]) { s.Update(func(v bool) bool { return !v }) }

// thousands formats n with commas, as en-US does.
func thousands(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + thousands(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// transparent is a color that draws nothing; a Color of 0 means unset.
var transparent = core.RGBA(0xffffff, 0)

// fillPage fills what is left of its parent along both axes and shrinks
// to it: a page whose scroll views scroll within it instead of growing it
// past the window (React Native's flex items do not shrink by default).
var fillPage = core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), MinWidth: core.Pt(0)}

// later runs fn after d under cx itself. Scope.AfterFunc belongs to the
// current owner, which in a press handler is the pressed view's: a timer
// started there ends when that view goes, as the chat's empty state does
// once a reply starts. A Post runs under cx, so the timer lives as long
// as it does.
func later(cx *core.Scope, d time.Duration, fn func()) {
	cx.Post(func() { cx.AfterFunc(d, fn) })
}

// captionf is a caption whose label follows fn.
func captionf(fn func() string) core.Node {
	return core.Text{ContentFunc: fn, Style: core.Style{FontSize: 14, Color: colors.muted}}
}
