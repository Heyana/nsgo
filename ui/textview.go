package ui

import (
	"time"

	"github.com/oneo-me/nsgo/base/textview"
	"github.com/oneo-me/nsgo/core"
)

// TextView is a textview.TextView with a reading typography in the
// theme's colors, NSH's: 15 pt text in 25.5 pt lines, semibold headings,
// code in the platform's fixed-pitch font, bordered code blocks and
// tables. Styles merge over it part by part, and Style over the root.
// Code blocks have a copy button unless NoCopyButton; color them with
// ui/highlight.
type TextView struct {
	Markdown     string
	MarkdownFunc func() string
	HTML         string
	HTMLFunc     func() string
	Document     textview.Document
	DocumentFunc func() textview.Document
	Extensions   []textview.Extension
	Streaming    bool
	Style        core.Style
	Styles       textview.Styles
	Selectable   bool
	Highlighter  textview.Highlighter
	NoCopyButton bool

	OnLinkPress     func(url string)
	RenderCustom    func(cx *core.Scope, node textview.Custom) core.Node
	CustomSpan      func(node textview.Custom) core.Span
	RenderCodeBlock func(cx *core.Scope, code textview.CodeBlock) core.Node
	RenderImage     func(cx *core.Scope, image textview.Image) core.Node
	RenderMath      func(cx *core.Scope, math textview.Math, display bool) core.Node
	TestID          string
}

// Render implements core.Node.
func (v TextView) Render(cx *core.Scope) core.Node {
	theme := UseTheme(cx)
	styles := v.Styles
	if v.Style != (core.Style{}) {
		styles = textview.Styles{}
		for p, s := range v.Styles {
			styles[p] = s
		}
		styles[textview.PartRoot] = v.Style.Merge(v.Styles[textview.PartRoot])
	}
	tv := textview.TextView{
		Markdown: v.Markdown, MarkdownFunc: v.MarkdownFunc, HTML: v.HTML, HTMLFunc: v.HTMLFunc,
		Document: v.Document, DocumentFunc: v.DocumentFunc,
		Extensions: v.Extensions, Streaming: v.Streaming, Styles: styles, Selectable: v.Selectable, Highlighter: v.Highlighter,
		StyleFunc:   func(p textview.Part) core.Style { return theme().textView(p) },
		OnLinkPress: v.OnLinkPress, RenderCustom: v.RenderCustom, CustomSpan: v.CustomSpan,
		RenderCodeBlock: v.RenderCodeBlock, RenderImage: v.RenderImage, RenderMath: v.RenderMath, TestID: v.TestID,
	}
	if !v.NoCopyButton {
		tv.CodeBlockActions = copyButton
	}
	return tv
}

// copyButton copies a code block's code, then says so for a moment.
func copyButton(cx *core.Scope, code textview.CodeBlock) core.Node {
	copied := core.NewSignal(cx, false)
	var stop func() bool
	return Button{Variant: Ghost, Label: "Copy",
		Style: core.Style{Padding: core.Symmetric(2, 8), FontSize: 12},
		OnPress: func() {
			cx.SetClipboard(code.Code)
			copied.Set(true)
			if stop != nil {
				stop()
			}
			stop = cx.AfterFunc(1600*time.Millisecond, func() { copied.Set(false) })
		},
		Children: []core.Node{core.Text{ContentFunc: func() string {
			if copied.Get() {
				return "Copied"
			}
			return "Copy"
		}}},
	}
}

// The reading typography, in points: text 15 in lines of 1.7, blocks
// 0.75 em apart.
const (
	bodySize core.Points = 15
	bodyLine core.Points = bodySize * 1.7
	codeSize core.Points = 13
	blockGap core.Points = bodySize * 0.75
)

// fade returns c at alpha a.
func fade(c core.Color, a float64) core.Color {
	return core.Color(uint32(c)&^0xff | uint32(a*255+0.5))
}

// heading returns a heading's style: its size and line, the room above it
// after a block, less the gap.
func heading(size, line, above core.Points, spacing float32) core.Style {
	return core.Style{FontSize: size, LineHeight: line, LetterSpacing: size * core.Points(spacing),
		Margin: core.Edges{Top: max(above-blockGap, 0)}}
}

