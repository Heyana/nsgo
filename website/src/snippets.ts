// The home page's code, after README.md's Usage.

export const goApp = `package main

import (
	"fmt"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{
		Style: core.Style{Padding: core.All(24), Gap: 12},
		Children: []core.Node{
			core.Text{ContentFunc: func() string {
				return fmt.Sprintf("Pressed %d times", count.Get())
			}},
			base.Button{
				Label:   "Press me",
				OnPress: func(core.PressEvent) {
					count.Update(func(n int) int { return n + 1 })
				},
				Style: core.Style{
					Padding:      core.Symmetric(6, 14),
					CornerRadius: 8,
					Background:   core.RGB(0x007aff),
					Color:        core.RGB(0xffffff),
				},
				Children: []core.Node{core.Text{Content: "Press me"}},
			},
		},
	}
}

func main() { nsgo.Run(Root) }`;
