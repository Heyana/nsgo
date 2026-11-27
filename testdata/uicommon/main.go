// Command uicommon shows common ui controls: linked_test.go checks that an app
// using them links no YAML parser, goldmark, SVG/XML parser, CLDR tables, or
// unused updater crypto.
package main

import (
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

func main() {
	nsgo.Run(func(cx *core.Scope) core.Node {
		name := core.NewSignal(cx, "")
		on := core.NewSignal(cx, false)
		checked := core.NewSignal(cx, false)
		return core.View{
			Style: core.Style{Padding: core.All(16), Gap: 12},
			Children: []core.Node{
				ui.Badge{Text: "New"},
				ui.Avatar{Name: "Alice"},
				ui.Input{Value: name.Get, OnChange: name.Set, Placeholder: "Name"},
				ui.Button{Text: "Submit", OnPress: func() {}},
				ui.Switch{Checked: on.Get, OnChange: on.Set, Label: "Notifications"},
				ui.Checkbox{Checked: checked.Get, OnChange: checked.Set, Text: "Agree"},
			},
		}
	})
}
