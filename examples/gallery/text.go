package main

import (
	"fmt"

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
	}}
}
