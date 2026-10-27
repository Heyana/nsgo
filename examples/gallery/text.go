package main

import (
	"fmt"

	"github.com/oneo-me/nsgo/core"
)

const lorem = "Native text, typeset once on the engine with the platform's text engine and drawn on the main thread without any layout work."

func bodyStyle(cx *core.Scope) core.Style {
	colors := colorsOf(cx)
	return core.Style{FontSize: 15, LineHeight: 22, Color: colors.text}
}

func linkStyle(cx *core.Scope) core.Style {
	colors := colorsOf(cx)
	return core.Style{Color: colors.accent, TextDecoration: core.TextDecorationUnderline, TextDecorationColor: alpha(colors.accent, 0.5)}
}

// specimenRow is a labeled specimen: a quiet label (and the property it
// shows, in code) above the text.
func specimenRow(cx *core.Scope, lbl, codeText string, first bool, children ...core.Node) core.Node {
	colors := colorsOf(cx)
	s := core.Style{Gap: 8}
	if !first {
		s.Padding, s.Borders, s.BorderColors = core.Edges{Top: 12}, core.Edges{Top: 1}, core.EdgeColors{Top: colors.line}
	}
	head := []core.Node{core.Text{Content: lbl, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 13, Color: colors.muted}}}
	if codeText != "" {
		head = append(head, core.Text{Content: codeText, MaxLines: 1, Style: core.Style{FontSize: 12, Color: colors.faint, FontFamily: mono}})
	}
	return core.View{Style: s, Children: append([]core.Node{core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignBaseline, Gap: 8}, Children: head}}, children...)}
}

func linksCard(cx *core.Scope) core.Node {
	tapped := core.NewSignal(cx, "nothing yet")
	set := func(s string) func(core.PressEvent) { return func(core.PressEvent) { tapped.Set(s) } }
	b := bodyStyle(cx)
	b.FontSize, b.LineHeight = 16, 25
	return card(cx, cardProps{icon: iconText, title: "Spans and links", subtitle: "Spans with presses of their own"},
		core.Text{OnPress: set("the sentence (not a link)"), Style: b, TestID: "links", Spans: []core.Span{
			{Content: "By continuing you agree to the "},
			{Content: "Terms of service", Style: linkStyle(cx), OnPress: set("Terms of service")},
			{Content: " and the "},
			{Content: "Privacy policy", Style: linkStyle(cx), OnPress: set("Privacy policy")},
			{Content: ", which mention "},
			{Content: "bold ", Style: core.Style{FontWeight: bold}},
			{Content: "links", Style: linkStyle(cx).Merge(core.Style{FontWeight: bold}), OnPress: set("a link inside bold text")},
			{Content: " too", Style: core.Style{FontWeight: bold}},
			{Content: "."},
		}},
		caption(cx, "Pressed", tapped.Get),
	)
}

func inlineCard(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	tints := tintsOf(cx)
	fin := finOf(cx)
	likes := core.NewSignal(cx, 3)
	b := bodyStyle(cx)
	b.FontSize, b.LineHeight = 16, 28
	return card(cx, cardProps{icon: iconImage, title: "Inline views and images", subtitle: "They wrap with the line"},
		core.Text{Style: b, Spans: []core.Span{
			{Content: "Made with "},
			{Attachment: core.Image{Source: core.ImageSource{Data: logoPNG}, Style: core.Style{Width: core.Pt(18), Height: core.Pt(18), CornerRadius: 5}}},
			{Content: " NSGo. Status "},
			{Attachment: core.View{Style: core.Style{Background: tints.up, CornerRadius: 9, Padding: core.Symmetric(0, 8), Height: core.Pt(18), Direction: core.Row,
				AlignItems: core.AlignCenter, Gap: 4}, Children: []core.Node{
				core.View{Style: core.Style{Width: core.Pt(6), Height: core.Pt(6), CornerRadius: 3, Background: colors.up}},
				core.Text{Content: "LIVE", Style: core.Style{Color: colors.up, FontSize: 11, FontWeight: bold, LetterSpacing: 0.5}},
			}}},
			{Content: " and a button inside the sentence "},
			{Attachment: press(pressProps{onPress: func() { likes.Update(func(n int) int { return n + 1 }) }, scale: 0.9, label: "Like", hitSlop: 10},
				core.View{Style: fin.raised.over(core.Style{Background: colors.raised, CornerRadius: 11, Padding: core.Symmetric(0, 9), Height: core.Pt(22),
					JustifyContent: core.JustifyCenter, BorderWidth: 1, BorderColor: colors.border}),
					Children: []core.Node{textf(func() string { return fmt.Sprintf("♥ %d", likes.Get()) }, 13, regular, colors.accent)}})},
			{Content: " that takes presses of its own."},
		}},
	)
}

