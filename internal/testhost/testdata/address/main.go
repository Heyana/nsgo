package main

import (
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

func main() {
	nsgo.Run(func(cx *core.Scope) core.Node {
		initial := cx.AppURL()
		return core.View{Style: core.Style{Padding: core.All(24), Gap: 12, Background: core.RGB(0xffffff), Color: core.RGB(0x000000)}, Children: []core.Node{
			core.Text{Content: "initial=" + initial},
			core.Text{TestID: "address", ContentFunc: func() string { return "current=" + cx.AppURL() }},
			core.Text{TestID: "push", Content: "Choose film", Style: core.Style{Padding: core.All(12)}, OnPress: func(core.PressEvent) {
				if err := cx.SetAppURL("/films/42?list=two", core.AppURLOptions{History: core.HistoryPush}); err != nil {
					panic(err)
				}
			}},
		}}
	})
}
