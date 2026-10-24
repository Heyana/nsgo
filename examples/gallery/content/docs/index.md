NSGo is a cross-platform native UI engine with a Go backend. An app's logic, its reactive graph and its layout are Go; each platform draws with its own views.

This site is the gallery, compiled to the web by NSGo itself. Every page in the sidebar runs here, and this guide is drawn with the same text view the app uses. On the web, the address bar names the open page, so a link can open it.

## Getting started

An app is a Go main package that calls `nsgo.Run`:

```go
package main

import (
	"fmt"

	"github.com/oneo-me/nsgo"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/ui"
)

func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return ui.ThemeView{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string {
			return fmt.Sprintf("Pressed %d times", count.Get())
		}},
		ui.Button{Label: "Press me", OnPress: func(core.PressEvent) {
			count.Update(func(n int) int { return n + 1 })
		}},
	}}
}

func main() { nsgo.Run(Root) }
```

Run it in a browser, or on this machine with its native backend:

```sh
go tool nsgo dev -target web .
go tool nsgo dev .
```

`nsgo.yaml` names the app and the targets `nsgo build` writes. The [README](https://github.com/oneo-me/nsgo#usage) walks through the smallest app and the tool, and [ARCHITECTURE.md](https://github.com/oneo-me/nsgo/blob/main/ARCHITECTURE.md) describes the engine's packages, platforms and boundaries.

## Explore

The sidebar groups every page: Overview for the welcome and this guide, Icon library for Lucide, then the themed **ui** components and the gallery's window. Search finds any page from the keyboard, and every control on a page is live. The elements and the unstyled **base** components they build on have their own gallery, `examples/base` in the repository.

The Theme page restyles the whole gallery at once, in the system's appearance or one chosen by hand.

## Status

NSGo is early. [CHANGELOG.md](https://github.com/oneo-me/nsgo/blob/main/CHANGELOG.md) records what each change adds, and the [roadmap](https://github.com/oneo-me/nsgo/blob/main/docs/roadmap.md) orders what comes next.
