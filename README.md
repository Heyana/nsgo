<p align="center">
  <img width="128" height="128" alt="NSGo" src="examples/gallery/assets/Icon.png">
</p>

# NSGo

**English** | [简体中文](README.zh-CN.md)

NSGo is a cross-platform native UI engine with a Go backend. App logic, the reactive graph and layout run in Go, and each platform draws with its own native UI.

NSGo follows the design of [Native Shell (NSH)](https://nsh.dev), which does the same in Rust. It exists for teams that want NSH's model without writing Rust.

## Status

NSGo is early. It has NSH's reactive model, layout, text, images, canvas, shaders, animations, services and unstyled base components on macOS, iOS, Windows, Linux, the web and Android, and apps embed in iOS, Android and Go desktop apps of their own. There is no release or stable API yet. [ARCHITECTURE.md](ARCHITECTURE.md#platforms) tracks each target.

## Goals

- Write apps and components in Go, with signals, memos and effects.
- Lay out with flexbox and grid, and render with each platform's own views and text.
- Target iOS, Android, macOS, Windows, Linux and the web, as NSH does.
- Embed NSGo apps in existing native apps, or package them as standalone apps.

## Usage

An app is a Go main package that calls `nsgo.Run`. Components are functions or types that return elements; signals update what reads them:

```go
package main

import (
	"fmt"

	"github.com/oneo-me/nsgo"
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		base.Button{
			Label:    "Press me",
			OnPress:  func(core.PressEvent) { count.Update(func(n int) int { return n + 1 }) },
			Style:    core.Style{Padding: core.Symmetric(6, 14), CornerRadius: 8, Background: core.RGB(0xbd481b), Color: core.RGB(0xffffff)},
			Children: []core.Node{core.Text{Content: "Press me"}},
		},
	}}
}

func main() { nsgo.Run(Root) }
```

Test it headless with `nsgotest`, no window needed; `go tool nsgo test -target macos` runs the same tests in a real window, with native input and screenshots (`app.Screenshot("name")`):

```go
func TestMain(m *testing.M) { nsgotest.Main(m) } // gives the platform the main thread on -target

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
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# appearance: dark      # light or dark whatever the system's
window: { width: 420, height: 560 }
icon:
  apple: assets/AppIcon.icon  # macOS and iOS, or an .icns for macOS
  png: assets/Icon.png        # every other target
dev:
  target: native        # native, web, ios, ios_simulator or android
  hot_reload: false     # true rebuilds and restarts on save
build:
  output: dist          # packages go to dist/<name>-<version>-<target>
  open: false           # true opens the output directory after building
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, android_arm64, android_x64, web]
# sign:                 # store releases; passwords come from NSGO_KEYSTORE_PASSWORD and NSGO_KEY_PASSWORD
#   profile: assets/AppStore.mobileprovision  # signs the IPA
#   keystore: assets/release.p12              # signs APKs, with key_alias
```

Beside `nsgo.yaml`, `resources/` holds files the app ships, where `asset:` images load from and `nsgo.Fonts("fonts/Brand.ttf")` registers fonts from; `assets/` holds project files that do not ship, such as the icons.

Add the tool to your module once; `go tool nsgo` then runs it without installing:

```sh
go get -tool github.com/oneo-me/nsgo/cmd/nsgo
```

`go tool nsgo dev` runs the app once, or with `dev.hot_reload` rebuilds and restarts it on each change. A failed build shows over the running app; Command or Control+R in the app, or `r` and Return in the terminal, builds it again. Its builds badge the icon DEV, or show `dev.icon`'s files, and on macOS the performance monitor.

`go tool nsgo build` writes a stripped release package per target: an `.app`, an `.ipa`, an `.apk`, Windows' and Linux's directories with a `.zip` or `.tar.gz` of each, or a web site; `-run` starts the one built for this machine. `-embed` packages the app for an iOS or Android app of your own instead, an XCFramework or an AAR whose `NSGoView` or `NsgoView` shows it, and Go desktop programs show apps with `nsgo.NewView`: see `examples/embed-*`.

`go tool nsgo test` runs the app's tests with a line per test; `-run` filters them, `-junit report.xml` writes a JUnit report and `-trace` prints each step's mount items and commit times. `-target` runs them on `macos`, `windows`, `linux`, `web` (in Chrome, Chromium or Edge), `ios_simulator` or `android`, a process per test, saving screenshots in `.nsgo/test/screenshots`. `build.targets` and `-target` accept `all` for every platform that needs only Go and Xcode; add `android_x64` when an NDK is installed.

Flags replace the `nsgo.yaml` values they name, such as `-target`, `-version` or `-output`; `dev` also takes `-device` for an iPhone, simulator or Android device and `-port` for its server. `-h` lists every flag.

## Development

Requirements: macOS with Xcode and Go 1.25 or later. Nothing else is needed to build every target for arm64, Android's APK included.

Android device and emulator development additionally needs `adb` from the Android SDK's platform-tools, and iOS device development needs an Apple ID in Xcode's Accounts, a trusted iPhone and Developer Mode on it.

The x86_64 emulator target, `android_x64`, also needs an Android NDK: Go links android/amd64 only with cgo. On Windows, Go 1.25 or later runs the native app and builds the Windows and web packages. On Linux, Go runs the native app once GTK 4.14 or newer, Pango and Cairo are installed, WebKitGTK 6.0 for web views and GStreamer plug-ins for videos.

Run the gallery from the repository root; it shows every element, `base` component and service in a standard window.

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build examples/gallery                 # release packages in examples/gallery/dist
go tool nsgo build -target all examples/gallery     # every platform's release packages
go test ./...
bench/run.sh                                       # measure the quotes benchmark (bench/README.md)
```

NSGo uses NSH as its reference implementation. Clone it into `nsh/` at the repository root; Git ignores that directory:

```sh
git clone https://github.com/longbridge/nsh.git nsh
```

Read [ARCHITECTURE.md](ARCHITECTURE.md) and [AGENTS.md](AGENTS.md) before changing code.

The website is the gallery compiled to the web at <https://oneo-me.github.io/nsgo/>. Run it with `go tool nsgo dev -target web examples/gallery`; the [Website workflow](.github/workflows/website.yml) publishes it to GitHub Pages.

## License

Licensed under the [Apache License, Version 2.0](LICENSE). NSGo is a port of NSH, and [NOTICE](NOTICE) records the attribution that port owes.

Unless you explicitly state otherwise, any contribution intentionally submitted for inclusion in NSGo by you, as defined in the Apache-2.0 license, shall be licensed under Apache-2.0, without any additional terms or conditions.

## Author

[Blog](https://oneo.me) · [X @oneo666](https://x.com/oneo666)
