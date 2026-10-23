// Package ui holds styled components: controls, fields, popups, surfaces
// and composites with the look of a theme, which apps adjust with Style
// or replace with a Theme of their own.
package ui

import (
	"runtime"

	"github.com/oneo-me/nsgo/core"
)

// Theme is the look of ui's components: semantic colors for the light and
// the dark appearance, corner radii and the base font size.
type Theme struct {
	Light, Dark Palette
	// Radius rounds controls; RadiusLarge rounds popups, dialogs and sheets.
	Radius, RadiusLarge core.Points
	FontSize            core.Points
}

// Palette is a theme's colors for one appearance. Hovered and pressed
// shades are derived from them.
type Palette struct {
	// Background and Foreground are the app's surface and its text.
	Background, Foreground core.Color
	// Surface and SurfaceForeground are raised layers: popups, dialogs and
	// sheets.
	Surface, SurfaceForeground core.Color
	// Muted is a quiet surface, such as a tab list; MutedForeground is
	// secondary text.
	Muted, MutedForeground core.Color
	// Accent marks the hovered item of a menu.
	Accent, AccentForeground core.Color
	// Border separates; Input outlines checkboxes, radios and an off switch;
	// Ring shows focus.
	Border, Input, Ring core.Color
	// Primary is the main action; Secondary a quiet one; Danger a
	// destructive one. Each has the color of what sits on it.
	Primary, PrimaryForeground     core.Color
	Secondary, SecondaryForeground core.Color
	Danger, DangerForeground       core.Color
	// Success, Warning and Info are status colors.
	Success, Warning, Info core.Color
	// Overlay dims what lies under a dialog or a sheet.
	Overlay core.Color
	// Selection is the background of selected text; Link colors links.
	Selection, Link core.Color
	// Shadow is the shadow of raised layers.
	Shadow core.Color
}

// DefaultTheme returns NSGo's theme, after Onevra's: warm neutral surfaces
// with a terracotta primary, lifted in the dark appearance for legibility.
func DefaultTheme() Theme {
	return Theme{
		Light: Palette{
			Background: core.RGB(0xfcfbf9), Foreground: core.RGB(0x33332f),
			Surface: core.RGB(0xfefefd), SurfaceForeground: core.RGB(0x33332f),
			Muted: core.RGB(0xf2f0ec), MutedForeground: core.RGB(0x72726a),
			Accent: core.RGB(0xeceae4), AccentForeground: core.RGB(0x33332f),
			Border: core.RGB(0xdeded5), Input: core.RGB(0xc4c4bc), Ring: core.RGBA(0xb94716, 0.55),
			Primary: core.RGB(0xbd481b), PrimaryForeground: core.RGB(0xffffff),
			Secondary: core.RGB(0xecebe9), SecondaryForeground: core.RGB(0x33332f),
			Danger: core.RGB(0xb4444c), DangerForeground: core.RGB(0xffffff),
			Success: core.RGB(0x059669), Warning: core.RGB(0xb45309), Info: core.RGB(0x3e7f96),
			Overlay:   core.RGBA(0x000000, 0.18),
			Selection: core.RGBA(0xe6652b, 0.22), Link: core.RGB(0xbd481b),
			Shadow: core.RGBA(0x000000, 0.14),
		},
		Dark: Palette{
			Background: core.RGB(0x211f1c), Foreground: core.RGB(0xeeeae2),
			Surface: core.RGB(0x2b2925), SurfaceForeground: core.RGB(0xeeeae2),
			Muted: core.RGB(0x1a1916), MutedForeground: core.RGB(0xbdb6aa),
			Accent: core.RGB(0x36332e), AccentForeground: core.RGB(0xeeeae2),
			Border: core.RGB(0x504b43), Input: core.RGB(0x68635b), Ring: core.RGBA(0xed986e, 0.8),
			Primary: core.RGB(0xed986e), PrimaryForeground: core.RGB(0x291b14),
			Secondary: core.RGB(0x312f2c), SecondaryForeground: core.RGB(0xeeeae2),
			Danger: core.RGB(0xe58f95), DangerForeground: core.RGB(0x291b14),
			Success: core.RGB(0x79bea1), Warning: core.RGB(0xe6b567), Info: core.RGB(0x86bfd3),
			Overlay:   core.RGBA(0x000000, 0.45),
			Selection: core.RGBA(0xed986e, 0.24), Link: core.RGB(0xed986e),
			Shadow: core.RGBA(0x000000, 0.4),
		},
		Radius:      8,
		RadiusLarge: 12,
		FontSize:    defaultFontSize(),
	}
}

