// Command spanbackgrounds exercises multiline span paint on native hosts.
package main

import (
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

func main() { nsgo.Run(App) }

func App(cx *core.Scope) core.Node {
	changed := core.NewSignal(cx, false)
	span := func(content string, top core.Length) core.Span {
		return core.Span{Content: content, Style: core.Style{Background: core.RGB(0xffcc00),
			CornerRadius: 6, BackgroundInsetTop: top, BackgroundInsetBottom: core.Pt(2)}}
	}
	return core.View{Style: core.Style{FlexGrow: 1, Background: core.RGB(0xffffff), Padding: core.All(20), Gap: 16}, Children: []core.Node{
		core.Text{Content: "Span backgrounds", Style: core.Style{FontSize: 20}},
		core.Text{TestID: "wrapped", Style: core.Style{Width: core.Pt(160), FontSize: 18},
			Spans: []core.Span{span("A highlighted phrase wraps across several lines.", core.Pt(0))}},
		core.Text{TestID: "band", Style: core.Style{Width: core.Pt(160), FontSize: 18},
			Spans: []core.Span{span("😀 A lower band also wraps.", core.Pct(62))}},
		core.Text{TestID: "changing", Style: core.Style{Width: core.Pt(160), FontSize: 18},
			SpansFunc: func() []core.Span {
				if changed.Get() {
					return []core.Span{{Content: "Paint removed"}}
				}
				return []core.Span{span("Paint changes", core.Pct(50))}
			}},
		core.View{Role: core.RoleButton, TestID: "toggle", Style: core.Style{Padding: core.All(8), Background: core.RGB(0xeeeeee)},
			OnPress: func(core.PressEvent) { changed.Set(!changed.Get()) }, Children: []core.Node{core.Text{Content: "Toggle paint"}}},
	}}
}