func fontsCard(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	fin := finOf(cx)
	isDark := cx.Dark()
	glow := alpha(colors.accent, 0.3)
	drop := paper(0.22)
	if isDark {
		glow, drop = alpha(colors.accent, 0.6), black(0.9)
	}
	return card(cx, cardProps{icon: iconStyles, title: "Type", subtitle: "Custom fonts, weights, styles, spacing"},
		specimenRow(cx, "Custom fonts", "nsgo.Fonts", true,
			core.Text{Content: "Custom fonts", Style: core.Style{FontFamily: "Pacifico", FontSize: 34, Color: colors.accent, LineHeight: 52}},
			core.Text{Spans: []core.Span{{Content: "Go Smallcaps "}, {Content: "and its italic", Style: core.Style{FontStyle: core.FontStyleItalic}}},
				TestID: "app-font", Style: core.Style{FontFamily: "Go Smallcaps", FontSize: 20, Color: colors.text}},
		),
		specimenRow(cx, "Weight, style, spacing", "FontWeight, FontStyle", false,
			core.Text{Style: core.Style{FontSize: 17, Color: colors.text}, Spans: []core.Span{
				{Content: "Light ", Style: core.Style{FontWeight: core.FontWeightLight}},
				{Content: "Bold ", Style: core.Style{FontWeight: bold}},
				{Content: "Italic ", Style: core.Style{FontStyle: core.FontStyleItalic}},
				{Content: "Both ", Style: core.Style{FontWeight: bold, FontStyle: core.FontStyleItalic}},
				{Content: "Spaced", Style: core.Style{LetterSpacing: 4, Color: colors.muted}},
			}}),
		specimenRow(cx, "Decorations", "TextDecoration", false,
			core.Text{Style: core.Style{FontSize: 17, Color: colors.text}, Spans: []core.Span{
				{Content: "Underline", Style: core.Style{TextDecoration: core.TextDecorationUnderline}},
				{Content: " "},
				{Content: "Strikethrough", Style: core.Style{TextDecoration: core.TextDecorationLineThrough, TextDecorationColor: colors.danger, Color: colors.muted}},
				{Content: " "},
				{Content: "Wavy", Style: core.Style{TextDecoration: core.TextDecorationUnderline, TextDecorationStyle: core.DecorationWavy, TextDecorationColor: colors.info, Color: colors.info}},
			}}),
		specimenRow(cx, "Transforms", "TextTransform", false,
			core.Text{Style: core.Style{FontSize: 17, Color: colors.text}, Spans: []core.Span{
				{Content: "uppercase ", Style: core.Style{TextTransform: core.TextTransformUppercase}},
				{Content: "capitalized words here", Style: core.Style{TextTransform: core.TextTransformCapitalize, Color: colors.muted}},
			}}),
		specimenRow(cx, "Shadows", "TextShadow…", false,
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
				core.Text{Content: "Glow", Style: core.Style{FontSize: 26, FontWeight: core.FontWeight(800), Color: colors.accent, TextShadowColor: glow, TextShadowRadius: 5}},
				core.View{Style: fin.raised.over(core.Style{Height: core.Pt(48), Padding: core.Symmetric(0, 14), JustifyContent: core.JustifyCenter, CornerRadius: radiusControl,
					BorderCurve: core.BorderCurveContinuous, Background: colors.raised, BorderWidth: 1, BorderColor: colors.border}), Children: []core.Node{
					core.Text{Content: "Drop shadow", Style: core.Style{FontSize: 26, FontWeight: core.FontWeight(800), Color: colors.info, TextShadowColor: drop,
						TextShadowOffset: core.Point{X: 2, Y: 3}, TextShadowRadius: 1.5}},
				}},
			}}),
	)
}

