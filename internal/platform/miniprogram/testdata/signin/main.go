package main

import (
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

// This owned acceptance app displays only the host and whether it returned
// a code. It never logs, stores or sends the short-lived credential.
func main() {
	nsgo.Run(func(cx *core.Scope) core.Node {
		status := core.NewSignal(cx, "Ready")
		return core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), Padding: core.All(20), Gap: 16, Color: core.RGB(0x111111), Background: core.RGB(0xffffff)}, Children: []core.Node{
			core.Text{ContentFunc: status.Get},
			core.View{TestID: "signin", Role: core.RoleButton, Style: core.Style{Width: core.Pt(220), Height: core.Pt(60), Background: core.RGB(0x2255aa)}, Children: []core.Node{core.Text{Content: "Request host login code"}}, OnPress: func(core.PressEvent) {
				cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, func(c core.MiniProgramCredential, err error) {
					if err != nil {
						status.Set(err.Error())
						return
					}
					status.Set("Host " + string(c.Platform) + " returned a nonempty login code")
				})
			}},
		}}
	})
}
