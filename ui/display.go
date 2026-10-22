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
			size := core.Pt(theme().small())
			return core.Style{Width: size, Height: size, FlexShrink: 0}
		}})
	}
	return core.View{StyleFunc: look(cx, b.Style, func(t Tokens) core.Style { return t.badge(b.Tone) }), TestID: b.TestID,
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
	return core.View{StyleFunc: look(cx, p.Style, Tokens.progress), Label: p.Label, TestID: p.TestID,
		Children: []core.Node{core.View{StyleFunc: func() core.Style {
			var v float64
			if p.Value != nil {
				v = min(max(p.Value(), 0), 1)
			}
			return core.Style{Height: core.Pct(100), Width: core.Pct(float32(v * 100)), CornerRadius: 3,
				Background: theme().Primary}
		}}}}
}
