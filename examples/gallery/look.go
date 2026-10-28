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
// subtree's theme or forced appearance reaches them. They read them in
// StyleFuncs, Draws and the like, which follow a change of theme or
// appearance in place, as NSH's styles do: nothing is built again.

// look is the gallery's colors where a scope is: each method reads its
// color from the palette shown there, the theme's roles under the
// gallery's names and the ones galleryTheme adds. A StyleFunc or Draw
// that calls one follows the palette; a method value (colors.muted) is a
// color to hand to what reads it so.
type look struct{ cx *core.Scope }

// colorsOf is the gallery's colors where cx is.
func colorsOf(cx *core.Scope) look { return look{cx} }

func (l look) role(name string) core.Color { return l.cx.Palette().Colors[name] }

// background is the page behind the cards; surface a card; inset a
// surface inside a card; raised a control on a card.
func (l look) background() core.Color { return l.cx.Palette().Background }
func (l look) surface() core.Color    { return l.cx.Palette().Surface }
func (l look) inset() core.Color      { return l.role("inset") }
func (l look) raised() core.Color     { return l.role("raised") }

// field is an input's well, below the card and the insets it sits on;
// fieldBorder its edge.
func (l look) field() core.Color       { return l.role("field") }
func (l look) fieldBorder() core.Color { return l.role("fieldBorder") }
func (l look) border() core.Color      { return l.cx.Palette().Border }

// line is the hairlines inside insets.
func (l look) line() core.Color  { return l.role("line") }
func (l look) text() core.Color  { return l.cx.Palette().Text }
func (l look) muted() core.Color { return l.cx.Palette().TextMuted }
func (l look) faint() core.Color { return l.role("faint") }

// accent is the accent as a foreground: text, icons, lines, small marks.
// accentFill is the accent as a fill, with onAccent on it.
func (l look) accent() core.Color     { return l.cx.Palette().Accent }
func (l look) accentFill() core.Color { return l.role("accentFill") }
func (l look) onAccent() core.Color   { return l.cx.Palette().AccentText }

// light is the main action's button, light on the dark page and dark on
// the light one, with onLight on it.
func (l look) light() core.Color   { return l.role("light") }
func (l look) onLight() core.Color { return l.role("onLight") }

// track and knob are a switch's when off.
func (l look) track() core.Color { return l.role("track") }
func (l look) knob() core.Color  { return l.role("knob") }

// hover is a row under the pointer, pressed one held down.
func (l look) hover() core.Color   { return l.cx.Palette().SurfaceHover }
func (l look) pressed() core.Color { return l.cx.Palette().SurfacePressed }

// code is code inside insets.
func (l look) code() core.Color   { return l.role("code") }
func (l look) info() core.Color   { return l.role("info") }
func (l look) warn() core.Color   { return l.cx.Palette().Warning }
func (l look) danger() core.Color { return l.cx.Palette().Danger }
func (l look) up() core.Color     { return l.cx.Palette().Success }
func (l look) down() core.Color   { return l.role("down") }

// colored is a StyleFunc that colors text and icons c.
func colored(c func() core.Color) func() core.Style {
	return func() core.Style { return core.Style{Color: c()} }
}

// filled is a StyleFunc that fills a view with c.
func filled(c func() core.Color) func() core.Style {
	return func() core.Style { return core.Style{Background: c()} }
}

// palette is the gallery's colors at one moment, for code that reads
// many at once, such as a Draw.
type palette struct {
	background, surface, inset, raised, field, fieldBorder, border, line core.Color
	text, muted, faint, accent, accentFill, onAccent, light, onLight     core.Color
	track, knob, hover, pressed, code, info, warn, danger, up, down      core.Color
}

// now is the colors where l is as they are now; a Draw that calls it
// draws again when they change.
func (l look) now() palette {
	p := l.cx.Palette()
	c := p.Colors
	return palette{
		background: p.Background, surface: p.Surface, inset: c["inset"], raised: c["raised"],
		field: c["field"], fieldBorder: c["fieldBorder"], border: p.Border, line: c["line"],
		text: p.Text, muted: p.TextMuted, faint: c["faint"],
		accent: p.Accent, accentFill: c["accentFill"], onAccent: p.AccentText,
		light: c["light"], onLight: c["onLight"], track: c["track"], knob: c["knob"],
		hover: p.SurfaceHover, pressed: p.SurfacePressed, code: c["code"],
		info: c["info"], warn: p.Warning, danger: p.Danger, up: p.Success, down: c["down"],
	}
}