// defaultFontSize is the size of the theme's text: 13 on macOS, as its
// controls are, and 14 elsewhere, as MyGo's default theme is.
func defaultFontSize() core.Points {
	if runtime.GOOS == "darwin" {
		return 13
	}
	return 14
}

// Tokens is a theme resolved for the current appearance.
type Tokens struct {
	Palette
	Dark                bool
	Radius, RadiusLarge core.Points
	FontSize            core.Points
}

// Tokens resolves the theme for the dark or the light appearance.
func (t Theme) Tokens(dark bool) Tokens {
	p := t.Light
	if dark {
		p = t.Dark
	}
	return Tokens{Palette: p, Dark: dark, Radius: t.Radius, RadiusLarge: t.RadiusLarge, FontSize: t.FontSize}
}

// Hover returns c as hovered: 6% toward black in the light appearance and
// toward white in the dark.
func (t Tokens) Hover(c core.Color) core.Color { return t.shade(c, 0.06) }

// Pressed returns c as pressed: 12% toward black in the light appearance
// and toward white in the dark.
func (t Tokens) Pressed(c core.Color) core.Color { return t.shade(c, 0.12) }

func (t Tokens) shade(c core.Color, amount float64) core.Color {
	to := core.RGB(0x000000)
	if t.Dark {
		to = core.RGB(0xffffff)
	}
	// The shade keeps c's alpha.
	return Mix(c, to, amount)&^0xff | c&0xff
}

// Tint returns the foreground at alpha: the neutral wash of a hovered or
// selected row, which reads on any surface of the theme.
func (t Tokens) Tint(alpha float64) core.Color { return Fade(t.Foreground, alpha) }

// Mix returns a blended amount of the way to b, alpha included.
func Mix(a, b core.Color, amount float64) core.Color {
	ar, ag, ab, aa := a.Components()
	br, bg, bb, ba := b.Components()
	mix := func(x, y float64) uint32 { return uint32((x+(y-x)*amount)*255 + 0.5) }
	return core.Color(mix(ar, br)<<24 | mix(ag, bg)<<16 | mix(ab, bb)<<8 | mix(aa, ba))
}

// Fade returns c with its alpha set to alpha.
func Fade(c core.Color, alpha float64) core.Color {
	return c&^0xff | core.Color(uint32(alpha*255+0.5)&0xff)
}

var (
	themeKey = core.NewKey[func() Theme]("theme", nil)
	darkKey  = core.NewKey[func() bool]("dark", nil)
)

// UseTheme returns what reads the theme the nearest ThemeView provides, or
// the default theme, for the appearance it chooses or else the system's.
// Apps style their own views with it. Calls of what it returns subscribe
// to both, so a style function using it follows them.
func UseTheme(cx *core.Scope) func() Tokens {
	theme, dark := themeKey.Use(cx), darkKey.Use(cx)
	if dark == nil {
		dark = cx.Env().Dark
	}
	return func() Tokens {
		if theme == nil {
			return DefaultTheme().Tokens(dark())
		}
		return theme().Tokens(dark())
	}
}

// ThemeView provides a theme to the components below it and paints its
// background, text color and font size. With no Theme it provides the
// default one.
type ThemeView struct {
	Theme func() Theme
	// Dark, when set, chooses the dark or the light palette below the view
	// instead of the system's appearance, for an app's own appearance
	// setting. Native parts the platform draws, such as window controls,
	// keep the system's.
	Dark     func() bool
	Style    core.Style
	Children []core.Node
	TestID   string
}

// Render implements core.Node.
func (v ThemeView) Render(cx *core.Scope) core.Node {
	if v.Theme != nil {
		themeKey.Provide(cx, v.Theme)
	}
	if v.Dark != nil {
		darkKey.Provide(cx, v.Dark)
	}
	theme := UseTheme(cx)
	return core.View{
		StyleFunc: func() core.Style {
			t := theme()
			return core.Style{Background: t.Background, Color: t.Foreground, FontSize: t.FontSize,
				SelectionColor: t.Selection}.Merge(v.Style)
		},
		Children: v.Children,
		TestID:   v.TestID,
	}
}
