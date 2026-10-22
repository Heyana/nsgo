package main

import (
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// theme is the gallery's palette for one appearance and the styles built
// from it. NSGo itself has no look of its own, so the gallery follows the
// Onevra design language: warm neutral surfaces, one terracotta accent, soft
// borders and 8-point control radii. Every demo takes its colors and metrics
// from here.
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

func newTheme(dark bool) *theme {
	if dark {
		return &theme{dark: true,
			background: core.RGB(0x211f1c), surface: core.RGB(0x2b2925), sunken: core.RGB(0x1a1916), sidebar: core.RGB(0x1b1a17),
			ink: core.RGB(0xeeeae2), muted: core.RGB(0xbdb6aa), line: core.RGB(0x504b43), hover: core.RGB(0x36332e),
			borderStrong: core.RGB(0x78736b), selection: core.RGBA(0xeeeae2, 0.10), raised: core.RGB(0x36332e),
			quiet: core.RGB(0x312f2c), quietHover: core.RGB(0x3a3734), quietPressed: core.RGB(0x464440),
			accent: core.RGB(0xed986e), accentHover: core.RGB(0xefa47f), accentPressed: core.RGB(0xd18661),
			accentSoft: core.RGB(0x392e26), onAccent: core.RGB(0x291b14),
			danger: core.RGB(0xe58f95), dangerHover: core.RGB(0xe89ca2), dangerPressed: core.RGB(0xca7e83),
			onDanger: core.RGB(0x291b14), success: core.RGB(0x79bea1), warning: core.RGB(0xe6b567),
			knob: core.RGB(0xf4f1ec), switchOff: core.RGB(0x68635b)}
	}
	return &theme{
		background: core.RGB(0xfcfbf9), surface: core.RGB(0xfefefd), sunken: core.RGB(0xf2f0ec), sidebar: core.RGB(0xf4f2ee),
		ink: core.RGB(0x33332f), muted: core.RGB(0x72726a), line: core.RGB(0xdeded5), hover: core.RGB(0xeceae4),
		borderStrong: core.RGB(0xb3b3ac), selection: core.RGBA(0x33332f, 0.08), raised: core.RGB(0xfefefd),
		quiet: core.RGB(0xecebe9), quietHover: core.RGB(0xe4e3e1), quietPressed: core.RGB(0xd8d7d5),
		accent: core.RGB(0xbd481b), accentHover: core.RGB(0xa63f18), accentPressed: core.RGB(0x8e3614),
		accentSoft: core.RGB(0xf4e6de), onAccent: core.RGB(0xffffff),
		danger: core.RGB(0xb4444c), dangerHover: core.RGB(0x9e3c43), dangerPressed: core.RGB(0x873339),
		onDanger: core.RGB(0xffffff), success: core.RGB(0x059669), warning: core.RGB(0xb45309),
		knob: core.RGB(0xffffff), switchOff: core.RGB(0xc4c4bc)}
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
