<p align="center">
  <img width="128" height="128" alt="NSGo" src="docs/images/logo.png">
</p>

# NSGo

**English** | [简体中文](README.zh-CN.md)

NSGo is a cross-platform native UI engine with a Go backend. App logic, the reactive graph and layout run in Go, and each platform draws with its own native UI.

NSGo follows the design of [Native Shell (NSH)](https://nsh.dev), which does the same in Rust. It exists for teams that want NSH's model without writing Rust.

## Status

NSGo is early. The engine has NSH's reactive model, block, flexbox and grid layout, lists, images, canvas, overlays, dialogs, navigation, unstyled base components and themed `ui` components, running on macOS, iOS, Windows and the web; Android is in progress and Linux is planned. There is no release or stable API yet. [ARCHITECTURE.md](ARCHITECTURE.md#platforms) tracks each target.

## Goals

- Write apps and components in Go, with signals, memos and effects.
- Lay out with flexbox and grid, and render with each platform's own views and text.
- Target iOS, Android, macOS, Windows, Linux and the web, as NSH does.
- Embed NSGo in existing native apps, or package it as a standalone app.

## Usage

An app is a Go main package that calls `nsgo.Run`. Components are functions or types that return elements; signals update what reads them:

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
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Label: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}

func main() { nsgo.Run(Root) }
```

Test it headless with `nsgotest`, no window needed:

```go
func TestPress(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Press("text:Press me")
	app.Find("text:Pressed 1 times")
}
```

Beside the package, `nsgo.yaml` names the app and its targets:

```yaml
name: Hello
id: dev.oneo.nsgo.hello
version: 0.1.0
window: { width: 420, height: 560 }
dev:
  target: native        # native, web or ios_simulator
  hot_reload: false     # run once and exit with the app; true rebuilds and restarts on save
build:
  output: dist          # packages go to dist/<name>-<version>-<target>
  open: true            # open the output directory after building
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, ios, ios_simulator, web]
```

Add the tool to your module once; `go tool nsgo` then runs it without installing:

```sh
go get -tool github.com/oneo-me/nsgo/cmd/nsgo
```

`go tool nsgo dev` runs the app once and exits when the app does. Set `dev.hot_reload` to `true` to rebuild and restart it when a source file changes. `go tool nsgo build` writes a stripped release package per target and opens the output directory, unless `build.open` is `false`.

Both commands' flags replace the `nsgo.yaml` values they name: `-name`, `-id`, `-version`, `-width` and `-height` on both, `-target` and `-hot-reload` on `dev`, and `-target`, `-output` and `-open` on `build`. `-h` lists every flag.

## Development

Requirements: macOS with Xcode and Go 1.25 or later. Nothing else is needed for any target. On Windows, Go 1.25 or later runs the native app and builds the Windows and web packages.

Run the example from the repository root:

```sh
go tool nsgo dev examples/hello                     # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev -target web examples/hello         # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/hello
go tool nsgo build examples/hello                   # release packages in examples/hello/dist
go test ./...
```

On Windows, `go test ./platform/windows` opens a real window to check input and painting; it skips where no desktop is available.

NSGo uses NSH as its reference implementation. Clone it into `nsh/` at the repository root; Git ignores that directory:

```sh
git clone https://github.com/longbridge/nsh.git nsh
```

Read [ARCHITECTURE.md](ARCHITECTURE.md) and [AGENTS.md](AGENTS.md) before changing code.

The website is a separate Bun project in `website/`; [website/README.md](website/README.md) has its commands.

## License

Licensed under the [Apache License, Version 2.0](LICENSE). NSGo is a port of NSH, and [NOTICE](NOTICE) records the attribution that port owes.

Unless you explicitly state otherwise, any contribution intentionally submitted for inclusion in NSGo by you, as defined in the Apache-2.0 license, shall be licensed under Apache-2.0, without any additional terms or conditions.

## Author

[Blog](https://oneo.me) · [X @oneo666](https://x.com/oneo666)
