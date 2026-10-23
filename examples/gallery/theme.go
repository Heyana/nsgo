package main

import (
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/ui"
)

// theme is the gallery's palette for one appearance and the styles the
// demos build from it. Every page and the shell take it from
// ui.DefaultTheme; the base and element demos apply it themselves, as
// those have no look of their own.
type theme struct {
	dark bool

	background, surface, sunken, sidebar core.Color
	ink, muted, line, hover              core.Color
	borderStrong, selection, raised      core.Color
	quiet, quietHover                    core.Color
	quietPressed                         core.Color
	accent, accentHover, accentPressed   core.Color
	accentSoft, onAccent                 core.Color
	danger, dangerHover, dangerPressed   core.Color
	onDanger, success, warning           core.Color
	knob                                 core.Color
}

// The radii shared by every demo. Controls take the 8-point radius; cards,
// popups and dialogs are larger surfaces.
const (
	radius       core.Points = 8
	cardRadius   core.Points = 12
	popupRadius  core.Points = 12
	dialogRadius core.Points = 16
)

// newTheme returns the gallery's palette for the appearance.
func newTheme(dark bool) *theme {
	p := ui.DefaultTheme().Tokens(dark)
	return &theme{dark: dark,
		background: p.Background, surface: p.Surface, sunken: p.Muted, sidebar: p.Muted,
		ink: p.Foreground, muted: p.MutedForeground, line: p.Border, hover: p.Accent,
		borderStrong: p.Input, selection: p.Tint(0.1), raised: p.Surface,
		quiet: p.Secondary, quietHover: p.Hover(p.Secondary), quietPressed: p.Pressed(p.Secondary),
		accent: p.Primary, accentHover: p.Hover(p.Primary), accentPressed: p.Pressed(p.Primary),
		accentSoft: p.Accent, onAccent: p.PrimaryForeground,
		danger: p.Danger, dangerHover: p.Hover(p.Danger), dangerPressed: p.Pressed(p.Danger),
		onDanger: p.DangerForeground, success: p.Success, warning: p.Warning,
		knob: core.RGB(0xffffff)}
}

// tealTheme is an app's own theme for the Theme page: a teal primary
// over cool neutrals.
func tealTheme() ui.Theme {
	return ui.Theme{
		Light: ui.Palette{
			Background: core.RGB(0xfbfdfc), Foreground: core.RGB(0x14302b),
			Surface: core.RGB(0xffffff), SurfaceForeground: core.RGB(0x14302b),
			Muted: core.RGB(0xedf4f2), MutedForeground: core.RGB(0x5a6f6a),
			Accent: core.RGB(0xe2f4ef), AccentForeground: core.RGB(0x0d4f47),
			Border: core.RGB(0xd8e4e0), Input: core.RGB(0xb9cbc6), Ring: core.RGB(0x2fb3a1),
			Primary: core.RGB(0x0b7d72), PrimaryForeground: core.RGB(0xffffff),
			Secondary: core.RGB(0xe4eeeb), SecondaryForeground: core.RGB(0x14302b),
			Danger: core.RGB(0xc8423d), DangerForeground: core.RGB(0xffffff),
			Success: core.RGB(0x2e9e62), Warning: core.RGB(0xc79a00), Info: core.RGB(0x2a9fbd),
			Overlay:   core.RGBA(0x0a1f1c, 0.32),
			Selection: core.RGBA(0x2fb3a1, 0.28), Link: core.RGB(0x0b7d72),
			Shadow: core.RGBA(0x0a1f1c, 0.14),
		},
		Dark: ui.Palette{
			Background: core.RGB(0x0e1817), Foreground: core.RGB(0xe4f1ee),
			Surface: core.RGB(0x162321), SurfaceForeground: core.RGB(0xe4f1ee),
			Muted: core.RGB(0x1c2b29), MutedForeground: core.RGB(0x93a8a3),
			Accent: core.RGB(0x1d3632), AccentForeground: core.RGB(0xd2f1ea),
			Border: core.RGB(0x283a37), Input: core.RGB(0x3d5551), Ring: core.RGB(0x5ccfb9),
			Primary: core.RGB(0x5ccfb9), PrimaryForeground: core.RGB(0x04231e),
			Secondary: core.RGB(0x223532), SecondaryForeground: core.RGB(0xe4f1ee),
			Danger: core.RGB(0xef6b65), DangerForeground: core.RGB(0x2a0806),
			Success: core.RGB(0x6fcf97), Warning: core.RGB(0xf2d63b), Info: core.RGB(0x6bcfe6),
			Overlay:   core.RGBA(0x000000, 0.5),
			Selection: core.RGBA(0x5ccfb9, 0.32), Link: core.RGB(0x5ccfb9),
			Shadow: core.RGBA(0x000000, 0.4),
		},
		Radius:      radius,
		RadiusLarge: cardRadius,
		FontSize:    15,
	}
}

