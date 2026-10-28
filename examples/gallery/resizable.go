package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// handleThumb is the thumb of a resize handle, as gpui-component draws
// it: none at rest, so the hairlines stay quiet; a faint pill grows in
// under the pointer, and a long solid one while pressed or dragged.
func handleThumb(cx *core.Scope) core.Style {
	return core.Style{Height: core.Pt(44), Width: core.Pt(3), CornerRadius: 1.5, Opacity: core.Alpha(0.001),
		Transform:  core.Transform{ScaleY: 0.001},
		Transition: core.Transition{Duration: 150 * time.Millisecond, Easing: core.EaseOut},
		GroupStyle: &core.GroupStyle{
			Hover:  &core.Style{Opacity: core.Alpha(0.4), Transform: core.Transform{ScaleY: 0.45}},
			Active: &core.Style{Opacity: core.Alpha(0.9), Transform: core.Transform{ScaleY: 1}},
		}}
}

// handleThumbColor is the thumb's color, for HandleThumbStyleFunc.
func handleThumbColor(cx *core.Scope) func() core.Style { return filled(colorsOf(cx).muted) }

// handleLine is a handle's line, which stays as it is: its thumb shows
// where to drag. It is a StyleFunc.
func handleLine(cx *core.Scope) func() core.Style { return filled(colorsOf(cx).border) }

func roundSizes(sizes []core.Points) string {
	parts := make([]string, len(sizes))
	for i, s := range sizes {
		parts[i] = fmt.Sprintf("%.0f", s)
	}
	return strings.Join(parts, " · ")
}

// pane is a panel's title and what it does.
func pane(cx *core.Scope, title, detail string, size func() string) core.Node {
	colors := colorsOf(cx)
	children := []core.Node{text(title, 14, semibold, colors.text), text(detail, 12, regular, colors.muted)}
	if size != nil {
		children = append(children, core.Text{ContentFunc: size, Style: core.Style{FontSize: 12, FontFamily: mono}, StyleFunc: colored(colors.text)})
	}
	return core.View{Style: core.Style{FlexGrow: 1, Gap: 4, Padding: core.All(16)}, Children: children}
}

// resizableDemo is base's ResizablePanelGroup and ResizablePanel in an
// editor's frame, a sidebar beside an editor over a console, each handle
// dragged within the panels' ranges; the console can be hidden and comes
// back at its size. The drags run on the platform; the sizes are the
// app's, reported when a drag ends.
func resizableDemo(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	// A phone leaves the editor room beside the sidebar.
	start := core.Points(220)
	if core.Untracked(cx, cx.Env().Window).Width < 600 {
		start = 130
	}
	sidebar, consoleSize := core.NewSignal(cx, start), core.NewSignal(cx, core.Points(140))
	shown := core.NewSignal(cx, true)
	last := core.NewSignal(cx, "—")
	return core.View{StyleFunc: func() core.Style {
		return fillPage.Merge(core.Style{Gap: 12, Background: colors.background(), Padding: core.All(16)})
	}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
			core.Text{Content: hint(cx, "Drag a line between two panels with your finger", "Drag a line between two panels"),
				Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 14}, StyleFunc: colored(colors.muted)},
			base.Button{OnPress: func(core.PressEvent) { toggled(shown) }, Selected: shown.Get, Label: "Console",
				StyleFunc: func() core.Style {
					return core.Style{CornerRadius: 6, BorderWidth: 1, BorderColor: colors.border(), Padding: core.Symmetric(6, 12),
						Active: &core.Style{Opacity: core.Alpha(0.7)}, Selected: &core.Style{Background: colors.raised()}}
				},
				Children: []core.Node{text("Console", 13, medium, colors.text)}},
		}},
		core.View{StyleFunc: func() core.Style {
			return core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(240), Overflow: core.Hidden, CornerRadius: 8, BorderWidth: 1, BorderColor: colors.border(),
				Background: colors.background()}
		}, Children: []core.Node{
			base.ResizablePanelGroup{TestID: "panels", HandleStyleFunc: handleLine(cx), HandleThumbStyle: handleThumb(cx), HandleThumbStyleFunc: handleThumbColor(cx), HandleLabel: "Resize the sidebar",
				Style:    core.Style{FlexGrow: 1},
				OnResize: func(s []core.Points) { last.Set("Row: " + roundSizes(s)) },
				Panels: []base.ResizablePanel{
					{Size: sidebar.Get, MinSize: 120, MaxSize: 360, OnResize: sidebar.Set, StyleFunc: filled(colors.raised),
						Children: []core.Node{pane(cx, "Files", "120 to 360 pt", func() string { return fmt.Sprintf("Sidebar %.0f pt", sidebar.Get()) })}},
					{MinSize: 160, Children: []core.Node{base.ResizablePanelGroup{Axis: base.Vertical, HandleStyleFunc: handleLine(cx), HandleThumbStyle: handleThumb(cx), HandleThumbStyleFunc: handleThumbColor(cx),
						HandleLabel: "Resize the console", Style: core.Style{FlexGrow: 1},
						OnResize: func(s []core.Points) { last.Set("Column: " + roundSizes(s)) },
						Panels: []base.ResizablePanel{
							{MinSize: 80, Children: []core.Node{pane(cx, "Editor", "Takes the room the others leave", nil)}},
							{Size: consoleSize.Get, MinSize: 60, Visible: shown.Get, OnResize: consoleSize.Set, StyleFunc: filled(colors.raised),
								Children: []core.Node{pane(cx, "Console", "Hidden, it keeps its size", func() string { return fmt.Sprintf("Console %.0f pt", consoleSize.Get()) })}},
						}}}},
				}},
		}},
		core.Text{ContentFunc: func() string { return "Last resize: " + last.Get() }, Style: core.Style{FontSize: 12, FontFamily: mono}, StyleFunc: colored(colors.muted)},
	}}
}
