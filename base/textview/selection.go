package textview

import (
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
)

var selectionIgnore = &core.SelectionBlock{Kind: core.SelectionBlockIgnore}

func selectionAlertName(a Alert) string {
	switch a {
	case AlertNote:
		return "NOTE"
	case AlertTip:
		return "TIP"
	case AlertImportant:
		return "IMPORTANT"
	case AlertWarning:
		return "WARNING"
	case AlertCaution:
		return "CAUTION"
	}
	return ""
}

func selectionAligns(align []ColumnAlign) []core.SelectionAlign {
	if len(align) == 0 {
		return nil
	}
	out := make([]core.SelectionAlign, len(align))
	for i, a := range align {
		switch a {
		case AlignLeft:
			out[i] = core.SelectionAlignLeft
		case AlignCenter:
			out[i] = core.SelectionAlignCenter
		case AlignRight:
			out[i] = core.SelectionAlignRight
		}
	}
	return out
}

func utf16StringLen(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// selectionInlines converts content into core.SelectionInlines aligned with the UTF-16 offsets of the spans r.spans emits for content (and the trailing StreamingCaret attachment when caret is true).
func (r *renderer) selectionInlines(content []Inline, caret bool) []core.SelectionInline {
	out := r.selectionInlinesOf(content)
	if caret {
		out = append(out, core.SelectionInline{Kind: core.SelectionInlineSkip, UTF16Len: 1})
	}
	return out
}

func (r *renderer) selectionInlinesOf(content []Inline) []core.SelectionInline {
	if len(content) == 0 {
		return nil
	}
	out := make([]core.SelectionInline, 0, len(content))
	for _, in := range content {
		switch in := in.(type) {
		case Text:
			if in.Text != "" {
				out = append(out, core.SelectionInline{Kind: core.SelectionInlineText, Text: in.Text})
			}
		case Strong:
			out = append(out, core.SelectionInline{Kind: core.SelectionInlineStrong, Children: r.selectionInlinesOf(in.Content)})
		case Emphasis:
			out = append(out, core.SelectionInline{Kind: core.SelectionInlineEmphasis, Children: r.selectionInlinesOf(in.Content)})
		case Strikethrough:
			out = append(out, core.SelectionInline{Kind: core.SelectionInlineStrikethrough, Children: r.selectionInlinesOf(in.Content)})
		case Underline:
			out = append(out, core.SelectionInline{Kind: core.SelectionInlineUnderline, Children: r.selectionInlinesOf(in.Content)})
		case Mark:
			out = append(out, core.SelectionInline{Kind: core.SelectionInlineMark, Color: in.Color, Children: r.selectionInlinesOf(in.Content)})
		case Superscript:
			out = append(out, core.SelectionInline{Kind: core.SelectionInlineSuperscript, Children: r.selectionInlinesOf(in.Content)})
		case Subscript:
			out = append(out, core.SelectionInline{Kind: core.SelectionInlineSubscript, Children: r.selectionInlinesOf(in.Content)})
		case Small:
			out = append(out, core.SelectionInline{Kind: core.SelectionInlineSmall, Children: r.selectionInlinesOf(in.Content)})
		case Code:
			out = append(out, core.SelectionInline{Kind: core.SelectionInlineCode, Text: in.Code})
		case Keyboard:
			out = append(out, core.SelectionInline{Kind: core.SelectionInlineKeyboard, Text: in.Text})
		case Math:
			si := core.SelectionInline{Kind: core.SelectionInlineMath, Text: in.TeX}
			if r.tv.RenderMath != nil {
				si.UTF16Len = 1
			}
			out = append(out, si)
		case Link:
			out = append(out, core.SelectionInline{Kind: core.SelectionInlineLink, URL: in.URL, Title: in.Title, Children: r.selectionInlinesOf(in.Content)})
		case Image:
			out = append(out, core.SelectionInline{Kind: core.SelectionInlineImage, URL: in.Src, Text: in.Alt, Title: in.Title, UTF16Len: 1})
		case Break:
			out = append(out, core.SelectionInline{Kind: core.SelectionInlineBreak, UTF16Len: 1})
		case FootnoteReference:
			out = append(out, core.SelectionInline{
				Kind:     core.SelectionInlineFootnoteRef,
				Label:    in.Label,
				Index:    in.Index,
				UTF16Len: utf16StringLen("[" + strconv.Itoa(in.Index) + "]"),
			})
		case Custom:
			if r.tv.CustomSpan != nil {
				s := r.tv.CustomSpan(in)
				if s.Attachment != nil {
					if s.Content != "" {
						out = append(out, core.SelectionInline{Kind: core.SelectionInlineText, Text: s.Content, UTF16Len: 1})
					} else {
						out = append(out, core.SelectionInline{Kind: core.SelectionInlineSkip, UTF16Len: 1})
					}
				} else if s.Content != "" {
					out = append(out, core.SelectionInline{Kind: core.SelectionInlineText, Text: s.Content})
				}
			} else if r.tv.RenderCustom != nil || r.renders(in.Name) {
				out = append(out, core.SelectionInline{Kind: core.SelectionInlineSkip, UTF16Len: 1})
			} else {
				var b strings.Builder
				in.write(&b)
				if b.Len() > 0 {
					out = append(out, core.SelectionInline{Kind: core.SelectionInlineText, Text: b.String()})
				}
			}
		}
	}
	return out
}

func documentSelectionBlocks(d Document) []core.SelectionBlock {
	out := blocksToSelection(d.Blocks)
	for _, f := range d.Footnotes {
		out = append(out, core.SelectionBlock{
			Kind:     core.SelectionBlockFootnote,
			Label:    f.Label,
			Index:    f.Index,
			Children: blocksToSelection(f.Children),
		})
	}
	return out
}

func blocksToSelection(blocks []Block) []core.SelectionBlock {
	var out []core.SelectionBlock
	for _, bl := range blocks {
		switch bl := bl.(type) {
		case Paragraph:
			out = append(out, core.SelectionBlock{
				Kind:    core.SelectionBlockParagraph,
				Inlines: inlinesToSelection(bl.Content),
			})
		case Heading:
			out = append(out, core.SelectionBlock{
				Kind:    core.SelectionBlockHeading,
				Level:   min(max(bl.Level, 1), 6),
				Inlines: inlinesToSelection(bl.Content),
			})
		case Blockquote:
			out = append(out, core.SelectionBlock{
				Kind:     core.SelectionBlockQuote,
				Alert:    selectionAlertName(bl.Alert),
				Title:    bl.Alert.String(),
				Children: blocksToSelection(bl.Children),
			})
		case List:
			items := make([]core.SelectionBlock, len(bl.Items))
			for i, it := range bl.Items {
				items[i] = core.SelectionBlock{
					Kind:     core.SelectionBlockListItem,
					Index:    i,
					Task:     it.Task,
					Done:     it.Done,
					Children: blocksToSelection(it.Children),
				}
			}
			out = append(out, core.SelectionBlock{
				Kind:     core.SelectionBlockList,
				Ordered:  bl.Ordered,
				Start:    bl.Start,
				Loose:    bl.Loose,
				Children: items,
			})
		case CodeBlock:
			out = append(out, core.SelectionBlock{
				Kind:     core.SelectionBlockCode,
				Language: bl.Language,
				Meta:     bl.Meta,
				Inlines:  []core.SelectionInline{{Kind: core.SelectionInlineText, Text: bl.Code}},
			})
		case Table:
			var cells []core.SelectionBlock
			rows := append([][]TableCell{bl.Head}, bl.Rows...)
			for ri, row := range rows {
				col := 0
				for _, c := range row {
					cells = append(cells, core.SelectionBlock{
						Kind:    core.SelectionBlockTableCell,
						Header:  ri == 0,
						Row:     ri,
						Col:     col,
						ColSpan: c.span(),
						Inlines: inlinesToSelection(c.Content),
					})
					col += c.span()
				}
			}
			out = append(out, core.SelectionBlock{
				Kind:     core.SelectionBlockTable,
				Align:    selectionAligns(bl.Align),
				Children: cells,
			})
		case Rule:
			out = append(out, core.SelectionBlock{Kind: core.SelectionBlockRule})
		case Details:
			var children []core.SelectionBlock
			if len(bl.Summary) > 0 {
				children = append(children, core.SelectionBlock{
					Kind:    core.SelectionBlockSummary,
					Inlines: inlinesToSelection(bl.Summary),
				})
			}
			children = append(children, blocksToSelection(bl.Children)...)
			out = append(out, core.SelectionBlock{
				Kind:     core.SelectionBlockDetails,
				Children: children,
			})
		case Math:
			out = append(out, core.SelectionBlock{
				Kind:    core.SelectionBlockMath,
				TeX:     bl.TeX,
				Inlines: []core.SelectionInline{{Kind: core.SelectionInlineMath, Text: bl.TeX}},
			})
		case Custom:
			switch {
			case len(bl.Children) > 0:
				out = append(out, blocksToSelection(bl.Children)...)
			case len(bl.Content) > 0:
				out = append(out, core.SelectionBlock{
					Kind:    core.SelectionBlockParagraph,
					Inlines: inlinesToSelection(bl.Content),
				})
			case bl.Text != "":
				out = append(out, core.SelectionBlock{
					Kind:    core.SelectionBlockParagraph,
					Inlines: []core.SelectionInline{{Kind: core.SelectionInlineText, Text: bl.Text}},
				})
			}
		}
	}
	return out
}

func inlinesToSelection(content []Inline) []core.SelectionInline {
	var r renderer
	return r.selectionInlinesOf(content)
}
