package main

import (
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

func main() {
	nsgo.Run(func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: func() string {
			if cx.Env().Network().Online {
				return "Online"
			}
			return "Offline"
		}}
	})
}
