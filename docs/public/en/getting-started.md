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

## Install nsgo

Install the `nsgo` command once, with Homebrew:

```sh
brew install nsgo-dev/tap/nsgo
```

Or with the install script, which puts it in `~/.local/bin`, or `$NSGO_INSTALL_DIR`, on macOS and Linux; `NSGO_VERSION=vX.Y.Z` picks a release:

```sh
curl -fsSL https://nsgo.dev/install.sh | sh
```

Or with Go, on any system, Windows included:

```sh
go install github.com/nsgo-dev/nsgo/cmd/nsgo@latest
```

`nsgo version` prints the version installed.

## Create an app

Let `nsgo` write an app and its module:

```sh
nsgo init -module example.com/hello hello
cd hello
```

`init` writes:

| File | What it is |
| --- | --- |
| `app.go` | The app, a counter: a `Root` component and `main`, which calls `nsgo.Run(Root)` |
| `app_test.go` | A test that presses the button |
| `nsgo.yaml` | The app's name, id, version and targets |
| `go.mod` | The module, named by `-module` or else after the directory, requiring the NSGo that ran `init` and running it as a tool |
| `go.sum` | The checksums of what the app needs, which `init` fills with `go mod tidy` |

In a module of your own, `nsgo init` without `-module` writes the app into the current directory, and adds NSGo's `require` and `tool` lines to `go.mod` when it lacks them, changing nothing else. A development build of `nsgo`, with no version to require, writes `go.mod` without them and prints the `go get` commands that add them. `nsgo init -template tsx` writes a TSX app instead, and `-template mixed` a Go app whose TSX entry uses Go components of its own.

### The project's NSGo

The `tool` line in `go.mod` pins the project to the version of NSGo it requires; `go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo` writes it into a module that lacks it. Inside the project, an installed `nsgo` at another version runs the pinned one, `go tool nsgo`, with the same arguments, so everyone builds the project with the same NSGo whatever they installed; `nsgo version` says which ran ([The module's nsgo](building.md#the-modules-nsgo)). `go tool nsgo` also works on its own, without installing anything, as in CI and for teammates who have not installed `nsgo`:

```sh
go tool nsgo test
```

### Templates from Go modules

Pass the template's module path with an optional `@version` and `#name`. An omitted version resolves to the latest version Go's module proxy can find.

```sh
nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
nsgo init -template 'example.com/my-templates@v1.2.0#platformer' -module example.com/hello hello
nsgo init -template example.com/my-templates -list
```

Publish either a single starter in the module's `template/` directory or several starters under `template/<name>/`, not both: when `template/` is itself a starter, the tool copies it whole, subdirectories included. Select a named starter with `#<name>`, quoting the argument since some shells treat `#` specially. `-list` prints one name per line, or `(default)` for a module whose `template/` is the starter. Include `nsgo.yaml` and `main.go` in each starter. The tool renders both, plus any file ending in `.tmpl`, with Go's `text/template`, and strips `.tmpl` from output names. It copies other files, including binary assets, unchanged.

Templates receive `.Name` (the app's display name), `.ID` (the generated app ID), `.Slug` (the lower-case, hyphenated name) and `.Module` (the value of `-module`, or the slug, where a slug that names a standard library package such as `time` becomes `example.com/<slug>`). A `-module` value that names a standard library package is refused. Ship a `go.mod.tmpl` to declare the generated app's module and dependencies, since Go excludes nested `go.mod` files from module downloads. Without one, the tool writes the same `go.mod` as it does for a built-in template. Either way it adds NSGo's `require` and `tool` lines the `go.mod` lacks, keeping a version of NSGo the template requires, and runs `go mod tidy`.

## Run it

```sh
nsgo dev
```

`dev` builds the app for this machine and opens it. On each save it rebuilds and restarts the app, and a change of TSX alone updates the running app in place. A failed build shows over the app, with buttons to copy the error and to build again.

Run it somewhere else with `-target`:

```sh
nsgo dev -target web                # in the browser
nsgo dev -target ios_simulator      # in the iOS simulator
nsgo dev -target tvos_simulator     # in the Apple TV simulator
nsgo dev -target watchos_simulator  # in the Apple Watch simulator
nsgo dev -target android            # on a connected device or emulator
```

`nsgo doctor` lists every target with whether this machine builds it and runs it, and what it lacks for those it cannot, such as Xcode, `adb`, a connected device or GTK.

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
