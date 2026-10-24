package ui

import (
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
)

// WindowControls is a hidden title bar's window buttons: minimize,
// maximize or restore, and close, after Onevra's. The app places them in
// the title bar it draws, where the platform draws none of its own, as
// Linux does; macOS keeps its traffic lights and Windows its caption
// buttons.
type WindowControls struct {
	Style  core.Style
	TestID string
}

// Render implements core.Node.
func (w WindowControls) Render(cx *core.Scope) core.Node {
	env := cx.Env()
	id := func(part string) string {
		if w.TestID == "" {
			return ""
		}
		return w.TestID + "-" + part
	}
	button := func(icon *core.SVG, label, testID string, press func(), style core.Style) core.Node {
		return IconButton{Icon: icon, OnPress: func(core.PressEvent) { press() }, Label: label, TestID: testID, Size: 14, Style: style}
	}
	// Close is red under the pointer, as Onevra draws it, and white
	// there.
	closeStyle := core.Style{
		Hover:  &core.Style{Background: core.RGB(0xe81123), Color: core.RGB(0xffffff)},
		Active: &core.Style{Background: core.RGB(0xc42b1c), Color: core.RGB(0xffffff)},
	}
	return core.View{
		Style:  core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4, FlexShrink: 0}.Merge(w.Style),
		TestID: w.TestID,
		Children: []core.Node{
			// Onevra sets the minus a little low, as its glyph reads.
			button(lucide.Minus(), "Minimize window", id("minimize"), cx.MinimizeWindow,
				core.Style{Padding: core.Edges{Top: 4}}),
			core.Show{When: func() bool { return env.Maximized() },
				Then: func(*core.Scope) core.Node {
					return button(lucide.Copy(), "Restore window", id("restore"), cx.ToggleMaximize, core.Style{})
				},
				Else: func(*core.Scope) core.Node {
					return button(lucide.Square(), "Maximize window", id("maximize"), cx.ToggleMaximize, core.Style{})
				}},
			button(lucide.X(), "Close window", id("close"), cx.CloseWindow, closeStyle),
		},
	}
}
