package main

import (
	"fmt"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

const lorem = "NSGo lays text out on the engine through the platform's text engine, " +
	"so layout knows each line before the main thread draws it. Long text wraps at the width it is given."

func textPage(cx *core.Scope, t *theme) core.Node {
	link := core.NewSignal(cx, "no link pressed yet")

	var sizes, weights []core.Node
	for _, size := range []core.Points{12, 15, 17, 22, 28} {
		sizes = append(sizes, core.Text{Content: fmt.Sprintf("%gpt", size), Style: core.Style{FontSize: size}})
	}
	for _, w := range []struct {
		name   string
		weight core.FontWeight
	}{{"Light", core.FontWeightLight}, {"Normal", core.FontWeightNormal}, {"Medium", core.FontWeightMedium},
		{"Semibold", core.FontWeightSemibold}, {"Bold", core.FontWeightBold}} {
		weights = append(weights, core.Text{Content: w.name, Style: core.Style{FontWeight: w.weight}})
	}
	aligned := func(align core.TextAlign, name string) core.Node {
		return core.Text{Content: name, Style: core.Style{TextAlign: align, Background: t.sunken, Padding: core.Symmetric(4, 8),
			CornerRadius: 4}}
	}
	linkStyle := core.Style{Color: t.accent, TextDecoration: core.TextDecorationUnderline}

	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Sizes and weights", "FontSize in points and FontWeight from light to bold.",
			core.View{Style: wrap.Merge(core.Style{AlignItems: core.AlignBaseline, Gap: 16}), Children: sizes},
			flow(weights...),
		),
		t.demo("Families and styles", "FontFamily picks a font; FontStyle italics it. An empty family is the system font.",
			flow(
				core.Text{Content: "System"},
				core.Text{Content: "Georgia", Style: core.Style{FontFamily: "Georgia", FontSize: 18}},
				core.Text{Content: "Menlo", Style: core.Style{FontFamily: "Menlo", FontSize: 15}},
				core.Text{Content: "Italic", Style: core.Style{FontStyle: core.FontStyleItalic}},
			),
		),
		t.demo("Decoration and spacing", "TextDecoration, LetterSpacing and LineHeight.",
			flow(
				core.Text{Content: "Underline", Style: core.Style{TextDecoration: core.TextDecorationUnderline}},
				core.Text{Content: "Line through", Style: core.Style{TextDecoration: core.TextDecorationLineThrough, Color: t.muted}},
				core.Text{Content: "SPACED", Style: core.Style{LetterSpacing: 4, FontWeight: core.FontWeightSemibold}},
			),
			core.Text{Content: lorem, Style: core.Style{LineHeight: 28}},
		),
		t.demo("Alignment", "TextAlign start, center and end; start and end follow the writing direction.",
			aligned(core.TextAlignStart, "Start"), aligned(core.TextAlignCenter, "Center"), aligned(core.TextAlignEnd, "End"),
		),
		t.demo("Line limits", "MaxLines ends the last line it shows with an ellipsis.",
			core.Text{Content: lorem + " " + lorem, MaxLines: 2},
		),
		t.demo("Spans and links", "Spans style runs of one text; a span with OnPress is a link.",
			core.Text{Spans: []core.Span{
				{Content: "Read "},
				{Content: "the architecture", Style: linkStyle, OnPress: func() { link.Set("pressed the architecture link") }},
				{Content: ", or see how "},
				{Content: "bold", Style: core.Style{FontWeight: core.FontWeightBold}},
				{Content: ", "},
				{Content: "colored", Style: core.Style{Color: t.danger}},
				{Content: " and "},
				{Content: "struck", Style: core.Style{TextDecoration: core.TextDecorationLineThrough}},
				{Content: " runs share a line, then "},
				{Content: "the core docs", Style: linkStyle, OnPress: func() { link.Set("pressed the core docs link") }},
				{Content: "."},
			}},
			t.live(link.Get),
		),
		t.demo("Backgrounds and attachments", "A span's Background and CornerRadius draw behind its characters, a box "+
			"per line. A span's Attachment flows a view in the lines; VerticalAlign places it.",
			core.Text{Style: core.Style{LineHeight: 26}, Spans: []core.Span{
				{Content: "Press "},
				{Content: " ⌘ K ", Style: core.Style{Background: t.sunken, CornerRadius: 4, FontFamily: core.FontMonospace, FontSize: 13}},
				{Content: " to search, call "},
				{Content: "Scope.Post", Style: core.Style{Background: t.sunken, CornerRadius: 4, FontFamily: core.FontMonospace, FontSize: 13}},
				{Content: " from other goroutines, and "},
				{Content: "mark what matters across the end of a line, as a highlighter would", Style: core.Style{Background: core.RGBA(0xf2c94c, 0.45), CornerRadius: 3}},
				{Content: ". A dot "},
				{Style: core.Style{VerticalAlign: core.VerticalAlignMiddle}, Attachment: core.View{TestID: "dot",
					Style: core.Style{Width: core.Pt(12), Height: core.Pt(12), CornerRadius: 6, Background: t.accent}}},
				{Content: " sits in the middle of its line, a badge "},
				{Style: core.Style{VerticalAlign: core.VerticalAlignTextBottom}, Attachment: core.View{
					Style:    core.Style{Background: t.accent, CornerRadius: 9, Padding: core.Symmetric(1, 8)},
					Children: []core.Node{core.Text{Content: "new", Style: core.Style{Color: core.RGB(0xffffff), FontSize: 12}}}}},
				{Content: " on the bottom of the text, and a button "},
				{Style: core.Style{VerticalAlign: core.VerticalAlignMiddle}, Attachment: t.plain("Press", func() { link.Set("pressed the button in the text") })},
				{Content: " takes its own presses. A tall box "},
				{Attachment: core.View{Style: core.Style{Width: core.Pt(24), Height: core.Pt(40), CornerRadius: 4, Background: t.sunken,
					BorderWidth: 1, BorderColor: t.accent}}},
				{Content: " grows its line."},
			}},
			t.live(link.Get),
		),
		t.demo("Selection", "Selectable text selects alone; a SelectionArea selects across its texts. Drag, double or "+
			"triple click, or long press on touch screens, then copy.",
			core.Text{Content: "This text selects on its own, in the accent.", Selectable: core.SelectableOn,
				Style: core.Style{SelectionColor: core.RGBA(0xbd481b, 0.3)}},
			base.SelectionArea{Style: core.Style{Gap: 8, Padding: core.All(12), Background: t.sunken, CornerRadius: radius},
				Children: []core.Node{
					core.Text{Content: "A selection area", Style: core.Style{FontWeight: core.FontWeightSemibold}},
					core.Text{Content: lorem},
					core.View{Style: core.Style{Direction: core.Row, Gap: 24}, Children: []core.Node{
						core.Text{Content: "Cells on one row"}, core.Text{Content: "copy with tabs"},
						core.Text{Content: "中文按字选择"},
					}},
					core.Text{Content: "This text stays out of the selection.", Selectable: core.SelectableOff,
						Style: t.caption()},
					t.plain("Buttons keep their presses", func() { link.Set("pressed the button in the area") }),
				}},
		),
	}}
}