var (
	row  = core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}
	wrap = core.Style{Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignCenter, Gap: 10}
)

func (t *theme) card() core.Style {
	return core.Style{Background: t.surface, CornerRadius: cardRadius, BorderWidth: 1, BorderColor: t.line,
		Padding: core.All(16), Gap: 12}
}

// tile is a small sunken box the layout and style demos arrange.
func (t *theme) tile() core.Style {
	return core.Style{Background: t.sunken, CornerRadius: radius, Padding: core.Symmetric(8, 12),
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}
}

// button is the accent-filled main action.
func (t *theme) button() core.Style {
	return core.Style{Background: t.accent, BorderWidth: 1, BorderColor: t.accent, CornerRadius: radius,
		Padding: core.Symmetric(6, 14), AlignSelf: core.AlignStart,
		Color: t.onAccent, FontWeight: core.FontWeightMedium, FontSize: 15,
		Hover:    &core.Style{Background: t.accentHover, BorderColor: t.accentHover},
		Active:   &core.Style{Background: t.accentPressed, BorderColor: t.accentPressed},
		Focus:    &core.Style{BorderColor: t.accentPressed},
		Disabled: &core.Style{Opacity: core.Alpha(0.45)}}
}

// secondary is the quiet action: a tinted face and a hairline border.
func (t *theme) secondary() core.Style {
	return core.Style{Background: t.quiet, BorderWidth: 1, BorderColor: t.line, CornerRadius: radius,
		Padding: core.Symmetric(6, 14), AlignSelf: core.AlignStart, Color: t.ink,
		FontWeight: core.FontWeightMedium, FontSize: 15,
		Hover:    &core.Style{Background: t.quietHover},
		Active:   &core.Style{Background: t.quietPressed},
		Focus:    &core.Style{BorderColor: t.accent},
		Disabled: &core.Style{Opacity: core.Alpha(0.45)}}
}

func (t *theme) dangerButton() core.Style {
	return core.Style{Background: t.danger, BorderWidth: 1, BorderColor: t.danger, CornerRadius: radius,
		Padding: core.Symmetric(6, 14), AlignSelf: core.AlignStart, Color: t.onDanger,
		FontWeight: core.FontWeightMedium, FontSize: 15,
		Hover:    &core.Style{Background: t.dangerHover, BorderColor: t.dangerHover},
		Active:   &core.Style{Background: t.dangerPressed, BorderColor: t.dangerPressed},
		Focus:    &core.Style{BorderColor: t.dangerPressed},
		Disabled: &core.Style{Opacity: core.Alpha(0.45)}}
}

// track is the recessed face behind segmented controls. It is a solid inset
// so that the raised selected tab reads on both cards and the window.
func (t *theme) track() core.Color { return t.sunken }

