package textview

import (
	"github.com/nsgo-dev/nsgo/base/internal/mathlayout"
	"github.com/nsgo-dev/nsgo/core"
)

func (r *renderer) mathBlock(b Math) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		var viewport core.Ref
		mathlayout.Width.Provide(cx, func() core.Points {
			if box, ok := viewport.ContentBox(); ok {
				return box.Size.Width
			}
			return 0
		})
		formula := r.selectable(core.Text{Style: core.Style{MinWidth: core.Pct(100), TextAlign: core.TextAlignCenter},
			SelectionBlock: &core.SelectionBlock{
				Kind: core.SelectionBlockMath, TeX: b.TeX,
				Inlines: []core.SelectionInline{{Kind: core.SelectionInlineMath, Text: b.TeX, UTF16Len: 1}},
			},
			Spans: []core.Span{{Content: "$$\n" + b.TeX + "\n$$", Attachment: r.tv.RenderMath(cx, b)}},
		})
		return core.View{Class: r.class(PartMathBlock), StyleFunc: func() core.Style { return r.style(PartMathBlock) }, Children: []core.Node{
			core.Scroll{Ref: &viewport, Horizontal: true, HideIndicator: r.tv.NoMathBlockScrollbar,
				Class: r.class(PartMathBlockScroll), StyleFunc: func() core.Style { return r.style(PartMathBlockScroll) }, Children: []core.Node{formula}},
		}}
	})
}
