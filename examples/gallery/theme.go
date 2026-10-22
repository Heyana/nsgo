package main

import (
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/ui"
)

// theme is a page's palette for one appearance and the styles the demos
// build from it. Base pages are black and white, as base has no look of
// its own: whatever color they show is the app's. UI pages and the shell
// take theirs from ui.DefaultTheme, whose palette comes from NSGo's icon.
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
	knob, switchOff                      core.Color
}

// The radii shared by every demo. Controls take the 8-point radius; cards,
// popups and dialogs are larger surfaces.
const (
	radius       core.Points = 8
	cardRadius   core.Points = 12
	popupRadius  core.Points = 12
	dialogRadius core.Points = 16
)

// newTheme returns the palette of a category's pages.
func newTheme(dark bool, k kind) *theme {
	if k == kindBase {
		return monoTheme(dark)
	}
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
		knob: core.RGB(0xffffff), switchOff: p.Input}
}

// monoTheme is black and white with grays between.
func monoTheme(dark bool) *theme {
	if dark {
		ink := core.RGB(0xf2f2f2)
		return &theme{dark: true,
			background: core.RGB(0x0a0a0a), surface: core.RGB(0x121212), sunken: core.RGB(0x1c1c1c), sidebar: core.RGB(0x0f0f0f),
			ink: ink, muted: core.RGB(0x9a9a9a), line: core.RGB(0x2e2e2e), hover: core.RGB(0x1f1f1f),
			borderStrong: core.RGB(0x5a5a5a), selection: core.RGBA(0xffffff, 0.1), raised: core.RGB(0x1c1c1c),
			quiet: core.RGB(0x1c1c1c), quietHover: core.RGB(0x262626), quietPressed: core.RGB(0x303030),
			accent: ink, accentHover: core.RGB(0xffffff), accentPressed: core.RGB(0xd0d0d0),
			accentSoft: core.RGB(0x262626), onAccent: core.RGB(0x0a0a0a),
			danger: ink, dangerHover: core.RGB(0xffffff), dangerPressed: core.RGB(0xd0d0d0), onDanger: core.RGB(0x0a0a0a),
			success: ink, warning: core.RGB(0xbdbdbd), knob: core.RGB(0x0a0a0a), switchOff: core.RGB(0x5a5a5a)}
	}
	ink := core.RGB(0x111111)
	return &theme{
		background: core.RGB(0xffffff), surface: core.RGB(0xffffff), sunken: core.RGB(0xf4f4f4), sidebar: core.RGB(0xf7f7f7),
		ink: ink, muted: core.RGB(0x6b6b6b), line: core.RGB(0xe0e0e0), hover: core.RGB(0xf0f0f0),
		borderStrong: core.RGB(0xa8a8a8), selection: core.RGBA(0x000000, 0.06), raised: core.RGB(0xffffff),
		quiet: core.RGB(0xf2f2f2), quietHover: core.RGB(0xe9e9e9), quietPressed: core.RGB(0xdedede),
		accent: ink, accentHover: core.RGB(0x2b2b2b), accentPressed: core.RGB(0x000000),
		accentSoft: core.RGB(0xededed), onAccent: core.RGB(0xffffff),
		danger: ink, dangerHover: core.RGB(0x2b2b2b), dangerPressed: core.RGB(0x000000), onDanger: core.RGB(0xffffff),
		success: ink, warning: core.RGB(0x555555), knob: core.RGB(0xffffff), switchOff: core.RGB(0xc8c8c8)}
}

// warmTheme is an app's own theme for the Theme page: Onevra's warm
// neutrals and terracotta accent.
func warmTheme() ui.Theme {
	return ui.Theme{
		Light: ui.Palette{
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
		Dark: ui.Palette{
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
