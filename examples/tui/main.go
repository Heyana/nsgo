// Command tui runs an ordinary NSGo view tree in a terminal. The styles
// are chosen explicitly by the application; core and base do not switch
// component forms or sizes when a terminal backend is selected.
package main

import (
	"fmt"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

func main() { nsgo.Run(Root) }

func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	name := core.NewSignal(cx, "")
	add := func() { count.Update(func(n int) int { return n + 1 }) }
	cx.OnKeyWith("mod+k", core.KeyOptions{When: core.KeyAlways}, add)
	cx.OnKeyWith("mod+q", core.KeyOptions{When: core.KeyAlways}, cx.Quit)
	buttonStyle := core.Style{Width: core.Pt(160), Height: core.Pt(48), Padding: core.All(8), BorderWidth: 1, BorderColor: core.RGB(0x648cc8)}
	return core.View{Style: core.Style{Width: core.Pct(100), Height: core.Pct(100), Padding: core.All(16), Gap: 16, Color: core.RGB(0xe6e6e6), Background: core.RGB(0x141820)}, Children: []core.Node{
		core.Text{Content: "NSGo terminal · 中文 · é", Style: core.Style{LineHeight: 16}},
		core.Text{Content: "Tab changes focus; Enter presses; Ctrl+K counts; Ctrl+Q quits.", Style: core.Style{LineHeight: 16}},
		core.Input{TestID: "name", Value: name.Get, OnChange: name.Set, Placeholder: "Name", Style: buttonStyle.Merge(core.Style{Width: core.Pt(320)})},
		core.View{TestID: "count", Role: core.RoleButton, Label: "Count", Style: buttonStyle, OnPress: func(core.PressEvent) { add() }, Children: []core.Node{core.Text{Content: "Count"}}},
		core.Text{TestID: "result", ContentFunc: func() string { return fmt.Sprintf("Hello %s: %d", name.Get(), count.Get()) }},
		core.Text{TestID: "size", ContentFunc: func() string {
			size := cx.Env().Window()
			return fmt.Sprintf("Surface: %.0f × %.0f", size.Width, size.Height)
		}},
	}}
}