// themed is what build returns, built again when what it reads (the
// palette) changes: a component's styles by part, for its StylesFunc.
func themed[T any](cx *core.Scope, build func() T) func() T {
	return core.NewMemoFunc(cx, build, nil).Get
}

// fixed is c as a color to hand to what takes one that may change.
func fixed(c core.Color) func() core.Color { return func() core.Color { return c } }

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

// tints are translucent tints of the tones where a scope is, for fills
// behind text, read as colorsOf's are.
type tints struct{ cx *core.Scope }

// tintsOf is the tones' tints where cx is.
func tintsOf(cx *core.Scope) tints { return tints{cx} }

// tint is c at the opacity of a tint in the appearance shown.
func (t tints) tint(c core.Color, light, dark float32) core.Color {
	if t.cx.Dark() {
		return alpha(c, dark)
	}
	return alpha(c, light)
}

func (t tints) accent() core.Color {
	colors := colorsOf(t.cx)
	if t.cx.Dark() {
		return alpha(colors.accent(), 0.14)
	}
	return alpha(colors.accentFill(), 0.12)
}
func (t tints) info() core.Color   { return t.tint(colorsOf(t.cx).info(), 0.10, 0.16) }
func (t tints) warn() core.Color   { return t.tint(colorsOf(t.cx).warn(), 0.11, 0.16) }
func (t tints) danger() core.Color { return t.tint(colorsOf(t.cx).danger(), 0.10, 0.16) }
func (t tints) up() core.Color     { return t.tint(colorsOf(t.cx).up(), 0.10, 0.16) }
func (t tints) down() core.Color   { return t.tint(colorsOf(t.cx).down(), 0.10, 0.16) }

// fins are the finishes where a scope is, read as colorsOf's colors
// are.
type fins struct{ cx *core.Scope }

// finOf is the finishes where cx is.
func finOf(cx *core.Scope) fins { return fins{cx} }

// shown is the finishes of the appearance shown.
func (f fins) shown() *finishes {
	if f.cx.Dark() {
		return &darkFinishes
	}
	return &lightFinishes
}

func (f fins) card() finish   { return f.shown().card }
func (f fins) inset() finish  { return f.shown().inset }
func (f fins) field() finish  { return f.shown().field }
func (f fins) raised() finish { return f.shown().raised }
func (f fins) light() finish  { return f.shown().light }
func (f fins) accent() finish { return f.shown().accent }
func (f fins) track() finish  { return f.shown().track }
func (f fins) knob() finish   { return f.shown().knob }
func (f fins) pill() finish   { return f.shown().pill }
func (f fins) chip() finish   { return f.shown().chip }
func (f fins) glass() finish  { return f.shown().glass }
func (f fins) menu() finish   { return f.shown().menu }

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

// color is the tone's foreground, read where colorsOf's colors are.
func (t tone) color(cx *core.Scope) core.Color {
	colors := colorsOf(cx)
	switch t {
	case toneAccent:
		return colors.accent()
	case toneInfo:
		return colors.info()
	case toneWarn:
		return colors.warn()
	case toneDanger:
		return colors.danger()
	case toneUp:
		return colors.up()
	case toneDown:
		return colors.down()
	}
	return colors.muted()
}

// shade is the tone's color, for what takes one that may change.
func (t tone) shade(cx *core.Scope) func() core.Color {
	return func() core.Color { return t.color(cx) }
}

// foreground is the tone's color for text and icons: the text's when
// neutral.
func (t tone) foreground(cx *core.Scope) func() core.Color {
	return func() core.Color {
		if t == toneNeutral {
			return colorsOf(cx).text()
		}
		return t.color(cx)
	}
}

// tint is the tone's translucent fill, read where colorsOf's colors are.
func (t tone) tint(cx *core.Scope) core.Color {
	tints := tintsOf(cx)
	switch t {
	case toneAccent:
		return tints.accent()
	case toneInfo:
		return tints.info()
	case toneWarn:
		return tints.warn()
	case toneDanger:
		return tints.danger()
	case toneUp:
		return tints.up()
	case toneDown:
		return tints.down()
	}
	return colorsOf(cx).raised()
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
// over its surface, quieter than pressed and than selected. Like the
// rest of a style, read it in a StyleFunc.
func hovered(cx *core.Scope) *core.Style {
	return &core.Style{Background: alpha(colorsOf(cx).text(), 0.05)}
}

// focusRing is the keyboard focus: a 2-point ring of info inside the
// view's edge (offset 2 puts it outside, around a filled button), read
// in a StyleFunc.
func focusRing(cx *core.Scope, offset core.Points) *core.Style {
	return &core.Style{OutlineWidth: 2, OutlineColor: colorsOf(cx).info(), OutlineOffset: offset}
}