func scalingCard(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	env := cx.Env()
	b := bodyStyle(cx)
	b.FontSize = 17
	return card(cx, cardProps{icon: iconSearch, title: "System text size", subtitle: "Env.FontScale and its limits",
		right: textf(func() string {
			s := env.FontScale()
			if s == 0 {
				s = 1
			}
			return fmt.Sprintf("%.2f×", s)
		}, 15, medium, colors.accent)},
		specimenRow(cx, "Default", "", true, core.Text{Content: "Scales with the system setting", Style: b}),
		specimenRow(cx, "Fixed", "NoFontScaling", false, core.Text{Content: "Fixed size", NoFontScaling: true, Style: b}),
		specimenRow(cx, "Capped", "MaxFontSizeMultiplier: 1.2", false, core.Text{Content: "At most 1.2×", MaxFontSizeMultiplier: 1.2, Style: b}),
	)
}

func truncationCard(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	b := bodyStyle(cx)
	centered := b
	centered.TextAlign, centered.Color = core.TextAlignCenter, colors.muted
	return card(cx, cardProps{icon: iconLists, title: "Truncation", subtitle: "MaxLines and EllipsizeMode"},
		specimenRow(cx, "Two lines, tail", "MaxLines: 2", true, core.Text{Content: lorem, MaxLines: 2, Style: b}),
		specimenRow(cx, "One line, head", "EllipsizeHead", false, core.Text{Content: lorem, MaxLines: 1, EllipsizeMode: core.EllipsizeHead, Style: b}),
		specimenRow(cx, "One line, middle", "EllipsizeMiddle", false, core.Text{Content: lorem, MaxLines: 1, EllipsizeMode: core.EllipsizeMiddle, Style: b}),
		specimenRow(cx, "One line, clip", "EllipsizeClip", false, core.Text{Content: lorem, MaxLines: 1, EllipsizeMode: core.EllipsizeClip, Style: b}),
		specimenRow(cx, "Centered, three lines", "TextAlignCenter", false, core.Text{Content: lorem + " " + lorem, MaxLines: 3, Style: centered}),
	)
}

const (
	chineseText  = "原生文本在引擎线程上一次排版，在主线程上直接绘制。中文在任意两个汉字之间都可以换行，但句号、逗号和右括号不会出现在行首，左括号也不会留在行尾（就像这样）。"
	japaneseText = "日本語の文章は、ひらがな・カタカナ・漢字に Latin の単語や数字（2024年）を混ぜて組みます。句読点は行頭に来ません。"
	arabicText   = "النص الأصلي يُنضَّد مرة واحدة على خيط المحرك ثم يُرسم على الخيط الرئيسي دون أي عمل إضافي في التخطيط."
)

func cjkCard(cx *core.Scope) core.Node {
	b := bodyStyle(cx)
	return card(cx, cardProps{icon: iconText, title: "Chinese and Japanese", subtitle: "Lines break between characters, never before closing punctuation"},
		specimenRow(cx, "Chinese paragraph", "", true, core.Text{Content: chineseText, Style: b}),
		specimenRow(cx, "Japanese: kana, kanji and Latin", "", false, core.Text{Content: japaneseText, Style: b}),
		specimenRow(cx, "Two lines, tail", "MaxLines: 2", false, core.Text{Content: chineseText, MaxLines: 2, Style: b}),
		specimenRow(cx, "One line, middle", "EllipsizeMiddle", false, core.Text{Content: japaneseText, MaxLines: 1, EllipsizeMode: core.EllipsizeMiddle, Style: b}),
	)
}

