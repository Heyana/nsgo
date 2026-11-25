---
title: Setup
description: Install what NSGo needs, create an app and run it on every target.
group: Get started
order: 10
---

## Requirements

- [Go](https://go.dev/dl/) 1.27.3 or later.
- To build for iOS, tvOS and watchOS, a Mac with Xcode. Every other target builds from macOS with Go alone; the x86_64 Android emulator's also needs an Android NDK.
- To run on an iPhone, an Apple ID in Xcode's Accounts and the iPhone trusted, with Developer Mode on.
- To run on Android, `adb` from the Android SDK's platform-tools.
- To run on Windows, Go alone, which also builds the Windows and web packages.
- On Linux, GTK 4.14 or newer, Pango and Cairo. Network images use libsoup 3, web views use WebKitGTK 6.0, and audio and video use GStreamer plug-ins. On systems with AppArmor user namespace restrictions, [WebKitGTK needs a bubblewrap profile](debugging.md#linux-web-views).

## Create an app

Make a module, add NSGo and its tool, then let the tool write an app:

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
```

`go get -tool` adds `nsgo` to the module once, so `go tool nsgo` runs it without installing anything. `init` writes:

| File | What it is |
| --- | --- |
| `app.go` | The app, a counter: a `Root` component and `main`, which calls `nsgo.Run(Root)` |
| `app_test.go` | A test that presses the button |
| `nsgo.yaml` | The app's name, id, version and targets |

`go tool nsgo init -template tsx` writes a TSX app instead, and `-template mixed` a Go app whose TSX entry uses Go components of its own.

### Templates from Go modules

Pass the template's module path with an optional `@version` and `#name`. An omitted version resolves to the latest version Go's module proxy can find.

```sh
go tool nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
go tool nsgo init -template example.com/my-templates@v1.2.0#platformer -module example.com/hello hello
go tool nsgo init -template example.com/my-templates -list
```

Publish a single starter in the module's `template/` directory, or several starters under `template/<name>/` and select one with `#<name>`. Include `nsgo.yaml` and `main.go` in each starter. The tool renders both, plus any file ending in `.tmpl`, with Go's `text/template`, and strips `.tmpl` from output names. It copies other files, including binary assets, unchanged.

Templates receive `.Name` (the app's display name), `.ID` (the generated app ID), `.Slug` (the lower-case, hyphenated name) and `.Module` (the value of `-module`, or the slug, where a slug that names a standard library package such as `time` becomes `example.com/<slug>`). A `-module` value that names a standard library package is refused. Ship a `go.mod.tmpl` to declare the generated app's module and dependencies, since Go excludes nested `go.mod` files from module downloads. Without one, `-module` writes the same minimal `go.mod` as it does for a built-in template.

## Run it

```sh
go tool nsgo dev
```

`dev` builds the app for this machine and opens it. On each save it rebuilds and restarts the app, and a change of TSX alone updates the running app in place. A failed build shows over the app, with buttons to copy the error and to build again.

Run it somewhere else with `-target`:

```sh
go tool nsgo dev -target web                # in the browser
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator
```

`go tool nsgo doctor` lists every target with whether this machine builds it and runs it, and what it lacks for those it cannot, such as Xcode, `adb`, a connected device or GTK.

[Running and debugging](debugging.md) covers devices, tracing updates, the performance monitor and other languages.

## With a coding agent

NSGo's skill teaches coding agents, such as Claude Code, Codex and Cursor, how an NSGo app is written, tested and shipped. Add it to the app's directory, or with `-g` for every project:

```sh
bunx skills add nsgo-dev/nsgo
```

Agents that read the docs themselves start from <https://nsgo.dev/llms.txt>, which lists every page as Markdown at `https://nsgo.dev/docs/<page>.md`; <https://nsgo.dev/llms-full.txt> holds them all. An agent can also use the running app ([Running and debugging](debugging.md#agents)).

## Next

- [Example: A todo app](tutorials/todo.md): state, a list and a test, in about ten minutes.
- [Example: A notes app](tutorials/notes.md): a navigation stack and notes kept on the device.
- [Example: Popular GitHub repositories](tutorials/repos.md): loading from the network with errors and a retry.
