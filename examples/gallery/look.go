package main

import (
	"time"

	"github.com/oneo-me/nsgo/core"
)

// The gallery's look, Onevra's palette: a warm page, cards with an inset
// surface and one orange accent, the logo's. Sizes are for phones: 16 pt
// gutters, 44 pt touch targets, 15 pt body text. The desktop layout keeps
// them and puts more side by side. base has no look, so every color and
// finish here is the gallery's own. Components take the scope they are
// built in and read their colors there (colorsOf, tintsOf, finOf), so a
// subtree's theme or forced appearance reaches them; the root rebuilds
// when its palette changes.

// palette is the gallery's colors in one appearance: the theme's roles
// under the gallery's names, and the ones galleryTheme adds.
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

// galleryTheme returns the gallery's theme: Onevra's (core.DefaultTheme), with
// the roles the gallery adds in Palette.Colors.
func galleryTheme() *core.Theme {
	t := core.DefaultTheme()
	t.Light.Colors = map[string]core.Color{
		"inset": core.RGB(0xf2efea), "raised": core.RGB(0xfcfbf9), "field": core.RGB(0xebe8e1), "fieldBorder": core.RGB(0xd3cfc6),
		"line": core.RGB(0xe5e1d9), "faint": core.RGB(0xa29e95), "accentFill": core.RGB(0xd4581b),
		"light": core.RGB(0x2d2b27), "onLight": core.RGB(0xffffff), "track": core.RGB(0xe5e1d9), "knob": core.RGB(0xffffff),
		"code": core.RGB(0x3a3631), "info": core.RGB(0x3d63d1), "down": core.RGB(0xb4444c),
	}
	t.Dark.Colors = map[string]core.Color{
		"inset": core.RGB(0x151412), "raised": core.RGB(0x2e2b27), "field": core.RGB(0x151412), "fieldBorder": core.RGB(0x3b3732),
		"line": core.RGB(0x2e2b27), "faint": core.RGB(0x6a645b), "accentFill": core.RGB(0xed986e),
		"light": core.RGB(0xeeeae2), "onLight": core.RGB(0x1f1d1a), "track": core.RGB(0x2e2b27), "knob": core.RGB(0xa39c90),
		"code": core.RGB(0xe2dcd2), "info": core.RGB(0x7ea8ff), "down": core.RGB(0xe58f95),
	}
	return t
}

// colorsOf is the gallery's colors where cx is: its theme's palette in the
// appearance shown there.
func colorsOf(cx *core.Scope) *palette {
	p := cx.Palette()
	c := p.Colors
	return &palette{
		background: p.Background, surface: p.Surface, inset: c["inset"], raised: c["raised"],
		field: c["field"], fieldBorder: c["fieldBorder"], border: p.Border, line: c["line"],
		text: p.Text, muted: p.TextMuted, faint: c["faint"],
		accent: p.Accent, accentFill: c["accentFill"], onAccent: p.AccentText,
		light: c["light"], onLight: c["onLight"], track: c["track"], knob: c["knob"],
		hover: p.SurfaceHover, pressed: p.SurfacePressed, code: c["code"],
		info: c["info"], warn: p.Warning, danger: p.Danger, up: p.Success, down: c["down"],
	}
}

// tintsOf is the tones' tints where cx is.
func tintsOf(cx *core.Scope) tintSet {
	c := colorsOf(cx)
	if cx.Dark() {
		return tintSet{alpha(c.accent, 0.14), alpha(c.info, 0.16), alpha(c.warn, 0.16),
			alpha(c.danger, 0.16), alpha(c.up, 0.16), alpha(c.down, 0.16)}
	}
	return tintSet{alpha(c.accentFill, 0.12), alpha(c.info, 0.10), alpha(c.warn, 0.11),
		alpha(c.danger, 0.10), alpha(c.up, 0.10), alpha(c.down, 0.10)}
}

// finOf is the finishes of the appearance shown where cx is.
func finOf(cx *core.Scope) *finishes {
	if cx.Dark() {
		return &darkFinishes
	}
	return &lightFinishes
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

// black, White and Paper are black, white and the warm shadow color at
// opacity a.
// black, white and paper are black, white and the warm shadow color at
// opacity a.
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
func (t tone) color(cx *core.Scope) core.Color {
	colors := colorsOf(cx)
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
func (t tone) tint(cx *core.Scope) core.Color {
	colors := colorsOf(cx)
	tints := tintsOf(cx)
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
func hovered(cx *core.Scope) *core.Style {
	colors := colorsOf(cx)
	return &core.Style{Background: alpha(colors.text, 0.05)}
}

// focusRing is the keyboard focus: a 2-point ring of info inside the
// view's edge (offset 2 puts it outside, around a filled button).
func focusRing(cx *core.Scope, offset core.Points) *core.Style {
	colors := colorsOf(cx)
	return &core.Style{OutlineWidth: 2, OutlineColor: colors.info, OutlineOffset: offset}
}
