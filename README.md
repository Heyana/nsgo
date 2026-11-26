<p align="center">
  <img src="examples/gallery/assets/Icon.png" alt="NSGo" width="128">
</p>

<h1 align="center">NSGo</h1>

<p align="center">
  <strong>English</strong> |
  <a href="README.zh-CN.md">简体中文</a>
</p>

<p align="center">
  <a href="https://github.com/nsgo-dev/nsgo/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/nsgo-dev/nsgo"></a>
  <a href="https://pkg.go.dev/github.com/nsgo-dev/nsgo"><img alt="Go Reference" src="https://pkg.go.dev/badge/github.com/nsgo-dev/nsgo.svg"></a>
  <a href="LICENSE"><img alt="License" src="https://img.shields.io/badge/license-Apache--2.0-blue"></a>
</p>

<p align="center">
  <a href="https://github.com/oneo-me"><img alt="GitHub" src="https://img.shields.io/badge/GitHub-oneo--me-181717?logo=github&logoColor=white"></a>
  <a href="https://oneo.me"><img alt="Blog" src="https://img.shields.io/badge/Blog-oneo.me-0969da"></a>
  <a href="https://x.com/oneo666"><img alt="X" src="https://img.shields.io/badge/X-%40oneo666-000000?logo=x&logoColor=white"></a>
  <a href="https://t.me/nsgo_dev"><img alt="Telegram" src="https://img.shields.io/badge/Telegram-%40nsgo__dev-26a5e4?logo=telegram&logoColor=white"></a>
  <a href="https://github.com/sponsors/oneo-me"><img alt="GitHub Sponsors" src="https://img.shields.io/badge/GitHub%20Sponsors-ea4aaa?logo=githubsponsors&logoColor=white"></a>
</p>

<p align="center">
  A cross-platform native UI engine with a Go backend: app logic, the reactive graph and layout run in Go, and each platform draws with its own native UI.<br>
  It follows the design of <a href="https://nsh.dev">Native Shell (NSH)</a>, for teams that want NSH's model without writing Rust.
</p>

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="website/src/assets/screens/desktop-en-dark.jpg">
    <img src="website/src/assets/screens/desktop-en-light.jpg" alt="The gallery's Stocks demo on macOS" width="75%">
  </picture>
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="website/src/assets/screens/phone-en-dark.jpg">
    <img src="website/src/assets/screens/phone-en-light.jpg" alt="The gallery's component list on a phone" width="21.6%">
  </picture>
</p>

## Highlights

- **Native views.** Text, input methods, scrolling, menus and accessibility are each platform's own, and what one platform has, such as Liquid Glass, the others imitate.
- **Every platform from one Mac.** Go builds every target, with Xcode for iOS, tvOS and watchOS and an NDK only for the x86_64 Android emulator: no Android SDK, JDK, GTK or Windows toolchain.
- **Fine-grained updates.** Components run once and signals update only what reads them, with flexbox and grid layout, CSS-like styles, Tailwind classes, themes and animations on the platform's frame clock.
- **Components.** `base` offers unstyled components from buttons to a million-row `DataTable`, charts, flow diagrams, a code editor, a rich-text editor and a terminal, and `ui` gives them NSGo's look.
- **Platform services.** Navigation, notifications, permissions, pickers, sharing, haptics, in-app purchases, widgets and Live Activities, and on desktops windows, panels, menus, tray icons and global shortcuts.
- **Localized.** YAML message catalogs with CLDR plurals, numbers and dates, and right-to-left layout.
- **TSX apps.** Write apps in TSX as NSH does, with hot updates that keep state, and ship them as signed, sandboxed packages a Go app runs.
- **Tests that act as users do.** `nsgotest` presses, types and reads what is on screen, headless or on each platform with native input and screenshots.
- **The `nsgo` tool.** Create apps, run them with hot reload and DevTools, and package them for app stores, Flathub and direct downloads with updates.
- **Small and fast.** A counter app packs to about 8 MB on the desktops and starts faster than NSH, MyGo and GPUI in NSGo's benchmarks.

## Platforms

