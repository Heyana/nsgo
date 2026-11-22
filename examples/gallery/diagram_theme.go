package main

import (
	"github.com/nsgo-dev/nsgo/base/diagram"
	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// renderThemedDiagram gives document diagrams the gallery's scoped palette. The mapping belongs to the app; base/diagram reads no ui colors itself.
func renderThemedDiagram(cx *core.Scope, code textview.CodeBlock) core.Node {
	colors := ui.ColorsOf(cx)
	return diagram.View{Source: code.Code, ErrorStyle: core.Style{FontFamily: core.FontMonospace}, ThemeFunc: func() *diagram.Theme {
		c := colors.Now()
		return &diagram.Theme{
			Background: c.Inset, Text: c.Text, Muted: c.Muted, Line: c.Border,
			Surface: c.Surface, SurfaceAlt: c.Raised, Border: c.Border,
			Accent: c.Accent, Critical: c.Danger,
			Series: [8]core.Color{c.Accent, c.Up, c.Warn, c.Danger, c.Info, c.Muted, c.AccentFill, c.Down},
			Radius: float32(ui.RadiusControl), StrokeWidth: 1,
		}
	}}
}
