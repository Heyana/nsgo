import { codeToHtml } from "shiki"

// The counter example from the README, highlighted while the site builds.
const counter = `package main

import (
	"fmt"

	"github.com/oneo-me/nsgo"
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
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
				OnPress: func() { count.Update(func(n int) int { return n + 1 }) },
				Children: []core.Node{core.Text{Content: "Press me"}},
			},
		},
	}
}

func main() { nsgo.Run(Root) }
`

export async function homeData() {
  return {
    counter: await codeToHtml(counter.trimEnd(), {
      lang: "go",
      themes: { light: "github-light", dark: "github-dark" },
      defaultColor: false,
    }),
  }
}
