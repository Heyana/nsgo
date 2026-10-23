package main

import (
	_ "embed"
	"strings"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/icons/lucide"
	"github.com/oneo-me/nsgo/ui"
	"github.com/oneo-me/nsgo/ui/highlight"
)

//go:embed content/docs/index.md
var guideEnglish string

//go:embed content/docs/index.zh.md
var guideChinese string

// docsPage is the guide, in English or Chinese. Its links leave the gallery
// on the web and copy their address everywhere else.
func docsPage(cx *core.Scope, _ *theme) core.Node {
	language := core.NewSignal(cx, "en")
	toast := ui.UseToast(cx)
	open := func(url string) {
		if url == "" || strings.HasPrefix(url, "#") {
			return
		}
		if openURL(url) {
			return
		}
		cx.SetClipboard(url)
		toast(ui.Toast{Title: "Link copied", Description: url, Icon: lucide.Link(), Tone: ui.ToneAccent})
	}
	tab := func(value, label string) core.Node {
		return ui.Tab{Label: label, TestID: "docs-" + value, Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter},
			Selected: func() bool { return language.Get() == value }, OnPress: func() { language.Set(value) }}
	}
	return core.View{Style: core.Style{Gap: 16}, Children: []core.Node{
		ui.Tabs{Label: "Language", Style: core.Style{MaxWidth: core.Pt(280)}, Children: []core.Node{
			tab("en", "English"),
			tab("zh", "简体中文"),
		}},
		ui.TextView{TestID: "docs", Selectable: true, Highlighter: highlight.New(cx), OnLinkPress: open,
			Style: core.Style{MaxWidth: core.Pt(720)},
			MarkdownFunc: func() string {
				if language.Get() == "zh" {
					return guideChinese
				}
				return guideEnglish
			}},
	}}
}