func (t Tokens) textView(p textview.Part) core.Style {
	alert := func(c core.Color) core.Style {
		return core.Style{Background: fade(c, 0.08), Borders: core.All(1), BorderColors: core.AllColors(fade(c, 0.35)),
			Color: c, Padding: core.Symmetric(12, 16), CornerRadius: 8, Gap: 4}
	}
	mono := core.Style{FontFamily: core.FontMonospace, FontSize: codeSize}
	switch p {
	case textview.PartRoot:
		return core.Style{FontSize: bodySize, LineHeight: bodyLine, Color: t.Foreground, Gap: blockGap}
	case textview.PartHeading:
		return core.Style{FontWeight: core.FontWeightSemibold}
	case textview.PartHeading1:
		return heading(27, 27*1.25, 27*1.1, -0.02)
	case textview.PartHeading2:
		return heading(20, 27, 20*1.6, -0.012)
	case textview.PartHeading3:
		return heading(17, 23, 17*1.4, -0.012)
	case textview.PartHeading4:
		return heading(15, 20.25, 15*1.4, -0.012)
	case textview.PartHeading5, textview.PartHeading6:
		s := heading(13, 17.5, 13*1.4, 0)
		if p == textview.PartHeading6 {
			s.Color = t.MutedForeground
		}
		return s
	case textview.PartBlockquote:
		return core.Style{Borders: core.Edges{Left: 2}, BorderColors: core.EdgeColors{Left: fade(t.Foreground, 0.15)},
			Padding: core.Edges{Left: 14}, Color: t.MutedForeground, Gap: blockGap}
	case textview.PartAlertNote:
		return alert(t.Info)
	case textview.PartAlertTip:
		return alert(t.Success)
	case textview.PartAlertImportant:
		if t.Dark {
			return alert(core.RGB(0xc4a7ff))
		}
		return alert(core.RGB(0x8250df))
	case textview.PartAlertWarning:
		return alert(t.Warning)
	case textview.PartAlertCaution:
		return alert(t.Danger)
	case textview.PartAlertTitle:
		return core.Style{FontWeight: core.FontWeightSemibold}
	case textview.PartAlertContent:
		return core.Style{Color: t.Foreground, Gap: blockGap}
	case textview.PartList:
		return core.Style{Gap: bodySize * 0.25}
	case textview.PartListItem:
		return core.Style{Direction: core.Row}
	case textview.PartListMarker:
		return core.Style{MinWidth: core.Pt(bodySize * 1.4), Padding: core.Edges{Right: 6}, TextAlign: core.TextAlignEnd,
			Color: t.MutedForeground}
	case textview.PartTaskMarker:
		return core.Style{Width: core.Pt(15), Height: core.Pt(15), Margin: core.Edges{Top: (bodyLine - 15) / 2, Right: 8},
			BorderWidth: 1, BorderColor: t.Input, CornerRadius: 4, AlignItems: core.AlignCenter,
			JustifyContent: core.JustifyCenter, FontSize: 11, LineHeight: 13, Color: t.PrimaryForeground}
	case textview.PartTaskMarkerChecked:
		return core.Style{Background: t.Primary, BorderColor: t.Primary}
	case textview.PartListItemContent:
		return core.Style{FlexGrow: 1, FlexShrink: 1, Gap: bodySize * 0.25}
	case textview.PartTaskDone:
		return core.Style{Color: t.MutedForeground, TextDecoration: core.TextDecorationLineThrough}
	case textview.PartCodeBlock:
		return core.Style{Background: t.Muted, BorderWidth: 1, BorderColor: t.Border, CornerRadius: 8, Overflow: core.Hidden}
	case textview.PartCodeBlockHeader:
		return core.Style{Padding: core.All(6)}
	case textview.PartCodeBlockCode:
		return mono.Merge(core.Style{LineHeight: codeSize * 1.6, Padding: core.Symmetric(14, 16)})
	case textview.PartCode, textview.PartMath:
		return mono.Merge(core.Style{Color: t.AccentForeground, Background: t.Muted, CornerRadius: 4})
	case textview.PartKeyboard:
		return mono.Merge(core.Style{FontSize: 12, FontWeight: core.FontWeightMedium, Background: t.Muted, CornerRadius: 4})
	case textview.PartMathBlock:
		return mono.Merge(core.Style{LineHeight: codeSize * 1.6, Padding: core.Symmetric(14, 16), Background: t.Muted,
			BorderWidth: 1, BorderColor: t.Border, CornerRadius: 8})
	case textview.PartStrong:
		return core.Style{FontWeight: core.FontWeightSemibold}
	case textview.PartEmphasis:
		return core.Style{FontStyle: core.FontStyleItalic}
	case textview.PartStrikethrough:
		return core.Style{TextDecoration: core.TextDecorationLineThrough}
	case textview.PartUnderline:
		return core.Style{TextDecoration: core.TextDecorationUnderline}
	case textview.PartMark:
		return core.Style{Background: fade(t.Warning, 0.3), CornerRadius: 3}
	case textview.PartSuperscript, textview.PartSubscript:
		return core.Style{FontSize: 11}
	case textview.PartSmall:
		return core.Style{FontSize: codeSize}
	case textview.PartLink:
		return core.Style{Color: t.Link, TextDecoration: core.TextDecorationUnderline}
	case textview.PartImage:
		return core.Style{CornerRadius: 8, Overflow: core.Hidden}
	case textview.PartTable:
		return core.Style{FontSize: 14, LineHeight: 14 * 1.55, Borders: core.Edges{Top: 1, Left: 1},
			BorderColor: t.Border}
	case textview.PartTableHead:
		return core.Style{Background: t.Muted}
	case textview.PartTableCell:
		return core.Style{Padding: core.Symmetric(8, 12), Borders: core.Edges{Right: 1, Bottom: 1}, BorderColor: t.Border}
	case textview.PartTableHeaderCell:
		return core.Style{FontSize: 13, LineHeight: 20, FontWeight: core.FontWeightSemibold, Color: t.MutedForeground}
	case textview.PartRule:
		return core.Style{Height: core.Pt(1), Background: fade(t.Foreground, 0.15),
			Margin: core.Symmetric(bodySize*1.6-blockGap, 0)}
	case textview.PartDetails:
		return core.Style{Gap: blockGap}
	case textview.PartSummary:
		return core.Style{Direction: core.Row, Gap: 6}
	case textview.PartSummaryMarker:
		return core.Style{Color: t.MutedForeground}
	case textview.PartFootnotes:
		return core.Style{FontSize: codeSize, LineHeight: codeSize * 1.6, Color: t.MutedForeground, Gap: 4,
			Borders: core.Edges{Top: 1}, BorderColor: t.Border, Padding: core.Edges{Top: 12}, Margin: core.Edges{Top: blockGap}}
	case textview.PartFootnote:
		return core.Style{Direction: core.Row, Gap: 6}
	case textview.PartFootnoteLabel:
		return core.Style{MinWidth: core.Pt(18)}
	case textview.PartFootnoteReference:
		return core.Style{FontSize: 11, Color: t.Link}
	}
	return core.Style{}
}
