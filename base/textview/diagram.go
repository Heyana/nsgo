package textview

import (
	"github.com/nsgo-dev/nsgo/base/internal/diagramlayout"
	"github.com/nsgo-dev/nsgo/core"
)

// diagramBlock keeps the source selectable while giving a renderer the viewport's width independently of the attachment's intrinsic size.
func (r *renderer) diagramBlock(b CodeBlock) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		width := core.NewSignal(cx, core.Points(0))
		diagramlayout.Width.Provide(cx, width.Get)
		diagram := r.selectable(core.Text{Style: core.Style{MinWidth: core.Pct(100), TextAlign: core.TextAlignCenter},
			SelectionBlock: &core.SelectionBlock{
				Kind: core.SelectionBlockCode, Language: b.Language, Meta: b.Meta,
				Inlines: []core.SelectionInline{{Kind: core.SelectionInlineText, Text: b.Code, UTF16Len: 1}},
			},
			Spans: []core.Span{{Content: "```" + b.Language + "\n" + b.Code + "\n```", Attachment: r.tv.RenderDiagram(cx, b)}},
		})
		return core.View{Class: r.class(PartDiagramBlock), StyleFunc: func() core.Style { return r.style(PartDiagramBlock) }, Children: []core.Node{
			core.Scroll{Horizontal: true, HideIndicator: r.tv.NoDiagramBlockScrollbar,
				Class: r.class(PartDiagramBlockScroll), StyleFunc: func() core.Style { return r.style(PartDiagramBlockScroll) },
				OnLayout: func(b core.Bounds) { width.Set(b.Size.Width) }, Children: []core.Node{diagram}},
		}}
	})
}