func rtlTextCard(cx *core.Scope) core.Node {
	b := bodyStyle(cx)
	rtl := b
	rtl.WritingDirection = core.WritingRTL
	return card(cx, cardProps{icon: iconText, title: "Right to left", subtitle: "Arabic and Hebrew, mixed with Latin text and numbers"},
		specimenRow(cx, "Arabic paragraph", "WritingRTL", true, core.Text{Content: arabicText, Style: rtl}),
		specimenRow(cx, "Arabic, English and numbers in one line", "", false, core.Text{Content: "تم شحن الطلب رقم 4521 عبر DHL في 12 مارس، والتسليم خلال 3 أيام.", Style: b}),
		specimenRow(cx, "Hebrew", "", false, core.Text{Content: "שלום! הגרסה 2.0 של NSGo זמינה עכשיו (גם ב-Linux).", Style: b}),
		specimenRow(cx, "One line, tail", "MaxLines: 1", false, core.Text{Content: arabicText, MaxLines: 1, Style: b}),
	)
}

func alignmentCard(cx *core.Scope) core.Node {
	const sentence = "Short, aligned in its box"
	const paragraph = "Justified text spreads the spaces of every line across the box, except on the last line of a paragraph, which stays at its start."
	aligned := func(a core.TextAlign) core.Style { s := bodyStyle(cx); s.TextAlign = a; return s }
	return card(cx, cardProps{icon: iconLists, title: "Alignment", subtitle: "TextAlign in left-to-right and right-to-left text"},
		specimenRow(cx, "Start", "TextAlignStart", true, core.Text{Content: sentence, Style: aligned(core.TextAlignStart)}),
		specimenRow(cx, "Center", "TextAlignCenter", false, core.Text{Content: sentence, Style: aligned(core.TextAlignCenter)}),
		specimenRow(cx, "End", "TextAlignEnd", false, core.Text{Content: sentence, Style: aligned(core.TextAlignEnd)}),
		specimenRow(cx, "Justify", "TextAlignJustify", false, core.Text{Content: paragraph, Style: aligned(core.TextAlignJustify)}),
		specimenRow(cx, "Justify, Chinese", "TextAlignJustify", false, core.Text{Content: chineseText, Style: aligned(core.TextAlignJustify)}),
		specimenRow(cx, "Start, in Arabic", "TextAlignStart", false, core.Text{Content: arabicText, Style: aligned(core.TextAlignStart)}),
	)
}

func selectionCard(cx *core.Scope) core.Node {
	colors := colorsOf(cx)
	b := bodyStyle(cx)
	b.SelectionColor = alpha(colors.accent, 0.35)
	return card(cx, cardProps{icon: iconText, title: "Selection", subtitle: hint(cx, "Long press to select", "Drag, or double or triple click"), right: badge(cx, "NSGo", toneNeutral)},
		specimenRow(cx, "On its own", "SelectableOn", true, core.Text{Content: "This text selects on its own.", Selectable: core.SelectableOn, Style: b}),
		specimenRow(cx, "Across texts", "View.Selectable", false,
			core.View{Selectable: true, Style: core.Style{Gap: 8}, Children: []core.Node{
				core.Text{Content: "A selectable view selects across its texts, as a web page does.", Style: b},
				core.Text{Content: "Start in one, end in another; copy them as one.", Style: b},
				core.Text{Content: "This one stays out of the selection.", Selectable: core.SelectableOff, Style: b.Merge(core.Style{Color: colors.faint})},
			}}),
	)
}

// textDemo is core.Text: spans and links, inline views, fonts, the
// system's text size, truncation, Chinese, Japanese and right-to-left
// text, alignment and selection.
func textDemo(cx *core.Scope) core.Node {
	return demoPage(cx, pageProps{}, linksCard, inlineCard, fontsCard, scalingCard, truncationCard, cjkCard, rtlTextCard, alignmentCard, selectionCard)
}