| Platform | Draws with | Status |
| --- | --- | --- |
| macOS | AppKit and Core Text | Runs |
| iOS and App Clips | UIKit and Core Text | Runs in the simulator and on devices; App Clips in the simulator |
| tvOS | UIKit, driven by the remote's focus | Runs in the simulator |
| watchOS | SwiftUI and Core Graphics | Runs in the simulator |
| Android | Android Views | Runs on arm64 devices and x86_64 emulators |
| Windows | Direct2D and DirectWrite | Runs |
| Linux | GTK 4, Pango and Cairo | Runs |
| Web | DOM elements, from WebAssembly | Runs |
| HarmonyOS | ArkUI | Preview |
| WeChat, QQ, Alipay and Douyin mini programs | Each platform's views | Experimental, within their 2 MB limit |

Apps also embed in iOS, Android, Go desktop and web apps of their own. [ARCHITECTURE.md](ARCHITECTURE.md#platforms) says how each target reaches its system.

## Getting started

With [Go](https://go.dev/dl/) 1.27.3 or later, and Xcode for iOS, tvOS and watchOS, install `nsgo` ([other ways](docs/public/en/getting-started.md#install-nsgo)), then create an app and run it with hot reload:

```sh
brew install nsgo-dev/tap/nsgo
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
nsgo init   # -template tsx for a TSX app
nsgo dev    # -target web, ios_simulator, tvos_simulator, watchos_simulator, android or miniprogram_wechat to run elsewhere
nsgo doctor # which targets this machine builds and runs, and what the others lack
```

An app is Go components that return elements, with signals updating what reads them:

```go
package main

import (
	"fmt"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}

func main() { nsgo.Run(Root) }
```

Its tests use the app as a user does, and `nsgo test` runs them headless or, with `-target`, on a platform:

```go
func TestPress(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Press("role:button Press me")
	app.Find("text:Pressed 1 times")
}
```

To try what is on `main` before it is released, run `go get github.com/nsgo-dev/nsgo@main`.

## Status

NSGo is early, and its API is not stable yet. [Releases](https://github.com/nsgo-dev/nsgo/releases) list what each release changed, and the [roadmap](docs/roadmap.md) what is still missing.

## Documentation

The [docs](https://nsgo.dev/docs/) cover components and state, layout, styles, navigation, a component reference, TSX apps, localization, testing, embedding, building and shipping; the [live demo](https://nsgo.dev/gallery/) runs the gallery in the browser.

Coding agents learn NSGo from its skill, `bunx skills add nsgo-dev/nsgo`, and read the docs as Markdown from [llms.txt](https://nsgo.dev/llms.txt).

## Development

```sh
go tool nsgo dev examples/gallery   # run the gallery
go tool nsgo check                  # vet every package
go tool nsgo test                   # every package's tests
```

[Contributing](https://nsgo.dev/docs/contributing/) has the rest of the setup. Read [ARCHITECTURE.md](ARCHITECTURE.md) and [AGENTS.md](AGENTS.md) before changing code, and [CHANGELOG.md](CHANGELOG.md) for what changed since the last release.

## Community

Questions, ideas and what you build with NSGo are welcome in nsgo-dev-community, the Telegram group [@nsgo_dev](https://t.me/nsgo_dev). Report bugs and propose changes in [issues](https://github.com/nsgo-dev/nsgo/issues).

<p align="center">
  <a href="https://t.me/nsgo_dev"><img src="docs/assets/telegram-qr.png" alt="The QR code of the Telegram group @nsgo_dev" width="200"></a>
</p>

## Sponsors

[Depot](https://depot.dev) sponsors NSGo's CI: its runners build and test NSGo on Linux, macOS and Windows.

<p align="center">
  <a href="https://depot.dev"><picture>
    <source media="(prefers-color-scheme: dark)" srcset="website/src/assets/sponsors/depot-dark.svg">
    <img src="website/src/assets/sponsors/depot-light.svg" alt="Depot" height="40">
  </picture></a>
</p>

## License

Licensed under the [Apache License, Version 2.0](LICENSE). NSGo is a port of NSH, and [NOTICE](NOTICE) records the attribution that port owes.

Unless you explicitly state otherwise, any contribution intentionally submitted for inclusion in NSGo by you, as defined in the Apache-2.0 license, shall be licensed under Apache-2.0, without any additional terms or conditions.
