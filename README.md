# NSGo

**English** | [简体中文](README.zh-CN.md)

NSGo is a cross-platform native UI engine with a Go backend. App logic, the reactive graph and layout run in Go, and each platform draws with its own native UI.

NSGo follows the design of [Native Shell (NSH)](https://nsh.dev), which does the same in Rust. It exists for teams that want NSH's model without writing Rust.

## Status

NSGo is early. A minimal engine renders text, text inputs and buttons on macOS, iOS and the web; Windows builds but is untested, Android is in progress and Linux is planned. There is no release or stable API yet. [ARCHITECTURE.md](ARCHITECTURE.md#platforms) tracks each target.

## Goals

- Write apps and components in Go, with signals, memos and effects.
- Lay out with flexbox and grid, and render with each platform's own views and text.
- Target iOS, Android, macOS, Windows, Linux and the web, as NSH does.
- Embed NSGo in existing native apps, or package it as a standalone app.

## Usage

An app is a Go main package that calls `nsgo.Run` and an `nsgo.yaml` beside it:

```yaml
name: Hello
id: dev.oneo.nsgo.hello
version: 0.1.0
window: { width: 420, height: 560 }
dev:
  target: native        # native, web or ios_simulator
build:
  output: dist          # packages go to dist/<name>-<version>-<target>
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, ios, ios_simulator, web]
```

Add the tool to your module once; `go tool nsgo` then runs it without installing:

```sh
go get -tool github.com/oneo-me/nsgo/cmd/nsgo
```

`go tool nsgo dev` runs the app and rebuilds it when a source file changes. `go tool nsgo build` writes a stripped release package per target.

## Development

Requirements: macOS with Xcode and Go 1.25 or later. Nothing else is needed for any target.

Run the example from the repository root:

```sh
go tool nsgo dev examples/hello                     # nsgo.yaml's dev target, rebuilt on save
go tool nsgo dev -target web examples/hello         # in the browser, reloaded on save
go tool nsgo dev -target ios_simulator examples/hello
go tool nsgo build examples/hello                   # release packages in examples/hello/dist
go test ./...
```

NSGo uses NSH as its reference implementation. Clone it into `nsh/` at the repository root; Git ignores that directory:

```sh
git clone https://github.com/longbridge/nsh.git nsh
```

Read [ARCHITECTURE.md](ARCHITECTURE.md) and [AGENTS.md](AGENTS.md) before changing code.

## Author

[Blog](https://oneo.me) · [X @oneo666](https://x.com/oneo666)
