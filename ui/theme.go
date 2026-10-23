// Package ui holds styled components: base's behavior with the look of a
// theme, which apps adjust with Style or replace with a Theme of their own.
package ui

import "github.com/oneo-me/nsgo/core"

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
		FontSize:    15,
	}
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

// The looks of the components.

var disabled = &core.Style{Opacity: core.Alpha(0.5)}

func (t Tokens) button(v Variant) core.Style {
	bg, fg, border := t.Primary, t.PrimaryForeground, t.Primary
	switch v {
	case Secondary:
		bg, fg, border = t.Secondary, t.SecondaryForeground, t.Border
	case Danger:
		bg, fg, border = t.Danger, t.DangerForeground, t.Danger
	case Ghost:
		return core.Style{CornerRadius: t.Radius, Padding: core.Symmetric(6, 14), AlignSelf: core.AlignStart,
			Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 6,
			Color: t.Foreground, FontSize: t.FontSize, FontWeight: core.FontWeightMedium,
			Hover:    &core.Style{Background: t.Accent, Color: t.AccentForeground},
			Active:   &core.Style{Background: t.Pressed(t.Accent), Color: t.AccentForeground},
			Disabled: disabled}
	}
	return core.Style{Background: bg, BorderWidth: 1, BorderColor: border, CornerRadius: t.Radius,
		Padding: core.Symmetric(6, 14), AlignSelf: core.AlignStart,
		Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 6,
		Color: fg, FontSize: t.FontSize, FontWeight: core.FontWeightMedium,
		Hover:    &core.Style{Background: t.Hover(bg)},
		Active:   &core.Style{Background: t.Pressed(bg)},
		Disabled: disabled}
}

// control is a checkbox's, radio's or switch's row: its indicator, then
// its label.
func (t Tokens) control() core.Style {
	return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, AlignSelf: core.AlignStart, Gap: 8,
		Color: t.Foreground, FontSize: t.FontSize, Disabled: disabled}
}

func (t Tokens) checkboxIndicator() core.Style {
	on := &core.Style{Background: t.Primary, BorderColor: t.Primary}
	return core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: t.Radius / 2,
		BorderWidth: 1, BorderColor: t.Input, Background: t.Background,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
		Color: t.PrimaryForeground, FontSize: 12, FontWeight: core.FontWeightBold,
		Hover: &core.Style{BorderColor: t.Ring}, Checked: on, Indeterminate: on}
}

func (t Tokens) radioIndicator() core.Style {
	return core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 9,
		BorderWidth: 1, BorderColor: t.Input, Background: t.Background,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter,
		Checked: &core.Style{BorderWidth: 5, BorderColor: t.Primary}}
}

func (t Tokens) radioGroup() core.Style { return core.Style{Gap: 8} }

func (t Tokens) switchTrack() core.Style {
	return core.Style{Width: core.Pt(40), Height: core.Pt(24), CornerRadius: 12, Padding: core.All(2),
		Background: t.Input, Transition: core.Transition{Duration: 150e6, Easing: core.EaseOut},
		Checked: &core.Style{Background: t.Primary}}
}

func (t Tokens) switchThumb() core.Style {
	return core.Style{Width: core.Pt(20), Height: core.Pt(20), CornerRadius: 10, Background: core.RGB(0xffffff),
		ShadowColor: t.Shadow, ShadowRadius: 2, ShadowOffset: core.Point{Y: 1},
		Transition: core.Transition{Duration: 150e6, Easing: core.EaseOut},
		Checked:    &core.Style{Transform: core.Transform{TranslateX: 16}}}
}

func (t Tokens) tabs() core.Style {
	return core.Style{Direction: core.Row, AlignSelf: core.AlignStart, Padding: core.All(3), Gap: 2,
		Background: t.Muted, CornerRadius: t.Radius}
}

func (t Tokens) tab() core.Style {
	return core.Style{Padding: core.Symmetric(5, 12), CornerRadius: t.Radius - 2,
		Color: t.MutedForeground, FontSize: t.FontSize, FontWeight: core.FontWeightMedium,
		Hover:    &core.Style{Color: t.Foreground},
		Selected: &core.Style{Background: t.Background, Color: t.Foreground, ShadowColor: t.Shadow, ShadowRadius: 2, ShadowOffset: core.Point{Y: 1}},
		Disabled: disabled}
}

func (t Tokens) popup() core.Style {
	return core.Style{Background: t.Surface, Color: t.SurfaceForeground, FontSize: t.FontSize,
		CornerRadius: t.RadiusLarge, BorderWidth: 1, BorderColor: t.Border, Padding: core.All(6),
		ShadowColor: t.Shadow, ShadowRadius: 16, ShadowOffset: core.Point{Y: 4}}
}

func (t Tokens) menuItem() core.Style {
	return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8,
		Padding: core.Symmetric(6, 10), CornerRadius: t.Radius - 2, MinWidth: core.Pt(160),
		Hover:    &core.Style{Background: t.Accent, Color: t.AccentForeground},
		Active:   &core.Style{Background: t.Pressed(t.Accent), Color: t.AccentForeground},
		Disabled: disabled}
}

func (t Tokens) menuSeparator() core.Style {
	return core.Style{Height: core.Pt(1), Background: t.Border, Margin: core.Symmetric(4, 0)}
}

