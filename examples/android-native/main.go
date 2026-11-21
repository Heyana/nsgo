// Command android-native shows a view from a precompiled Android library.
package main

import (
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

const badgeName = "badge"

func main() {
	nsgo.Run(root, nsgo.Component(badgeName, func(*core.NativeContext) core.NativeComponent {
		return badgeView{}
	}))
}

func root(*core.Scope) core.Node {
	return core.View{
		Style: core.Style{Width: core.Pct(100), Height: core.Pct(100),
			AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
		Children: []core.Node{core.Native{
			Component: badgeName,
			Props:     map[string]any{"caption": "Packaged DEX"},
			Style:     core.Style{Width: core.Pt(280), Height: core.Pt(60)},
		}},
	}
}

type badgeView struct{}

func (badgeView) View() any           { return "dev.nsgo.example.BadgeView" }
func (badgeView) SetProp(string, any) {}
func (badgeView) Command(string, any) {}
func (badgeView) Close()              {}