// withAlpha returns c at an opacity; core keeps RGBA as one value.
func withAlpha(c core.Color, a float32) core.Color {
	r, g, b, _ := c.Components()
	return core.RGBA(uint32(r*255)<<16|uint32(g*255)<<8|uint32(b*255), a)
}

func (t *theme) popup() core.Style {
	return core.Style{Background: t.surface, CornerRadius: popupRadius, BorderWidth: 1, BorderColor: t.line, Padding: core.All(6),
		Color: t.ink, ShadowColor: core.RGBA(0x000000, 0.14), ShadowRadius: 16, ShadowOffset: core.Point{Y: 4}}
}

func (t *theme) menuItem() core.Style {
	return core.Style{Padding: core.Symmetric(6, 10), CornerRadius: radius - 2, MinWidth: core.Pt(140),
		Hover: &core.Style{Background: t.hover}, Active: &core.Style{Background: t.accentSoft}}
}

func (t *theme) field() core.Style {
	return core.Style{Height: core.Pt(36), Padding: core.Symmetric(0, 10), FontSize: 15, Color: t.ink,
		Background: t.surface, CornerRadius: radius, BorderWidth: 1, BorderColor: t.line,
		Hover:    &core.Style{BorderColor: t.borderStrong},
		Focus:    &core.Style{BorderColor: t.accent},
		Disabled: &core.Style{Background: t.sunken, Color: t.muted}}
}

// navItem is a sidebar row. Selection is a soft neutral face, as Onevra draws
// it, with the accent reserved for the current label.
func (t *theme) navItem() core.Style {
	return core.Style{Padding: core.Symmetric(6, 10), CornerRadius: radius, FontSize: 14, Color: t.ink,
		Hover:    &core.Style{Background: t.hover},
		Selected: &core.Style{Background: t.selection, Color: t.selectedInk(), FontWeight: core.FontWeightMedium}}
}

// selectedInk is the accent of a selected navigation label. The light
// appearance darkens it for contrast on the selected face.
func (t *theme) selectedInk() core.Color {
	if t.dark {
		return t.accent
	}
	return t.accentPressed
}

func (t *theme) backdrop() core.Style { return core.Style{Background: core.RGBA(0x000000, 0.18)} }

func (t *theme) caption() core.Style { return core.Style{Color: t.muted, FontSize: 13} }

// primary, plain and danger are buttons labelled with text.
func (t *theme) primary(label string, onPress func()) core.Node {
	return t.labelled(t.button(), label, onPress)
}

func (t *theme) plain(label string, onPress func()) core.Node {
	return t.labelled(t.secondary(), label, onPress)
}

func (t *theme) destructive(label string, onPress func()) core.Node {
	return t.labelled(t.dangerButton(), label, onPress)
}

func (t *theme) labelled(style core.Style, label string, onPress func()) core.Node {
	return base.Button{OnPress: onPress, Label: label, Style: style, Children: []core.Node{core.Text{Content: label}}}
}

// demo is one card of a section: a title, a line saying what it shows
// and the demo itself.
func (t *theme) demo(title, about string, children ...core.Node) core.Node {
	head := []core.Node{
		core.Text{Content: title, Role: core.RoleHeader, Style: core.Style{FontSize: 15, FontWeight: core.FontWeightSemibold}},
		core.Text{Content: about, Style: t.caption()},
	}
	return core.View{Style: t.card(), Children: append(head, children...)}
}

// note is a muted line of text inside a demo.
func (t *theme) note(text string) core.Node { return core.Text{Content: text, Style: t.caption()} }

// live is a muted line showing what fn returns.
func (t *theme) live(fn func() string) core.Node {
	return core.Text{ContentFunc: fn, Style: t.caption()}
}

// hstack lays nodes out in a row.
func hstack(nodes ...core.Node) core.Node { return core.View{Style: row, Children: nodes} }

// flow lays nodes out in a row that wraps.
func flow(nodes ...core.Node) core.Node { return core.View{Style: wrap, Children: nodes} }