func (t Tokens) tooltip() core.Style {
	return core.Style{Background: t.Foreground, Color: t.Background, FontSize: t.FontSize - 2,
		CornerRadius: t.Radius - 2, Padding: core.Symmetric(4, 8)}
}

func (t Tokens) dialog() core.Style {
	return core.Style{Background: t.Surface, Color: t.SurfaceForeground, FontSize: t.FontSize,
		CornerRadius: t.RadiusLarge, BorderWidth: 1, BorderColor: t.Border, Padding: core.All(24), Gap: 12,
		MinWidth: core.Pt(280), MaxWidth: core.Pct(90),
		ShadowColor: t.Shadow, ShadowRadius: 24, ShadowOffset: core.Point{Y: 8}}
}

func (t Tokens) sheet(side core.Placement) core.Style {
	s := core.Style{Background: t.Surface, Color: t.SurfaceForeground, FontSize: t.FontSize,
		Padding: core.All(24), Gap: 12, ShadowColor: t.Shadow, ShadowRadius: 24}
	switch side {
	case core.PlaceLeft, core.PlaceRight:
		s.Width, s.MaxWidth = core.Pt(320), core.Pct(85)
	default:
		s.CornerRadius = t.RadiusLarge
	}
	return s
}

func (t Tokens) backdrop() core.Style { return core.Style{Background: t.Overlay} }

// iconButton has no face until hovered or pressed, when the foreground
// tints it.
func (t Tokens) iconButton() core.Style {
	return core.Style{Width: core.Pt(32), Height: core.Pt(32), FlexShrink: 0, CornerRadius: t.Radius,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Color: t.MutedForeground,
		Hover:    &core.Style{Background: t.Tint(0.08)},
		Active:   &core.Style{Background: t.Tint(0.14)},
		Disabled: disabled}
}

func (t Tokens) field() core.Style {
	return core.Style{Height: core.Pt(36), Padding: core.Symmetric(0, 10), CornerRadius: t.Radius,
		BorderWidth: 1, BorderColor: t.Border, Background: t.Background, Color: t.Foreground, FontSize: t.FontSize,
		Hover:    &core.Style{BorderColor: t.Input},
		Focus:    &core.Style{BorderColor: t.Ring},
		Disabled: disabled}
}

func (t Tokens) selectField() core.Style {
	return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, AlignSelf: core.AlignStart, Gap: 8,
		MinWidth: core.Pt(140), Height: core.Pt(34), Padding: core.Symmetric(0, 10), CornerRadius: t.Radius,
		BorderWidth: 1, BorderColor: t.Border, Background: t.Background, Color: t.Foreground, FontSize: t.FontSize,
		Hover:  &core.Style{Background: t.Hover(t.Background)},
		Active: &core.Style{Background: t.Pressed(t.Background)}}
}

func (t Tokens) slider() core.Style {
	return core.Style{Height: core.Pt(20), MinWidth: core.Pt(120), JustifyContent: core.JustifyCenter, Disabled: disabled}
}

func (t Tokens) sliderTrack() core.Style {
	return core.Style{Height: core.Pt(4), CornerRadius: 2, Background: t.Tint(0.12)}
}

func (t Tokens) sliderFill() core.Style {
	return core.Style{CornerRadius: 2, Background: t.Primary}
}

func (t Tokens) sliderThumb() core.Style {
	return core.Style{Width: core.Pt(16), Height: core.Pt(16), CornerRadius: 8, Background: core.RGB(0xffffff),
		BorderWidth: 1, BorderColor: core.RGBA(0x000000, 0.08),
		ShadowColor: t.Shadow, ShadowRadius: 3, ShadowOffset: core.Point{Y: 1}}
}

// badge is a pill: neutral on the surface, or washed with its tone's color.
func (t Tokens) badge(tone Tone) core.Style {
	bg, fg, border := t.Surface, t.Foreground, t.Border
	if tint, ok := t.tone(tone); ok {
		bg, fg, border = Mix(t.Background, tint, 0.12), Mix(tint, t.Foreground, 0.3), Mix(t.Background, tint, 0.3)
	}
	return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, AlignSelf: core.AlignStart, FlexShrink: 0, Gap: 4,
		Padding: core.Symmetric(2, 8), CornerRadius: 12, BorderWidth: 1, BorderColor: border, Background: bg,
		Color: fg, FontSize: t.small(), FontWeight: core.FontWeightMedium}
}

// tone returns a tone's color, or false for the neutral tone.
func (t Tokens) tone(tone Tone) (core.Color, bool) {
	switch tone {
	case ToneAccent:
		return t.Primary, true
	case ToneSuccess:
		return t.Success, true
	case ToneWarning:
		return t.Warning, true
	case ToneDanger:
		return t.Danger, true
	case ToneInfo:
		return t.Info, true
	}
	return 0, false
}

// small is the font size of captions, badges and descriptions.
func (t Tokens) small() core.Points { return core.Points(float32(t.FontSize)*0.85 + 0.5) }

func (t Tokens) progress() core.Style {
	return core.Style{Height: core.Pt(6), CornerRadius: 3, Background: t.Tint(0.1), Overflow: core.Hidden}
}
