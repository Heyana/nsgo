package main

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/base/textview"
	"github.com/oneo-me/nsgo/core"
)

const sampleDoc = `## A document

TextView shows **Markdown**, HTML or blocks with *texts*, views and scroll views. Links report presses: [NSGo](https://github.com/oneo-me/nsgo).

> [!TIP]
> Alerts, task lists and @mentions, an extension of this page's.

- [x] Code blocks with actions
- [ ] Tables with their own

` + "```go\nfunc main() {\n\tnsgo.Run(app)\n}\n```" + `

| Part | Styled by |
|------|-----------|
| Heading | PartHeading |
| Table cell | PartTableCell |
`

// reply is what the streaming demo writes, a few characters at a time.
const reply = "Sure. A **TextView** follows a reply as it streams: the syntax its end leaves open shows as if closed, " +
	"and the caret stays at its end.\n\n1. Parse the text again\n2. Rebuild only the last block\n\n" +
	"```sh\ngo run ./examples/gallery\n```"

// textStyles is the gallery's reading typography: base has none.
func (t *theme) textStyles() textview.Styles {
	code := core.Style{FontFamily: core.FontMonospace, FontSize: 13}
	return textview.Styles{
		textview.PartRoot:              {Gap: 10, Color: t.ink, FontSize: 15, LineHeight: 22},
		textview.PartHeading:           {FontWeight: core.FontWeightSemibold, Margin: core.Edges{Top: 6}},
		textview.PartHeading2:          {FontSize: 20, LineHeight: 26},
		textview.PartLink:              {Color: t.accent, TextDecoration: core.TextDecorationUnderline},
		textview.PartStrong:            {FontWeight: core.FontWeightSemibold},
		textview.PartEmphasis:          {FontStyle: core.FontStyleItalic},
		textview.PartCode:              code.Merge(core.Style{Background: t.sunken}),
		textview.PartBlockquote:        {Gap: 4, Padding: core.Edges{Left: 12}, Borders: core.Edges{Left: 3}, BorderColors: core.EdgeColors{Left: t.line}},
		textview.PartAlertTitle:        {FontWeight: core.FontWeightSemibold, Color: t.success},
		textview.PartList:              {Gap: 4},
		textview.PartListItem:          {Direction: core.Row, Gap: 8},
		textview.PartListMarker:        {Color: t.muted},
		textview.PartTaskMarker:        {Width: core.Pt(16), Height: core.Pt(16), CornerRadius: 4, BorderWidth: 1, BorderColor: t.borderStrong, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Margin: core.Edges{Top: 3}},
		textview.PartTaskMarkerChecked: {Background: t.accent, BorderColor: t.accent, Color: t.onAccent, FontSize: 11},
		textview.PartListItemContent:   {FlexGrow: 1, FlexShrink: 1, Gap: 4},
		textview.PartCodeBlock:         {Background: t.sunken, CornerRadius: radius, BorderWidth: 1, BorderColor: t.line},
		textview.PartCodeBlockScroll:   {Padding: core.Symmetric(10, 12)},
		textview.PartCodeBlockCode:     code.Merge(core.Style{LineHeight: 19}),
		textview.PartCodeBlockHeader:   {Padding: core.All(4)},
		textview.PartCodeBlockAction:   {Padding: core.Symmetric(2, 8), CornerRadius: 6, Background: t.surface, BorderWidth: 1, BorderColor: t.line, Hover: &core.Style{Background: t.hover}},
		textview.PartTable:             {Gap: 6},
		textview.PartTableActions:      {Direction: core.Row, JustifyContent: core.JustifyEnd},
		textview.PartTableRow:          {Borders: core.Edges{Bottom: 1}, BorderColors: core.EdgeColors{Bottom: t.line}},
		textview.PartTableCell:         {Padding: core.Symmetric(6, 10)},
		textview.PartTableHeaderCell:   {FontWeight: core.FontWeightSemibold},
		textview.PartCustom:            {Color: t.accent, FontWeight: core.FontWeightMedium},
	}
}

func textViewPage(cx *core.Scope, t *theme) core.Node {
	pressed := core.NewSignal(cx, "nothing yet")
	streamed := core.NewSignal(cx, "")
	var stop func() bool
	stream := func() {
		if stop != nil {
			stop()
		}
		streamed.Set("")
		var step func()
		step = func() {
			n := min(len(streamed.GetUntracked())+5, len(reply))
			streamed.Set(reply[:n])
			if n < len(reply) {
				stop = cx.AfterFunc(40*time.Millisecond, step)
			}
		}
		step()
	}
	// The page's extension, given to every text view on it.
	textview.Provide(cx, textview.Plugin{
		Extension: textview.Extension{Name: "mention", Pattern: regexp.MustCompile(`@\w+`)},
	})
	action := func(cx *core.Scope, a textview.CodeBlockAction) core.Node {
		return core.Text{Style: core.Style{FontSize: 12}, ContentFunc: func() string {
			if a.Done() {
				return "Copied"
			}
			return "Copy"
		}}
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		t.demo("Markdown", "base/textview's TextView: blocks the app styles part by part, a copy action on code and actions above tables.",
			textview.TextView{Markdown: sampleDoc, Styles: t.textStyles(), Selectable: true, TestID: "document",
				CodeBlockActions: []string{textview.CopyAction}, RenderCodeBlockAction: action,
				OnLinkPress: pressed.Set,
				RenderTableActions: func(cx *core.Scope, table textview.Table) core.Node {
					return core.Text{Content: strings.Repeat("·", 3) + " " + strconv.Itoa(len(table.Rows)) + " rows", Style: t.caption()}
				}},
			t.live(func() string { return "Pressed " + pressed.Get() }),
		),
		t.demo("Streaming", "A reply arriving chunk by chunk, repaired as it streams, with a caret at its end.",
			flow(t.primary("Stream a reply", stream)),
			textview.TextView{MarkdownFunc: streamed.Get, Styles: t.textStyles(), Options: textview.Options{Streaming: true},
				StreamingCaret: core.View{Style: core.Style{Width: core.Pt(8), Height: core.Pt(16), CornerRadius: 2, Background: t.accent,
					Animation: core.KeyframeAnimation{Keyframes: core.KeyframesPulse}}}},
		),
	}}
}
