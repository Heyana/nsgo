package ui

import "github.com/oneo-me/nsgo/core"

// Tone is the color a badge or a toast speaks in.
type Tone uint8

const (
	// ToneNeutral is the surface's own color, the default.
	ToneNeutral Tone = iota
	// ToneAccent is the primary color.
	ToneAccent
	ToneSuccess
	ToneWarning
	ToneDanger
	ToneInfo
)

// Badge is a small pill of text, such as a status, a count or a version:
// neutral, or washed with its Tone's color. Icon, when set, leads it.
type Badge struct {
	Text string
	// TextFunc replaces Text, again when what it reads changes.
	TextFunc func() string
	Icon     *core.SVG
	Tone     Tone
	Style    core.Style
	TestID   string
}

// Render implements core.Node.
func (b Badge) Render(cx *core.Scope) core.Node {
	theme := UseTheme(cx)
	var children []core.Node
	if b.Icon != nil {
		children = append(children, core.Icon{SVG: b.Icon, StyleFunc: func() core.Style {
			size := core.Pt(core.Points(float32(theme().FontSize)*0.85 + 0.5))
			return core.Style{Width: size, Height: size, FlexShrink: 0}
		}})
	}
	return core.View{StyleFunc: look(cx, b.Style, func(t Tokens) core.Style { return badgeStyle(t, b.Tone) }), TestID: b.TestID,
		Children: append(children, core.Text{Content: b.Text, ContentFunc: b.TextFunc, MaxLines: 1})}
}

// Progress is a bar filled with the primary color as far as Value, from 0
// to 1, says.
type Progress struct {
	Value  func() float64
	Style  core.Style
	Label  string
	TestID string
}

// Render implements core.Node.
func (p Progress) Render(cx *core.Scope) core.Node {
	theme := UseTheme(cx)
	return core.View{StyleFunc: look(cx, p.Style, progressStyle), Label: p.Label, TestID: p.TestID,
		Children: []core.Node{core.View{StyleFunc: func() core.Style {
			var v float64
			if p.Value != nil {
				v = min(max(p.Value(), 0), 1)
			}
			return core.Style{Height: core.Pct(100), Width: core.Pct(float32(v * 100)), CornerRadius: 3,
				Background: theme().Primary}
		}}}}
}

// badgeStyle is a pill: neutral on the surface, or washed with its tone's
// color.
func badgeStyle(t Tokens, tone Tone) core.Style {
	bg, fg, border := t.Surface, t.Foreground, t.Border
	if tint, ok := toneColor(t, tone); ok {
		bg, fg, border = Mix(t.Background, tint, 0.12), Mix(tint, t.Foreground, 0.3), Mix(t.Background, tint, 0.3)
	}
	return core.Style{Direction: core.Row, AlignItems: core.AlignCenter, AlignSelf: core.AlignStart, FlexShrink: 0, Gap: 4,
		Padding: core.Symmetric(2, 8), CornerRadius: 12, BorderWidth: 1, BorderColor: border, Background: bg,
		Color: fg, FontSize: core.Points(float32(t.FontSize)*0.85 + 0.5), FontWeight: core.FontWeightMedium}
}

func progressStyle(t Tokens) core.Style {
	return core.Style{Height: core.Pt(6), CornerRadius: 3, Background: t.Tint(0.1), Overflow: core.Hidden}
}

// toneColor returns a tone's color, or false for the neutral tone.
func toneColor(t Tokens, tone Tone) (core.Color, bool) {
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
