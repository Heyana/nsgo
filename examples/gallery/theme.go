package main

import (
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// theme is the gallery's palette for one appearance and the styles
// the demos build from it. base has no look, so the app chooses every
// color; these are the ui package's default theme's, written out.
type theme struct {
	dark bool

	background, surface, sunken        core.Color
	ink, muted, line, hover            core.Color
	borderStrong, selection            core.Color
	quiet, quietHover                  core.Color
	quietPressed                       core.Color
	accent, accentHover, accentPressed core.Color
	accentSoft, onAccent               core.Color
	danger, dangerHover, dangerPressed core.Color
	onDanger, success, warning         core.Color
	knob                               core.Color
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
	t := &theme{dark: dark,
		background: core.RGB(0xfcfbf9), surface: core.RGB(0xfefefd), sunken: core.RGB(0xf2f0ec),
		ink: core.RGB(0x33332f), muted: core.RGB(0x72726a), line: core.RGB(0xdeded5), hover: core.RGB(0xeceae4),
		borderStrong: core.RGB(0xc4c4bc), quiet: core.RGB(0xecebe9),
		accent: core.RGB(0xbd481b), onAccent: core.RGB(0xffffff),
		danger: core.RGB(0xb4444c), onDanger: core.RGB(0xffffff),
		success: core.RGB(0x059669), warning: core.RGB(0xb45309), knob: core.RGB(0xffffff)}
	if dark {
		t.background, t.surface, t.sunken = core.RGB(0x211f1c), core.RGB(0x2b2925), core.RGB(0x1a1916)
		t.ink, t.muted, t.line, t.hover = core.RGB(0xeeeae2), core.RGB(0xbdb6aa), core.RGB(0x504b43), core.RGB(0x36332e)
		t.borderStrong, t.quiet = core.RGB(0x68635b), core.RGB(0x312f2c)
		t.accent, t.onAccent = core.RGB(0xed986e), core.RGB(0x291b14)
		t.danger, t.onDanger = core.RGB(0xe58f95), core.RGB(0x291b14)
		t.success, t.warning = core.RGB(0x79bea1), core.RGB(0xe6b567)
	}
	t.accentSoft, t.selection = t.hover, withAlpha(t.ink, 0.1)
	t.quietHover, t.quietPressed = t.shade(t.quiet, 0.06), t.shade(t.quiet, 0.12)
	t.accentHover, t.accentPressed = t.shade(t.accent, 0.06), t.shade(t.accent, 0.12)
	t.dangerHover, t.dangerPressed = t.shade(t.danger, 0.06), t.shade(t.danger, 0.12)
	return t
}

// shade moves c an amount toward black in the light appearance and toward
// white in the dark, for hovered and pressed faces.
func (t *theme) shade(c core.Color, amount float64) core.Color {
	to := 0.0
	if t.dark {
		to = 1
	}
	r, g, b, _ := c.Components()
	mix := func(x float64) core.Color { return core.Color((x+(to-x)*amount)*255 + 0.5) }
	return mix(r)<<24 | mix(g)<<16 | mix(b)<<8 | c&0xff
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

// navItem is a row of the section list. Selection is a soft neutral face, as Onevra draws
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
	return base.Button{OnPress: func(core.PressEvent) { onPress() }, Label: label, Style: style, Children: []core.Node{core.Text{Content: label}}}
}

// toggle is base.Switch styled: a track whose knob moves to the end
// while checked, then the label.
func (t *theme) toggle(label string, checked func() bool, onChange func(bool)) core.Node {
	return base.Switch{Label: label, Checked: checked, OnChange: onChange, Style: row,
		TrackStyle: core.Style{Width: core.Pt(36), Height: core.Pt(20), CornerRadius: 10, Padding: core.All(2),
			Background: t.borderStrong, JustifyContent: core.JustifyCenter, Checked: &core.Style{Background: t.accent}},
		ThumbStyle: core.Style{Width: core.Pt(16), Height: core.Pt(16), CornerRadius: 8,
			Background: t.knob, Checked: &core.Style{AlignSelf: core.AlignEnd}},
		Children: []core.Node{core.Text{Content: label}}}
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
