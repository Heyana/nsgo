package main

import (
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// theme is the gallery's palette for one appearance and the styles built
// from it. NSGo has no look of its own, so every demo takes its colors,
// radii and spacing from here.
type theme struct {
	dark bool

	background, surface, sunken, sidebar core.Color
	ink, muted, line, hover              core.Color
	accent, accentSoft, onAccent         core.Color
	danger, success, warning             core.Color
}

func newTheme(dark bool) *theme {
	if dark {
		return &theme{dark: true,
			background: core.RGB(0x0d1117), surface: core.RGB(0x161b22), sunken: core.RGB(0x1c2128), sidebar: core.RGB(0x10151c),
			ink: core.RGB(0xe6edf3), muted: core.RGB(0x8d96a0), line: core.RGB(0x30363d), hover: core.RGB(0x21262d),
			accent: core.RGB(0x4493f8), accentSoft: core.RGBA(0x4493f8, 0.18), onAccent: core.RGB(0xffffff),
			danger: core.RGB(0xf85149), success: core.RGB(0x3fb950), warning: core.RGB(0xd29922)}
	}
	return &theme{
		background: core.RGB(0xf6f7f9), surface: core.RGB(0xffffff), sunken: core.RGB(0xeff2f5), sidebar: core.RGB(0xf0f2f5),
		ink: core.RGB(0x1f2328), muted: core.RGB(0x656d76), line: core.RGB(0xd8dee4), hover: core.RGB(0xe6eaef),
		accent: core.RGB(0x0969da), accentSoft: core.RGB(0xddf4ff), onAccent: core.RGB(0xffffff),
		danger: core.RGB(0xcf222e), success: core.RGB(0x1f883d), warning: core.RGB(0xbf8700)}
}

// Spacing and radii shared by every demo.
const (
	radius      core.Points = 8
	cardRadius  core.Points = 12
	popupRadius core.Points = 10
)

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
	return core.Style{Background: t.sunken, CornerRadius: 6, Padding: core.Symmetric(8, 12),
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}
}

func (t *theme) button() core.Style {
	return core.Style{Background: t.accent, BorderWidth: 1, BorderColor: t.accent, CornerRadius: radius,
		Padding: core.Symmetric(8, 14), AlignSelf: core.AlignStart,
		Color: t.onAccent, FontWeight: core.FontWeightSemibold, FontSize: 15,
		Hover:    &core.Style{Opacity: core.Alpha(0.92)},
		Active:   &core.Style{Opacity: core.Alpha(0.75)},
		Disabled: &core.Style{Opacity: core.Alpha(0.4)}}
}

func (t *theme) secondary() core.Style {
	return t.button().Merge(core.Style{Background: t.surface, BorderWidth: 1, BorderColor: t.line, Color: t.ink,
		FontWeight: core.FontWeightMedium,
		Hover:      &core.Style{Background: t.hover},
		Active:     &core.Style{Background: t.line}})
}

func (t *theme) dangerButton() core.Style {
	return t.button().Merge(core.Style{Background: t.danger, BorderColor: t.danger})
}

func (t *theme) popup() core.Style {
	return core.Style{Background: t.surface, CornerRadius: popupRadius, BorderWidth: 1, BorderColor: t.line, Padding: core.All(6),
		Color: t.ink, ShadowColor: core.RGBA(0x000000, 0.18), ShadowRadius: 14, ShadowOffset: core.Point{Y: 4}}
}

func (t *theme) menuItem() core.Style {
	return core.Style{Padding: core.Symmetric(7, 12), CornerRadius: 6, MinWidth: core.Pt(140),
		Hover: &core.Style{Background: t.hover}, Active: &core.Style{Background: t.accentSoft}}
}

func (t *theme) field() core.Style {
	return core.Style{Height: core.Pt(36), Padding: core.Symmetric(0, 10), FontSize: 15, Color: t.ink,
		Background: t.surface, CornerRadius: radius, BorderWidth: 1, BorderColor: t.line,
		Focus:    &core.Style{BorderColor: t.accent},
		Disabled: &core.Style{Background: t.sunken, Color: t.muted}}
}

func (t *theme) backdrop() core.Style { return core.Style{Background: core.RGBA(0x000000, 0.4)} }

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
		core.Text{Content: title, Role: core.RoleHeader, Style: core.Style{FontSize: 16, FontWeight: core.FontWeightSemibold}},
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
