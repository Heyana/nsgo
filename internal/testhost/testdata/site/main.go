package main

import (
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/web"
)

func main() {
	nsgo.Run(func(cx *core.Scope) core.Node {
		article := core.NewSignal(cx, false)
		return core.View{Style: core.Style{Padding: core.All(20), Gap: 10}, Children: []core.Node{
			web.Head{Title: "Site", Description: "Site summary"},
			core.Text{TestID: "heading", Content: "Site heading", Role: core.RoleHeader, AccessibilityLevel: func() int { return 1 }},
			core.Image{Source: core.ImageSource{URI: "asset:/mark.svg"}, Style: core.Style{Width: core.Pt(20), Height: core.Pt(20)}},
			base.Link{TestID: "article", Href: "/article", OnOpen: func(string) { article.Set(true) },
				Style: core.Style{Width: core.Pt(180), Height: core.Pt(40)}, Children: []core.Node{core.Text{Content: "Open article"}}},
			core.Show{When: article.Get, Then: func(*core.Scope) core.Node {
				return core.Group{web.Head{Title: "Article", Description: "Article summary"},
					base.Button{TestID: "back", Label: "Back", OnPress: func(core.PressEvent) { article.Set(false) }}}
			}},
		}}
	})
}
