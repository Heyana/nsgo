// Command hello shows a text, an input and a button on every target.
package main

import (
	"fmt"

	"github.com/oneo-me/nsgo"
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// Root shows a text, an input and a button.
func Root(cx *core.Scope) *core.Element {
	count := core.NewSignal(cx, 0)
	name := core.NewSignal(cx, "")

	greeting := func() string {
		if name.Get() == "" {
			return "Hello, NSGo!"
		}
		return "Hello, " + name.Get() + "!"
	}

	return core.View(
		core.TextFn(greeting).Style(core.Style{FontSize: 28, FontWeight: core.FontWeightBold, Color: core.RGB(0x1f2328)}),
		core.Input().
			Value(name.Get).
			OnChange(name.Set).
			Placeholder("Your name").
			Style(core.Style{Height: 40, Padding: core.Symmetric(0, 8), FontSize: 16, Color: core.RGB(0x1f2328),
				Background: core.RGB(0xffffff), CornerRadius: 8}),
		base.Button(func() { count.Update(func(n int) int { return n + 1 }) },
			core.Text("Press me").Style(core.Style{FontSize: 17, FontWeight: core.FontWeightBold, Color: core.RGB(0xffffff)}),
		).Style(core.Style{Background: core.RGB(0x1f883d), CornerRadius: 10, Padding: core.Symmetric(12, 24),
			AlignItems: core.AlignCenter, AlignSelf: core.AlignStart}),
		core.TextFn(func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }).
			Style(core.Style{FontSize: 15, Color: core.RGB(0x6e7781)}),
	).Style(core.Style{FlexGrow: 1, Padding: core.Edges{Top: 64, Right: 24, Bottom: 24, Left: 24}, Gap: 16,
		Background: core.RGB(0xf4f5f7)})
}

func main() { nsgo.Run(Root) }
