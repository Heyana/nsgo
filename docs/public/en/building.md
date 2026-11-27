---
title: Building and shipping
description: Package an app for every platform, make installers, sign and notarize it, publish it on GitHub and embed it in other apps.
group: Toolchain
order: 22
---

## Build

```sh
nsgo build
```

`build` writes a stripped release package for each target `nsgo.yaml` lists, into `dist/<name>-<version>-<target>`: an `.app`, an `.ipa`, an `.apk`, Windows' and Linux's directories, or a web site, whose page fetches each TSX module an `import()` names when the import runs. `-target` replaces the list, and `all` means every target that needs only Go and Xcode; add `android_x64` when an Android NDK is installed, and `miniprogram`, or `miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` or `miniprogram_douyin` alone, for [mini programs](#mini-programs), and `harmonyos` for a [HarmonyOS](#harmonyos) preview. `-run` starts the package built for this machine, or installs it on the iPhone, simulator or Android device `-device` picks.

Everything to hand out lands in `dist/releases`, the only directory to look in:

| In `dist/releases` | What |
| --- | --- |
| `*.dmg`, `*-setup.exe`, `*.deb`, `*.pkg.tar.gz`, `*.AppImage` | [Installers](#installers) for macOS, Windows and Linux |
| `*.AppImage.zsync` | Delta update controls for AppImageUpdate and compatible launchers |
| `*-ios.ipa`, `*-tvos.ipa`, `*.apk` | The iOS, tvOS and Android packages, for the stores or devices |
| `*.pkg`, `*.msix`, `*.aab`, `*.flatpak`, `*.tar.gz` and `flatpak/` | A `store` build's [store packages](#store-packages) |
| `install.sh`, `install.ps1` | Scripts installing the app from [GitHub releases](#publish-on-github) |
| `appcast-<target>.xml` | The feeds [updating](#updates) apps read |
| `homebrew/<name>.rb`, `winget/` | Direct-release [Homebrew cask and winget manifests](#channels) |
| `*-web.zip` | The site, to unpack and serve as it is |
| `miniprogram/<platform>/` | Each mini program's project, to upload from its developer tool |

Each file is named `<name>-<version>-<system>-<arch>`, such as `hello-1.2.0-windows-x64-setup.exe`, so users find their system's on a release page, and has its `.sha256` beside it. A build of another version empties the directory first; builds of the same version, on other machines too, add to it. Simulators' and watchOS's packages stay in their `dist/<name>-<version>-<target>` directories.

The web site includes `.wasm.br` and `.wasm.gz` when they are smaller than the original module. Enable precompressed files on hosts that support them. Serve them at the original `.wasm` URL with `Content-Type: application/wasm`, the matching `Content-Encoding`, and `Vary: Accept-Encoding`. The original module works on any static host. [Publishing on the web](web.md) covers semantic headings, browser links, per-page metadata and CDN loading.

### Offline web packages

Set `web.offline: true` in `nsgo.yaml` to cache a web release on its first online launch.

```yaml
web:
  offline: true
```

Serve the published directory over HTTPS, or use `nsgo build -target web -run` on localhost. The service worker caches the page, WebAssembly, scripts and packaged resources. Online visits load the published version. After the installation succeeds and the previous release's tabs close, the latest release can launch from its cache. Requests for an app's online services still need a connection.

## Terminal

Run the app in an interactive terminal with `nsgo dev -target tui`. `nsgo test -target tui` runs its tests there through `nsgotest.Main`, one application per test process. The terminal controls the size: a cell represents 8 by 16 layout points. Choose your application's styles explicitly; components keep their ordinary forms and sizes. `examples/tui` demonstrates fields, buttons, shortcuts and resizing.

Build a console executable with `nsgo build -target tui_macos_arm64`, or the matching `tui_macos_x64`, `tui_windows_arm64`, `tui_windows_x64`, `tui_linux_arm64` or `tui_linux_x64`. The `tui` build/check group includes all six, and `all` includes them too. Each package directory contains the executable and resources; it has no GUI installer. All six build with Go alone, except that a macOS terminal build linking a package that declares macOS libraries signs them ad hoc on a Mac; elsewhere it ships them as published and needs an arm64 one signed already.

The backend supports cell text, backgrounds, solid borders, clipping, scrolling, focus and text editing. It uses the terminal's font and colors; Control is `mod`, including on a Mac. Ctrl+C copies inside an editor and exits when no binding or editor takes it. Bracketed paste inserts text without running shortcuts, and the clipboard is process-local. Canvas, images, media, native hosted components, system menus/alerts, animations, zoom, bidirectional shaping, head/middle ellipsis and a system clipboard bridge are not implemented yet. `base/terminal` remains the terminal control embedded in a GUI application.

While `nsgo dev` owns the terminal, tool and application diagnostics go to `.nsgo/tui.log`. Exit restores the screen, cursor and terminal modes before returning to the shell. Terminal test screenshots rasterize the cells with a fixed portable font rather than capturing the emulator; use text assertions for Unicode glyphs missing from that face.

## nsgo.yaml

`nsgo` refuses unknown keys in this file, including misspellings nested under a section. Free-form maps such as `apple.info` and `macos.entitlements` remain open for platform-specific keys.

Tools written in Go can read `nsgo.yaml` with the same schema, validation and defaults as `nsgo build` through `github.com/nsgo-dev/nsgo/project`: `project.Load(dir)` decodes the file strictly into a `*project.Config` and resolves paths such as `Build.Output` (defaulting to `dist`) against `dir` without making network or Git calls, and `(*project.Config).Channel()` returns the effective distribution channel (`updates.channel`, else `direct` when `updates.key` is set, else `none`).

```yaml
name: Hello
id: dev.example.hello   # the bundle ID; without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# copyright: © 2026 Example  # the Windows executable's details and installer, and the macOS About panel
# appearance: dark      # light or dark whatever the system's
# theme: theme.json     # a TSX app's theme: NSGo's or a shadcn/ui one
# i18n: { dir: locales, source: en, coverage: warn }  # coverage: off allows dependency translations to fall back without a warning
icon:
  apple: assets/AppIcon.icon  # macOS, iOS and watchOS, or an .icns for macOS
  windows: assets/IconWindows.png  # optional Windows artwork
  linux: assets/IconLinux.png      # optional Linux artwork
  png: assets/Icon.png        # every other target
  tvos: assets/AppIconTV.brandassets  # the Apple TV's layered icon and top shelf art, or a wide PNG
  watchos: assets/AppIconWatch.icon   # the Apple Watch's own, drawn in circles, or a square PNG
  android:
    png: assets/IconAndroid.png  # optional bitmap; valid without adaptive layers
    foreground: assets/IconForeground.png  # Android's adaptive icon: 432 px, the logo within the center 264 px
    background: "#1F1F1F"                 # a color or a square PNG
    monochrome: assets/IconMonochrome.png  # optional: the shape Android 13's themed icons tint
dev:
  target: native        # native, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android, harmonyos or miniprogram_<platform>
  hot_reload: true      # false builds once, watching no files
  # icon: { png: assets/Icon-dev.png }  # replaces icon's files in dev's builds; defaults to assets/*-dev.*
build:
  output: dist
  tags: []              # Go build tags every build, test and check of the app takes, such as timetzdata
  ldflags: []           # the app's own linker flags, such as -X main.releaseDate=2026-11-17
  open: false           # true opens the output directory after building
  installers: true      # false leaves out the disk image, the Windows installer and the Linux packages
  # skip_installers: [macos]  # omit installers for the listed desktop systems
  jobs: 0               # how many targets build at once; 0 is half the CPUs, 1 one after another
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # the Sign in with Apple entitlement, for iOS, tvOS and Mac App Store builds
# apple: { purchases: true }  # StoreKit's library, which in-app purchases go through, in macOS, iOS and tvOS packages
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # add required-reason APIs and tracking declarations to PrivacyInfo.xcprivacy
# apple: { requires_full_screen: false }  # let an iPad run the app in Split View, Slide Over and Stage Manager, in all four orientations
# apple: { orientations: [portrait, landscape_left] }  # the orientations a full-screen app takes on an iPad
# apple: { multiple_scenes: true }  # let an iPad open more than one window of the app, each showing the main view with its own state
# apple: { info: { UIBackgroundModes: [audio] } }  # background playback for iOS Picture in Picture
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # manifest permissions beyond the runtime's
# android: { recording: true }  # the microphone service microphone.RecordWAV records in, with RECORD_AUDIO, the foreground-service permissions and POST_NOTIFICATIONS, which RecordWAV asks for so its notification shows from Android 13; a refusal still records
# android: { uses_cleartext_traffic: true }  # allow plain HTTP to any host in the APK and App Bundle
# android: { background_downloads: true }  # dataSync foreground service for cx.Download
# android: { data_sync: true }  # generic dataSync foreground service for mobile.StartDataSync
# android: { background_motion: true }  # explicit finite motion shortService; streams also need a notification title
# android: { loopback_cleartext: true }  # allow plain HTTP to loopback in the APK and App Bundle
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app: see Commands
# windows: { command_line: true }  # add <name>.com, a console companion that forwards arguments and streams to <name>.exe; not in store builds
# windows: { path: true }  # the installer puts the app's folder on the user's Path, so consoles find its commands
# windows: { publisher: Example Ltd }  # the company the Windows executable's details, its installer and the uninstall entry name; else windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # the Linux packages' maintainer, else the app's name
# linux: { categories: [Office, Dictionary] }  # the freedesktop menu categories of the app's desktop entry, at least one a main category such as Office, Utility or Game, else menus file it under Other; your own start with X-
# linux: { share: packaging/share }  # files the .deb and Arch package install under /usr/share and the AppImage in its usr/share, such as metainfo/<id>.metainfo.xml or bash-completion/completions/<command>
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # packages the .deb and Arch package also depend on and recommend, after GTK 4 and WebKitGTK 6, each as dpkg or pacman writes it
# release: { github: me/hello }  # the GitHub repository the install scripts and updates install from, else the Git remote origin's; tag_prefix: v by default
# updates: { key: <public key> }  # how the app updates, by platform and channel: see Updates
# store: { dir: store }           # store listings directory (YAML and Markdown per language)
# registries: { "@acme": https://nsgo.acme.example/registry.yaml, "@team": tools/registry.yaml }  # custom registries for nsgo add @name/item
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # the mini programs'
```

A bare OS name in `targets` — or in `nsgo build -target` and `nsgo check -target` — means its architectures (`windows` is `windows_arm64` and `windows_x64`; `android` is `android_arm64` only, as the x86_64 build needs an NDK). A tool that writes or checks `targets` shares the tool's own table through `github.com/nsgo-dev/nsgo/targets`: `targets.Names()` lists every accepted name, and `targets.Expand` replaces `all`, `tui`, `miniprogram` and a bare OS name as `nsgo build` does.

`macos_universal`, which `macos` and `all` leave out, builds both macOS architectures and joins them into one app, its libraries and widgets included. The Steam channel uses the universal app bundle. Installer builds wrap it in one disk image, with one Sparkle feed on the direct channel. `nsgo test -target` also takes the name of the architecture its tests run for: a desktop OS's on this machine's architecture and Android's on `arm64`, so `windows_x64` runs the `windows` target on an x64 Windows machine, while an architecture the target does not run for is refused with the name to use.

Flags replace the values they name, such as `-target`, `-version` or `-output`; `-h` lists them all.

`build.ldflags` holds the app's own linker flags, each written as go build's `-ldflags` takes it, such as `-X main.releaseDate=2026-11-17` or `-X 'main.channel=beta builds'`. Every build that links the app takes them: `dev` and its reloads, `build` with `-embed` too and `test` with `-target` too, on every target. `-ldflags` on `build` and `test` adds to them rather than replacing them, repeated for more. They come before the tool's own `-X` flags, and the linker keeps a variable's last, so the name, ID and version in `core.App` stay those of `nsgo.yaml`. The tool passes go its own `-ldflags`, which replaces one in `GOFLAGS`, so linker flags belong here; a headless `test`'s `-ldflags` after the directory is go test's and replaces them all.

Every go command the tool runs for the app takes `build.tags`, in `dev` and its reloads, `build` with `-embed` too, `test` with `-target` too and `check`, on every target. NSGo's own tags, such as `nsgodev` in `dev` or `nsgotest` in target tests, are added to them, as are the tags of `-tags` in `GOFLAGS`, which would otherwise be replaced; the rest of `GOFLAGS` applies as it is.

`apple.info.LSMinimumSystemVersion` raises the macOS package's minimum, and `apple.info.MinimumOSVersion` raises the selected iOS, tvOS or watchOS package's. Write quoted version strings, such as `"14.2"` or `"18.0"`, with one to three numeric parts. The runtime floors are macOS 13.0 (Go 1.27), iOS 15.0, tvOS 17.0 and watchOS 26.0; lower or malformed values fail. The mobile key must meet every selected mobile target's floor. App Clips keep their own `app_clip.minimum`. The values reach package metadata, asset compilation, signing projects and the app's linker; embedded iOS frameworks use them too, with Mac Catalyst at least 16.0 for Go's macOS 13 runtime.

`icon.windows` and `icon.linux` select square PNGs for their app packages, installer art and store logos. They default to `assets/IconWindows.png` and `assets/IconLinux.png`, then fall back to `icon.png`; `dev.icon` accepts the same settings and `-dev` default files. `icon.android.png` selects an Android bitmap independently, including when no adaptive layers are configured. Other targets keep the shared `icon.png` fallback.

An Apple TV's icon is a layered card, not a square: `tvos` takes Xcode's `.brandassets`, its `App Icon` stacks and top shelf images, or a wide image such as 1280×768, which the tool draws to fit the card over its edge color and centers on the top shelf images. Without either, the PNG sits at three quarters of the card's height. An Apple Watch masks its icon to a circle, so `watchos` takes an `.icon` drawn for watchOS's circles or a square PNG; without it the app uses `apple`, then `png`. Both fall back to `assets/AppIconTV.*` and `assets/AppIconWatch.*`, as `apple` and `png` do.

On Android, `icon.android.foreground` and `background` make the launcher icon adaptive, so launchers mask it to their shape and move it as they like instead of shrinking `png` onto a plate of their own. Each layer is 108 dp square, a PNG scaled to 432 px; the launcher shows the center 72 dp, 288 px, and masks it to a shape that may cut into everything outside the center 66 dp, 264 px, so keep the logo in that circle and let the background fill the whole square. `background` is a PNG or a color such as `"#1F1F1F"`. `monochrome`, a shape whose alpha is all that counts, is what Android 13 and later tint when the user turns on themed icons; without it the icon keeps its colors there. The package keeps `icon.android.png`, else `icon.png`, as the plain bitmap icon, which no launcher of Android 9 or later, the oldest NSGo runs on, picks over the adaptive one; without `png`, the tool draws it from the foreground over the background. Without adaptive layers, the launcher shows the Android bitmap or the shared `png`.

`android.uses_cleartext_traffic: true` opts an APK or App Bundle into plain HTTP to any host, setting both the manifest flag and its network security policy. Without that opt-in, `android.loopback_cleartext: true` allows plain HTTP only from `localhost`, `127.0.0.1` and `::1`, such as video served by the app itself. The loopback-only policy denies cleartext to other hosts; the all-host opt-in takes precedence when both settings are true. For an embedded AAR, configure the host app’s Android network security policy.

Every build also takes NSGo's own flags as it starts, such as `--nsgo-fps` ([Running and debugging](debugging.md#flags-of-every-build)). In `nsgo dev`, Command/Control+R reloads from every open window and remains available after the main view closes when another window keeps an app function running.

### Launch screen

Set `launch_screen` to show an opaque background, a centered PNG, or both on iOS and Android 12 or later. Image paths are relative to `nsgo.yaml`. The image fits proportionally inside 128 × 128 points or dp. A transparent PNG with a longest side of at least 512 pixels keeps it sharp on high-density screens.

```yaml
launch_screen:
  background: "#F5F1EA"
  image: assets/Launch.png
  dark:
    background: "#201C26"
    image: assets/LaunchDark.png
```

The system selects the appearance unless `appearance` pins it. Omit either `dark` field to reuse the corresponding value. An omitted background or image keeps the platform's default. Embedded apps get their launch screen from the host.

## Android native classes

List precompiled Android classes in the app's `nsgo.yaml` to use a custom View from `core.Native`.

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

Paths are relative to `nsgo.yaml` unless absolute. Each path names a `.dex` file or a JAR containing D8's root `classes.dex`, `classes2.dex` and later shards. Prepare those files with D8 using `--min-api 28`. On arm64, NSGo builds the APK or App Bundle from these precompiled files with Go alone. Android development sessions rebuild when a listed file changes. For an embedded AAR, the host's Android build supplies its Java dependencies. [D8 documentation](https://developer.android.com/tools/d8)

The [Android native example](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native) registers a class with a public `Context` constructor and configures it with a Go prop.

## Games

Set `game: true` in `nsgo.yaml` for a game. Android classifies the app as a game and uses the system's Game Mode optimizations. macOS and iOS opt into Game Mode, macOS defaults to the Games category, and Linux adds `Game` to the desktop entry. An explicit `macos.category` or Game Mode key in `apple.info` takes precedence.

Set `gamepad: true` when your app handles a hardware gamepad. Android declares both gamepad and touchscreen optional. macOS, iOS and tvOS declare the Extended Gamepad profile, and tvOS also the Siri Remote's Micro Gamepad, with controller keys in `apple.info` taking precedence.

## License notices

`nsgo build` puts `THIRD_PARTY_NOTICES.txt` in the standalone package's resources. It includes the license texts of the Go modules used by that target, the Go runtime and NSGo's bundled Java or JavaScript runtime. Web builds name the file by its content, as for other resources. Use `core.ThirdPartyNotices()` to show the same text in an in-app credits screen. The function returns an empty string in development, tests and builds of embedded libraries. [Native-library declarations](native-libraries.md) add their original license texts to the same notices.

By default an unknown or missing third-party license fails the build and a detected copyleft license warns. Modules listed in the app's Go workspace (`go.work`) are treated as first-party code alongside the app's own module: any license files they contain are included, and they do not trigger third-party checks. Set either rule to `error` or `warn`. The `allow` list exempts exact module paths from both rules and keeps any license text the build found.

```yaml
licenses:
  unknown: error
  copyleft: warn
  allow: [example.org/reviewed-module]
```

## Adding libraries

Add a library, a Go module, with `nsgo add`, which is `go get` with its plan shown first:

```sh
nsgo add -list                           # list libraries and components in NSGo's built-in index
nsgo add -list icons flags               # filter case-insensitively by words across name, description and tags
nsgo add media                           # resolve a short name in NSGo's built-in index
nsgo add github.com/acme/chart           # its newest release, unless go.mod has it already
nsgo add github.com/acme/chart@v1.3.0    # this version, even one below go.mod's
nsgo add -dry-run github.com/acme/chart  # print the plan and write nothing
```

Bare names (without a dot in their first segment) resolve only in NSGo's built-in index (`cmd/nsgo/registry.yaml`, embedded in `nsgo` with nothing fetched or cached). A bare name plans the same way as its module path, using the entry's `version` when present else `@upgrade`, and refuses a module with no release tag (`<name> has no release; add its module by path with a version`). Unknown names suggest `nsgo add -list`. `nsgo add -list [words...]` prints each matching item's name, kind (`library` or `component`), module path and description, and works inside or outside a module. Items with kind `component` are listed in the index and refused by `nsgo add` until component copying lands.

```text
github.com/acme/chart
  module     github.com/acme/chart v1.3.0, published 2026-11-20
  owner      github.com/acme, the first module from it in go.mod
  licenses   github.com/acme/chart: MIT
             example.com/units: MIT
  next       import "github.com/acme/chart"

example.com/hello
  go.mod     + require github.com/acme/chart v1.3.0 // indirect
             + require example.com/units v1.0.0 // indirect
             ~ require github.com/nsgo-dev/nsgo v0.4.0 => v0.4.2, a release
  go.sum     + 6 lines
  tools      go tool nsgo then runs v0.4.2, and so does an installed nsgo here
Apply these changes? [y/N]
```

For each module, the plan names its version and publication date, and its owner: the host, and on a code host such as github.com the account, marked when `go.mod` has no module from it yet. Retraction and deprecation notices, a version with no release tag, a `replace` of the module, the classified licenses of every module and native library it brings in (noting when a registry license differs from what was detected), each target's native libraries with their licenses and download hosts, any package that does not build for a target (such as when build constraints exclude every file), and the import line of its root package follow when they apply. Target inspection runs for each of the app's expanded `build.targets`, or the host's target without `nsgo.yaml`. When a module ships an [`nsgo-module.yaml`](authoring/shipping.md#module-manifest), the plan uses its declared `import` packages, names any of the app's `build.targets` its `targets` lack, lists its `components` with their `nsgo add <module>#<name>` commands, and prints the author's `notes` last and marked. Then come the lines of `go.mod` that change, `go` and `toolchain` included, the `go tool` commands that would run another version, the modules fetched from their own hosts and those `go.sum` takes unchecked by the checksum database. `go get` marks a module no package imports yet `// indirect`, and `go mod tidy` drops the mark once one does.

In a terminal, `add` asks before writing, and `-y` applies the plan without asking; without a terminal and without `-y`, it writes nothing and exits with status 1. An interrupt, such as Ctrl+C, leaves `go.mod` and `go.sum` as they were and exits with status 130. A module `go.mod` already requires is left as it is, its version printed, unless a version is typed.

Planning runs nothing but the go command, on copies of `go.mod` and `go.sum`:

- with the toolchain the module already uses and `GOTOOLCHAIN=local`, so no other toolchain is downloaded or run;
- through the module proxies `GOPROXY` lists before its first `direct`, which becomes `off`, so only the modules `GOPRIVATE` or `GONOPROXY` marks are fetched from their own hosts, with `GONOSUMDB` and `GOAUTH` as they are;
- with `GOWORK=off` for `go get`, so in a workspace it plans `go.mod` for the module alone, as `GOWORK=off go get` writes it, and says so; per-target package listing reads the planned `go.mod` and `go.sum` through `-overlay` with `-mod=readonly`, so it also runs in a workspace and ignores a `vendor` directory.

Whether or not `-y` is given, `add` refuses:

- moving NSGo's requirement to a pseudo-version or a pre-release, as a library that requires a commit of NSGo would; `go get github.com/nsgo-dev/nsgo@<version>` moves it there on purpose first;
- a module that needs a newer Go than the module's toolchain; `go get go@<version>` raises the module's first;
- lowering a module through a query such as `@latest`; typing the version lowers it, and the plan calls it a downgrade;
- a module or native library whose license fails the app's `licenses` policy (unrecognized or missing license text by default, or copyleft text under `licenses.copyleft: error`), naming the `licenses.allow` entry or `warn` setting for `nsgo.yaml`; `add` never edits the policy.

It writes `go.mod` and `go.sum` only when neither has changed since planning, each through a temporary file renamed over it, and puts both back on an error or an interrupt. In a module with a `vendor` directory, the plan names `go mod vendor` as the next step.

`add` is new after NSGo 0.3.4. In a module that pins 0.3.4 or an earlier release, an installed `nsgo` does not run [the module's nsgo](#the-modules-nsgo) for it but prints the `go get github.com/nsgo-dev/nsgo@<version>` that brings `add`; there, `go tool nsgo add` runs the pinned nsgo, which does not know `add`.

### Custom registries

Teams can name their own libraries and forks in `nsgo.yaml` under `registries:` without shadowing NSGo's own names:

```yaml
registries:
  "@acme": https://nsgo.acme.example/registry.yaml
  "@team": tools/registry.yaml
```

Each key is `@` followed by 1 to 64 lowercase letters, digits or hyphens (not starting or ending with `-`; `@nsgo` is reserved). Each value is an `https` URL without user credentials, query or fragment (`http` is accepted only for a loopback host), or a path relative to `nsgo.yaml`. Unknown keys and bad entries fail with their line number, and `nsgo` never adds an unconfigured registry to `nsgo.yaml` on its own.

```sh
nsgo add @acme/chart                 # resolve chart in @acme; bare names never check custom registries
nsgo add @acme/chart@v1.3.0          # pin a specific version
nsgo add -list                       # list NSGo's index and every configured registry's items (with their @prefix)
nsgo add -list @acme charts          # filter one registry by words across name, description and tags
```

A registry file is a single strict YAML document of at most 4 MiB with `version: 1` and an `items` list (`name`, `module`, optional `component`, optional tagged `version`, single-line `description`, SPDX `license`, optional `https` `homepage`, and optional lowercase `tags`), without anchors, aliases, merge keys, control characters or Unicode format characters. Remote registries follow up to 10 redirects (refusing redirects to plain HTTP or non-loopback HTTP), use a 30-second connect timeout and 5-minute total timeout, and cache their copy and ETag at `<user cache dir>/nsgo/registries/<sha256>.yaml`. Offline, `nsgo add` reuses the cached copy and shows its age on the plan's first line alongside the registry and URL; with no cached copy, the error names the URL. Private registries use a file in the repository or an intranet server, and private Go modules keep `GOPRIVATE` and `GOAUTH`.

## Libraries from Go packages

Desktop app builds and target tests compile app code with `CGO_ENABLED=0`, as they do NSGo itself. If an app package contains cgo files, the build stops and names them instead of silently dropping them. Use purego and `nsgo-natives.yaml` for native libraries.

Apps that import `base/terminal` also ship libghostty-vt: Windows and Linux packages place the library beside the executable; macOS and iOS bundles sign it into their frameworks. [Embedding](embedding.md) explains the companion XCFramework for iOS hosts.

A dependency can declare prebuilt desktop shared libraries in `nsgo-natives.yaml`, and from version 2 of that file frameworks for iOS and tvOS. NSGo verifies their SHA-256, copies them into the app before signing and includes them in installers; the app author runs no extra step. From version 2 it can also name Linux packages, which the `.deb` and the Arch package then depend on or recommend after the app's own `linux.depends` and `linux.recommends`. [Native libraries](native-libraries.md) describes the package author's format and loading paths.

## Commands

An app can ship command-line programs beside its desktop executable, such as a CLI working on the app's data. List them under `commands`, each built from a Go main package:

```yaml
commands:
  - name: hello-cli             # the file's name, hello-cli.exe on Windows
    package: ./cmd/hello-cli    # a main package, built for each desktop target with CGO_ENABLED=0 and build.tags
    ldflags: [-X main.channel=beta]  # its own linker flags, after build.ldflags
windows:
  path: true                    # the installer puts the app's folder on the user's Path
```

A name is letters, digits, `-` and `_`, and may not be the app's executable, its Linux command, `Uninstall`, `AppRun` or another command's in any case. `macos_universal` joins the two macOS builds of each command, as it joins the app's.

A package links with `build.ldflags` and `-ldflags`, then its own `ldflags`, whose `-X` values win, then the app's identity, so `core.App()` gives the command the name, ID, version and build number of `nsgo.yaml`, as it gives the app. A command that does not import `core` takes the version with `-X`, such as `-ldflags "-X main.version=1.2.0"` on `build`, which the app and its commands both take.

| Package | Commands |
| --- | --- |
| macOS | In `Contents/MacOS`, signed before the app as it is, as `<id>.<command>`: ad hoc, or with `sign.identity`, the hardened runtime and a timestamp, so the app [notarizes](#notarize), but without the app's entitlements. The disk image puts nothing on `PATH`; the [Homebrew cask](#channels) links each command, and a user may link `/Applications/<name>.app/Contents/MacOS/<command>` themselves |
| Windows | `<command>.exe` beside `<name>.exe`, [signed](#sign-windows-packages) as it is, which the installer installs and its uninstaller removes; with `windows.path` the installer also puts the folder on the user's `Path`, as `windows.command_line` does, so consoles opened after installing find the commands. A command an update renames or drops stays in the folder, as a file the app no longer ships does, since the uninstaller removes only what its version installed |
| Linux | In `/opt/<slug>` with a `/usr/bin/<command>` link that the `.deb` and Arch package own, so removing the package removes it, and dpkg and pacman refuse to install over another package's; the AppImage holds them beside the app and puts nothing on `PATH` |

The installers, their checksums and the [update](#updates) appcasts carry the commands as they carry the app's other files, so an update replaces them with the app. [Store packages](#store-packages) leave them out, as the Mac App Store's sandbox, the MSIX and the Flatpak run only the app's own executable.

## Extensions

A command `nsgo` does not know itself runs an extension, as git and cargo do, so a library built on NSGo can add its commands to `nsgo`'s:

1. In a module, `nsgo X args…` runs `go tool X args…` when the `go.mod` found from the working directory upward declares a tool whose command name is `X`: the last element of its package path, before a major version such as `/v2`. The extension is then the version the module requires.
2. Otherwise it runs a program named `nsgo-X` on `PATH`, which works outside a module too, such as before a project exists.
3. Otherwise `nsgo` prints its usage and exits with status 2, as it does for any unknown command.

Built-in commands always win, and a name that is not letters, digits, `-` and `_` runs nothing. The extension takes the arguments unchanged, `nsgo`'s standard input, output and error, and its environment with `NSGO_EXTENSION=1` and `NSGO_VERSION`, the `nsgo` module's version, which is empty for a build from a checkout; `nsgo` exits with the extension's status. The usage, which `nsgo help` prints, lists the extensions found and where each comes from.

A library ships one as a Go main package whose directory is the command's name, such as `example.com/maps/cmd/maps` for `nsgo maps`. An app adds it to its module with `go get -tool example.com/maps/cmd/maps@latest`, and `go install example.com/maps/cmd/nsgo-maps@latest` puts an `nsgo-maps` on `PATH` for use outside a module, which may run the same code as the tool.

### The module's nsgo

An `nsgo` installed on `PATH` never builds a project with another version than the one the project pins. In a module whose `go.mod` declares the tool `github.com/nsgo-dev/nsgo/cmd/nsgo` at another version, `nsgo` first runs the module's, `go tool nsgo` with the same arguments, which then resolves the built-in commands and extensions, and exits with its status. `NSGO_DELEGATED=1` in its environment keeps it from doing the same again, and `NSGO_NO_DELEGATE=1` runs the `nsgo` called instead. Outside a module, in one without the tool, in NSGo's own module and at the same version, nothing changes. A built-in command newer than the pinned release, such as `add` in a module that pins 0.3.4, is not handed on: `nsgo` exits with status 1 and names the `go get` that brings it ([Adding libraries](#adding-libraries)). `nsgo version` says which one ran, such as `nsgo v0.4.0 (the module's; called as v0.3.1)`.

## Installers

For the desktop targets, `build` writes what users install the app from into `dist/releases`, each with a `.sha256` checksum:

| Target | Installer |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg`, a disk image showing the app, an arrow and a link to Applications to drag it to, signed with `sign.identity` and [notarized](#notarize) |
| Windows | `<name>-<version>-windows-<arch>-setup.exe`, a native Windows installer that installs for the user, again in the folder the user chose before, closing the app if it runs, with a Start menu shortcut, an optional desktop one and an uninstaller that removes only what it installed, [signed with `sign.certificate`](#sign-windows-packages) where the app signs Windows packages; `windows.command_line` also installs the `.com` companion and puts the app's folder on the user's `Path`, so consoles opened after installing run the app's command, while Win+R, which finds the companion too, starts the app and closes the console it opened; the uninstaller takes the folder off `Path` only if an installer added it, never an entry you had |
| Linux | `<name>-<version>-linux-<arch>.deb` for Debian and Ubuntu, and `<name>-<version>-linux-<arch>.pkg.tar.gz` for Arch Linux, which install the app in `/opt/<slug>` with a command, links to its [commands](#commands), its desktop entry and icon (the Arch package includes `.MTREE` for `pacman -Qkk`), and `<name>-<version>-linux-<arch>.AppImage`, which runs on any distribution without installing |

`-installers=false` or `build.installers: false` builds the desktop apps without installers or store packages. To skip only selected desktop systems, list `macos`, `windows` or `linux` in `build.skip_installers`. The `-skip-installers` flag replaces that list with comma-separated names; `-skip-installers=` clears it.

Building a DMG mounts the disk image with `hdiutil attach`, which can be denied without a logged-in GUI session. On a headless Mac, use `-skip-installers=macos` or set `build.skip_installers: [macos]` to keep the signed `.app` and still build the Windows and Linux installers.

The Windows setup and uninstaller build with Go alone, on any system, and use native Windows dialogs. They speak the languages of the app's catalogs they have words for, Simplified and Traditional Chinese included, picking the system's UI language; `/LANG=<LCID>` picks one for a single run, such as `/LANG=2052` for Simplified Chinese. `/S` installs or uninstalls silently, a final `/D=<folder>` chooses the folder, spaces included, and `/RELAUNCH` opens the app after an update. Without `/D`, an upgrade installs in the folder of the installation it replaces.

The installers take pictures and icons of the app's own; each is the app's icon or the built-in picture otherwise:

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # the disk image's window, which takes its size in points
  dmg_icon: assets/DMGIcon.png              # the disk image's icon: an .icns or a square PNG
windows:
  installer_icon: assets/Setup.png          # the setup program's icon: an .ico or a square PNG
  uninstaller_icon: assets/Uninstall.png    # the uninstaller's
```

A background PNG with a twin twice its size, such as `DMGBackground@2x.png`, stays sharp on Retina screens. The app's icon sits a quarter in from the left, Applications a quarter in from the right, each with its name centered from top to bottom, where the built-in picture draws its arrow; leave room for them in a background of your own. The Linux packages have no pictures of their own; they install the app's icon.

The Linux packages also install the app's AppStream metadata, `/usr/share/metainfo/<id>.metainfo.xml`, by which software centers list it, written from `linux.summary`, `linux.license`, if set, and the app's names, with `Utility` as its category when `linux.categories` names none; the AppImage holds it in its AppDir's `usr/share`, beside a link to its desktop entry in `usr/share/applications`, which the metadata names. Files the system reads from `/usr/share`, such as richer metadata, more desktop entries, shell completions or manual pages, go in the directory `linux.share` names, relative to `nsgo.yaml`: the `.deb` and Arch package install its tree under `/usr/share`, and the AppImage under its AppDir's `usr/share`. Its `metainfo/<id>.metainfo.xml` replaces the one NSGo writes, and a file at a path the package writes itself, such as the app's desktop entry or icon, fails the build. Files in `resources/`, however named, stay beside the executable in `/opt/<slug>`, and store builds' Flatpaks take NSGo's metadata alone.

The AppImage needs FUSE, which most distributions have, and takes GTK 4 from the system, as the packages do; its runtime is downloaded once into the user cache.

AppImages embed a `gh-releases-zsync` locator using `release.github` or the GitHub remote `origin`, selecting the latest stable release and this app’s architecture. Upload each `.AppImage` with its generated `.AppImage.zsync` file. For another host, set `linux.appimage_update` to `zsync|https://example.com/latest.AppImage.zsync`, a stable URL served with HTTP range support; it also accepts `gh-releases-zsync|owner|repo|latest-pre|name-*-linux-x64.AppImage.zsync` for a prerelease channel. An explicit empty string disables embedded discovery; without a GitHub repository or a configured locator it is empty too. The `.zsync` file is still generated, separately from the signed appcast.

## Store packages

`-channel store`, or `updates.channel: store`, builds the desktop and Android targets for their stores, which update them, in place of the installers and install scripts:

| Target | Store package |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.pkg` for the Mac App Store: the app in the App Sandbox, signed with the provisioning profile's identity and packaged with your team's installer identity |
| Windows | `<name>-<version>-windows-<arch>.msix` for the Microsoft Store, which signs it on publishing |
| Linux | Installable `<name>-<version>-linux-<arch>.flatpak`, plus an AppStream-bearing `.tar.gz` and `flatpak/<id>.yml` for Flathub or flatpak-builder on GNOME's runtime |
| Android | `<name>-<version>-android-<arch>.aab`, an App Bundle for Google Play signed with `sign.keystore`, the upload key |

The MSIX reads each language's display name from the catalogs' `appName`, falling back to the source language. Its Start, taskbar and Store logos include 100%, 125%, 150%, 200% and 400% variants. The tool writes their resource index on macOS; no Windows SDK is needed.

`-target macos` selects both macOS architectures on the store channel and writes one `<name>-<version>-macos-universal.pkg`, as does `-target macos_universal`. Select `-target android_arm64,android_x64` for one `<name>-<version>-android-universal.aab` containing `arm64-v8a` and `x86_64`. The x86_64 slice requires an Android NDK.

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType, which the Mac App Store requires
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # beyond the network, the files the user picks and what apple.info explains
windows:
  store:                               # as Partner Center's Product identity lists them
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # the devices the app uses, which Windows asks the user for
linux:
  summary: Says hello                  # one line for software centers
  license: MIT                         # an SPDX expression; LicenseRef-proprietary by default
  # flatpak_runtime: "51"              # GNOME's runtime version
android:
  target_sdk: 36                       # the targetSdkVersion Google Play requires
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # the Mac App Store's profile
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # by default the keychain's of the profile's team
```

The runtime targets API 36 when `android.target_sdk` is unset. An Android App Bundle build fails below API 36, which [Google Play requires](https://support.google.com/googleplay/android-developer/answer/11926878) for new mobile apps and updates from August 31, 2026. A standalone APK build warns and continues.

Set `sign.macos_profile` to sign the Mac package for upload. Without widgets, the unsigned package still runs in the sandbox, which lets you test access to the app's container, the network and the files the user picks. A store build with widgets requires both `sign.macos_profile` for `<id>` and a macOS `sign.widgets_profile` for `<id>.widgets`, from the same team and granting `apple.app_group` (default `group.<id>`). Both bundles use that shared container; no home-directory exception is added. `-installers=false` keeps the sandbox and profile signatures while omitting the `.pkg` and its installer identity. Upload the `.msix` in Partner Center and the `.aab` in Play Console. Submit `flatpak/<id>.yml` to Flathub once the release holds the archives it names. `flatpak-builder` builds it locally too. Install a bundle with `flatpak install --user ./<name>-<version>-linux-<arch>.flatpak`; GNOME's matching runtime must be installed or available from a configured remote. Builds write the bundle in Go without a Linux toolchain. Store packages use a transparent icon when artwork is absent and Utility when `linux.categories` is unset. For installation outside the Store, set [`sign.certificate`](#sign-windows-packages) to sign the MSIX; its certificate subject must match `windows.store.publisher`, and the receiving machine must trust the certificate. Without signing settings the Store signs the uploaded package.

On macOS and iOS, a scroll view's edge effects (`Scroll.EdgeEffects`), macOS backdrop blurs with no `IOS.BackdropMaterial`, and backdrop color filters (alone, or over a blur with no `IOS.BackdropMaterial`) draw through Core Animation's private `CABackdropLayer` and `CAFilter`, as AppKit's and UIKit's own materials and edge effects do, since no public API blurs by a radius, varies a blur across a view, or applies a backdrop color matrix. App Store review may reject apps that use private APIs: NSGo calls them only for scroll views with edge bars and those backdrop filters, but every macOS and iOS build contains their names.

Every Apple bundle `build` writes, the app, its widget extension, App Clip and Apple Watch app, and a `-dynamic` `-embed` framework, carries a privacy manifest, `PrivacyInfo.xcprivacy`, declaring the required-reason APIs NSGo and Go's runtime use: user defaults (`CA92.1`), file timestamps (`C617.1`) and system boot time (`35F9.1`). `apple.privacy` adds the app's own: `accessed_apis`, each category's reasons, `tracking`, `tracking_domains` and `collected_data`, entries of `NSPrivacyCollectedDataTypes` as Apple names them; a `PrivacyInfo.xcprivacy` in `resources/` is merged with them rather than replaced.

The Info.plist of each iOS, tvOS and watchOS bundle `build` writes for devices, the app's, its widget extension's and App Clip's, carries the SDK and Xcode `DT` keys App Store Connect checks, read from the Xcode that builds it, and `UIRequiredDeviceCapabilities: [arm64]`; simulator bundles leave them out, and a key `apple.info` sets keeps its value. `apple.multiple_scenes: true` sets `UIApplicationSupportsMultipleScenes` in an iOS app's scene manifest, which is false by default: on an iPad the user can then open more than one window of the app, and each shows the app's main view anew, with state of its own and the app's shared signals and storage, until the user closes it.

### Publish to Google Play

Enable the Google Play Developer API and give a service account permission to release the app to the chosen track. Set `NSGO_PLAY_SERVICE_ACCOUNT` to the account's JSON key or to the path of its key file. [Google Play API setup](https://developers.google.com/android-publisher/getting_started)

```sh
nsgo build -target android -channel store
nsgo publish -target android -track internal
```

The command finds the App Bundle for the project's version in `build.output/releases`. Pass a `.aab` path after the flags to select a file when there are multiple builds. Use `internal`, `production`, or the track ID of a closed test. The command reads the package and version code from the bundle, requires a code higher than every release on that track, then uploads the bundle and commits the release.

## Store listings

Store metadata (titles, subtitles, descriptions, keywords, release notes, URLs, screenshots and previews) lives in the repository as plain-text YAML and Markdown under `store/` (or `store.dir` in `nsgo.yaml`):

```text
store/
  listing.yaml              # version: 1, default_locale, app_store and google_play settings
  en-US/
    listing.yaml            # version: 1, title, subtitle, short_description, keywords, URLs, media
    description.md          # full description (optional file in place of YAML description)
    release_notes.md        # what's new (optional file in place of YAML release_notes)
    promotional_text.md     # App Store promotional text (optional file)
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

Both `listing.yaml` files require `version: 1` and refuse unknown keys. Canonical locale directories such as `zh-Hans` map automatically to each store's code (`zh-Hans` on App Store Connect, `zh-CN` on Google Play) unless overridden per store with `app_store.locale` or `google_play.locale`.

```sh
nsgo publish -store all -dry-run                  # validate lengths, locales and media without uploading
nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
nsgo publish -store play -track production        # also update release notes on the named Play track
```

Validation runs before reading credentials or making any HTTP request, checking required fields, locale codes, URL schemes, media files and counts (up to 10 screenshots and 3 previews per App Store display type; up to 8 screenshots per Google Play image type), and character limits (App Store: title 30, subtitle 30, keywords 100, promotional text 170, description 4000, release notes 4000; Google Play: title 30, short description 80, description 4000, release notes 500).

Upload credentials come from the environment alone: `NSGO_APP_STORE_KEY`, `NSGO_APP_STORE_KEY_ID` and `NSGO_APP_STORE_ISSUER` (falling back to the notarization key `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` and `NSGO_NOTARY_ISSUER`) for App Store Connect, and `NSGO_PLAY_SERVICE_ACCOUNT` (service-account JSON file path or JSON text) for Google Play.

### Store compliance

Use `nsgo.yaml` to track the status of each release questionnaire.

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

Set an entry to `true` after completing its questionnaire. A matching `-channel store` or `-channel steam` build reminds you about entries set to `false`. Omit any entry you do not track in YAML.

- Complete the [age rating questionnaire](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating) and [App Privacy details](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy) in App Store Connect.
- For Apple's [encryption questionnaire](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation), set `apple.info.ITSAppUsesNonExemptEncryption` to your app's answer as a YAML boolean. If Apple sends an approval code, put it in `apple.info.ITSEncryptionExportComplianceCode` as a string. You can leave the keys out and answer at upload. Apple store builds check the types of these keys and the structure of the bundle's privacy manifest.
- Complete [Data safety](https://support.google.com/googleplay/android-developer/answer/10787469) in Play Console. Apps distributed exclusively on the internal testing track are exempt.
- Complete the [Content Survey](https://partner.steamgames.com/doc/gettingstarted/contentsurvey) in Steamworks before submitting the game for review.
## App Store Connect

Create an app in App Store Connect and sign an iOS device IPA with an App Store distribution profile or a macOS store PKG with your team's installer identity. On a Mac with Xcode, set `NSGO_APP_STORE_KEY` to your App Store Connect team key's `.p8` file or its text, `NSGO_APP_STORE_KEY_ID` to its key ID, and `NSGO_APP_STORE_ISSUER` to its issuer ID; without them the notarization key (`NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, `NSGO_NOTARY_ISSUER`) is used, as for store listings. Give that key access to the app and permission to upload builds.

```sh
nsgo publish ios path/to/Hello.ipa
nsgo publish macos path/to/Hello.pkg
```

The tool checks the bundle's build number before uploading, then reports Apple's processing result for TestFlight. Increase `build_number` for the next upload. macOS build numbers increase across all app versions, while iOS can restart the sequence for a new version. Submit a processed build for App Review separately in App Store Connect. [Apple's distribution guide](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)

## Store asset packs

Declare large asset directories in `mobile.asset_packs`. Files within those directories are delivered separately from the base Android App Bundle and iOS app. Pack names start with a lowercase letter and can contain lowercase letters, digits and underscores.

```yaml
mobile:
  asset_packs:
    - {name: levels, dir: resources/levels, delivery: install-time}
    - {name: voices, dir: large-assets/voices, delivery: fast-follow}
```

Build Android packs with `nsgo build -channel store -target android_arm64`. Use `bundletool build-apks --local-testing` and `bundletool install-apks` to test the bundle on a device. NSGo opens installed `install-time` packs. The `fast-follow` modules are included for Play delivery, and reading those packs from the Play Asset Delivery SDK is still pending.

On iOS, `nsgo build -target ios` maps `install-time` to Apple's essential policy and `fast-follow` to prefetch, both on first install and updates. Xcode 26 or later builds the Background Assets downloader. Use a shared `apple.app_group` or the default `group.<id>`. The app's `sign.profile` and the `<id>.assetdownloader` extension's `sign.asset_packs_profile` both need that group. Upload the `.aar` files from `dist/releases` alongside the IPA using App Store Connect's asset-pack upload workflow. [Creating managed asset packs](https://developer.apple.com/documentation/backgroundassets/creating-managed-asset-packs)

From a goroutine, call `mobile.AssetPackReady(ctx, "levels")` to check readiness and `mobile.OpenAssetPackFile("levels", "intro.map")` to open a file relative to the declared directory. Close the returned reader after use. [Testing Apple asset packs locally](https://developer.apple.com/documentation/backgroundassets/testing-asset-packs-locally)

## Steam depots

```sh
nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

Set `updates.channel: steam` to use the channel by default. Select macOS, Windows and Linux targets. Upload each `dist/<name>-<version>-<target>` directory to its Steam depot, using the app bundle on macOS, the executable on Windows and Linux arm64, and `steam-launch.sh` on Linux x64 as the launch option. Steam handles updates. The directories contain no installers, self-updater or `steam_appid.txt`.

Linux x64 depots include GTK 4 and its dependencies for Steam Linux Runtime 3.0 and 4.0. The launcher uses them only when the system has no GTK 4.14 or newer. Keep `nsgo-gtk/` next to the launcher: its `licenses/` holds the libraries' licenses, the sources they were built from and the build script, which the LGPL requires for GLib, Pango and GTK. Linux arm64 needs GTK 4 available in its runtime.

For local testing outside the Steam client, set `steam: {app_id: 480}` in `nsgo.yaml`, replacing 480 with your Steamworks App ID. `nsgo dev` writes the ID to `steam_appid.txt` in its native build directory and launches the app with that directory as its working directory. [Steamworks API](https://partner.steamgames.com/doc/api/steam_api), [SteamPipe](https://partner.steamgames.com/doc/sdk/uploading).

## Publish on GitHub

When the build has a macOS, Windows or Linux target, `dist/releases` also holds `install.sh` and `install.ps1`, which install the app from the releases of the GitHub repository `release.github` names, else the one the app's Git remote `origin` is on; without either `build` writes none and says so. Upload its files, the top level without `miniprogram/`, to the release tagged `v<version>` (`release.tag_prefix` replaces `v`; the scripts encode reserved URL characters in that prefix):

```sh
gh release create v1.2.0 dist/releases/*.*
```

Users then install or update the app with one line, which the build prints:

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS and Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

On macOS the script copies the app from the disk image into `/Applications`, or `~/Applications` when it cannot write there; on Linux it installs the Arch package with `pacman`, else the Debian one with `apt-get` or `dpkg`, through `sudo`; on Windows it runs the installer silently, which installs where the app already is. Each checks the file against its checksum first. `<NAME>_VERSION`, the app's name in capitals, such as `HELLO_VERSION=1.1.0`, installs another release, and `<NAME>_DOWNLOAD_URL` downloads from a mirror.

## Publish on itch.io

Set `itch.project` to your itch.io game's `owner/game` slug in `nsgo.yaml`. To override channel names, set `itch.channels` by build target. The defaults are `html5` for web, `osx-<arch>` for macOS, `windows-<arch>` for Windows and `linux-<arch>` for Linux.

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

With `BUTLER_API_KEY` set as a secret in CI, build and upload the targets.

```sh
nsgo build -channel none -installers=false -target web,macos_arm64
nsgo publish itch -target web,macos_arm64
```

Use the same `-output` and `-version` on both commands if you override them. The upload takes each target's package directory and sets its itch.io version to the build's version. NSGo uses `butler` on `PATH` or downloads a pinned copy from itch.io. After the first web upload, set the itch.io game type to HTML and mark the upload playable in the browser. [Butler documentation](https://itch.io/docs/butler/pushing.html)

## Updates

`updates` has the app keep itself up to date the way its platform and the channel users get it from do:

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # the public key nsgo keygen prints
  # channel: store                    # direct, store, steam or none; direct by default with a key, none without
  # feed: https://example.com/hello/  # where the appcasts and installers are; by default the latest GitHub release
  # automatic: false                  # check only when asked; true by default
  # automatic_downloads: true          # Sparkle downloads automatically; false by default, macOS only
```

`nsgo keygen` prints a key pair and writes no file: put the public key in `updates.key` and commit it, and keep the secret one out of the repository, in a password manager and in `NSGO_UPDATES_KEY` where releases build, such as a GitHub Actions secret. Each direct build signs the installers with the secret key and writes `appcast-<target>.xml` beside them, a feed in [Sparkle](https://sparkle-project.org)'s format that the app reads under `feed`: upload the appcasts with the installers. Without the secret key, `build` writes no macOS or Windows appcast, as those install only signed installers, and Linux and Android apps reading an unsigned appcast open the release's page instead of installing its package.

`-release-notes notes.md` puts a Markdown file of the release's notes, such as its changelog section, into each appcast, rendered as HTML without the file's own raw HTML. Sparkle and WinSparkle show it in their update dialogs, and the prompt on Linux and Android shows it as text below the version, up to 1,500 characters; apps read it as `core.UpdateCheck.Notes`.

| Target | How a direct build updates |
| --- | --- |
| macOS | Sparkle, which `build` puts into the app, checks daily, offers the new version, downloads the disk image, checks its signature, installs it and restarts the app; the default app menu has Check for Updates…, which an app setting its own menu adds with `updater.Check` |
| Windows | WinSparkle, beside the executable, does the same in the language the app shows as it starts, when WinSparkle has it, and runs the installer silently, which starts the new version |
| Linux, Android | The app reads its appcast, and once the user chooses Update streams and verifies the signed package directly to disk with progress reported (`updater.Status` and the platform task progress indicator), then installs it through `pkexec dpkg`, `pkexec pacman` (`.pkg.tar.gz`/`.pkg.tar.zst`), atomic AppImage replacement (restarting on Linux), or Android's package installer; installs outside those formats or an unsigned appcast offer the release's page instead |
| iOS | The app asks the App Store, which updates it, and offers its page there |

For a published standalone web app, `updater.Check(cx)` compares the running release with a fresh `nsgo.json` and offers to reload after confirmation. It also notices changed build files under the same version label. It needs no appcast key; explicit `none`/`steam` channels still disable checks, and embedded hosts manage their own reloads. Mini programs use their platform's update manager and restart only once the downloaded update is ready. These targets check when the app calls the updater; mini-program download/check timing belongs to the host. They expose no automatic-check preference setters. A custom `cx.CheckForUpdates` handler uses `UpdateCheck.Restart` for a prepared update whose version label may be absent.

### Channels

The channel decides who updates the app. `-channel` replaces `updates.channel` for one build, so one project builds for each:

| Channel | For | The packages |
| --- | --- | --- |
| `direct` | GitHub, a website, Homebrew casks and winget, which install the release's own files | Update themselves as above |
| `store` | The App Store, the Mac App Store, the Microsoft Store, Google Play, Flathub and other stores | Are the [store packages](#store-packages) and carry no self-updater; iOS and macOS ask their App Store without a key, and a Google Play install can use Play's native update flow |
| `steam` | Steam | Produce [depot directories](#steam-depots) that Steam updates |
| `none` | Linux distributions and anyone who repackages the app | Check nothing |

Stores reject apps that update themselves, so upload a `store` build, never a direct one. Each direct release with macOS or Windows installers also writes a Homebrew cask (`dist/releases/homebrew/<name>.rb`, declaring `auto_updates true`, checksums and download URLs for the built disk images, and a `binary` link to each of the app's [commands](#commands)) and a winget multi-file manifest (`dist/releases/winget/<id>.yaml`, `<id>.installer.yaml` and `<id>.locale.en-US.yaml` for the built NSIS installers). Wherever the system tells who installed the app, the app checks that too: an Android app from another store, a Flatpak or Snap, and a TestFlight build leave updates to that store. A manual `updater.Check` in a Google Play install asks Play In-App Updates to offer its immediate flow when allowed; Play handles consent, downloading, installation and restart. Development builds never enter that flow. Standalone Android packages bundle the SDK; [embedding hosts](embedding.md#android) supply it through Gradle.

### Checking from the app

`updater.Check(cx)` checks as a *Check for Updates…* menu item does, and shows what it finds; `updater.CheckInBackground(cx)` shows only a newer version, and `updater.CheckDaily(cx)` does that at most once a day. `updater.Status(cx)` reads the live download and install progress on Linux and Android. While `automatic` is on, the app checks that way on Linux, direct Android builds and Apple App Store builds as it starts and whenever the user comes back to it; a check that fails, as one offline does, is tried again an hour later. Development builds never update. In Steam builds, these three functions do nothing.

`updater.CapabilityOf(cx)` reports how this copy updates without a network request: `ManagedSelf`, `ManagedPage`, `ManagedDevelopment`, `ManagedChannel`, `ManagedStore` (with its `Store` name), or `ManagedUnavailable`. A development build, disabled channel or store installation produces a reason-specific explanation when checked; `core.UpdateCheck.Err` is reserved for failures such as a bad feed or a bundled updater that cannot load.

Read `updater.Status(cx)` inside a binding to follow `Capability`, `AutomaticChecks`, `AutomaticDownloads`, `Available`, `Notes` and `LastCheck`, and on Linux and Android an update's `Phase`, `Loaded` and `Total` (`Fraction()` for a progress bar). `updater.SetAutomaticChecks(cx, on)` persists the user's choice and changes the daily scheduler at runtime. Sparkle and WinSparkle own their preferences; Linux, Android and iOS keep the choice in a private storage partition. `updater.SetAutomaticDownloads(cx, on)` is supported only on macOS, defaulting to `updates.automatic_downloads`; other platforms report false and ignore the setter. Unsupported and externally managed builds report both settings false. WinSparkle reports found-update metadata by reading the appcast after its found callback; it still controls installation. Linux resolves Debian or Arch package ownership and AppImage identity at launch; those copies report `ManagedSelf`, while other unpackaged copies report `ManagedPage` before any check.

TSX apps import the same capabilities from `nsgo/updater`: `checkForUpdate`, `checkInBackground`, `checkDaily`, `capability`, reactive `status`, `setAutomaticChecks` and `setAutomaticDownloads`, running on the same platform updaters. Sandboxed plugins cannot import `nsgo/updater` unless the host names it in `Sandbox.Modules`.

## Launch at login

`desktop.SetLaunchAtLogin(cx, on)` registers the app with the platform's login mechanism, and `desktop.LaunchAtLogin(cx, done)` reads it. An MSIX built by the current tool includes a disabled startup task automatically; enabling it respects the user's Startup apps switch and administrator policy. Ordinary Windows and Linux installs keep their Run value and XDG autostart entry. macOS uses the bundled app's native login service.

Flatpak requests the Background portal, which may ask for consent, without granting access to host configuration files. A subsequent read waits for the native reply. The portal has no read-only startup query: Flatpak reads return only the last confirmed reply in this run, false before one, and cannot detect a change outside the app or recover the setting on the next run.

An externally packaged Snap declares the autostart filename on its app:

```yaml
apps:
  app:
    command: bin/app
    autostart: app.desktop
```

NSGo writes that filename under `$SNAP_USER_DATA/.config/autostart`, which snapd launches through the app's wrapper. `desktop.LaunchedAtLogin(cx)` distinguishes a login launch before the app builds, including an MSIX StartupTask activation.

## File types

`file_types` registers the documents an app opens: from the Finder, the Dock and Open With on macOS, from Explorer's Open With through the Windows installer or the MSIX on Windows, through the desktop entry of the Debian, Arch, AppImage and Flatpak packages on Linux, and through Open In on iOS:

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # <id>.<first extension> by default; another's ID imports its type
    # icon: assets/Flow.icns       # macOS document icon; or a square PNG
    # role: viewer                 # editor by default
    # conforms: [public.json]      # public.data by default
    # mime: [application/json]     # existing MIME types; otherwise inferred on Linux
```

The Windows installer adds the app to Open With without replacing the user's default, and removes its registrations on upgrade and uninstall. Linux names the MIME types of common extensions from a fixed table, and gives an unknown extension of the app's own type `application/x-<type id>` with a shared MIME-info definition. `mime` names existing types instead; an imported type whose extension the table lacks needs it. Running an AppImage registers nothing: a desktop integration tool installs its entry and definitions.

The app hears the files with `desktop.OnOpenFiles`, including a later Windows or Linux launch while it is running and iOS scene document opens. Files that launched the app wait for the first handler:

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

On Windows and Linux, both first and later launches take arguments naming existing regular files (including local `file:` URLs) before the first `--`, after NSGo removes its flags. The handler receives absolute paths. The default scan does not interpret subcommands or flags, so `app -- editor scene.flow` keeps both arguments out of file delivery.

For a CLI that needs to distinguish documents from subcommands or flag values, pass `nsgo.LaunchFiles(parse)` to `nsgo.Run`. The parser gets the arguments before `--`, without the executable or NSGo's flags, with local `file:` URLs already turned into paths; on Linux it also sees any links among them. It returns filesystem paths, and NSGo resolves relative ones against the launching process's working directory before a running instance gets them. Return `nil` to open no files for that launch, or pass `nsgo.LaunchFiles(nil)` to disable command-line file opens entirely, including Open With and choosing a recent file on Windows and Linux, which relaunch the app with the file's path. Links still reach `OnOpenLinks` after `--`; on Windows a link launch never opens files or calls the parser.

On iOS, regular documents are imported into app-owned temporary files with their original names. File-provider reads are coordinated off the UI thread, and their security scopes end before the handler runs. Move an import into `cx.DataDir()` to keep it; edits affect the copy. iOS uses the app's document icon; `icon` applies on macOS. App Clips and tvOS packages register no document types. File opens stay separate from `OnOpenLinks`.

`desktop.AddRecentFile(cx, path)` records a document the user opened: in the Dock menu and File › Open Recent on macOS, the Jump List's Recent category on Windows, and the desktop's recently used files on Linux. A choice reaches the same handler. `desktop.ClearRecentFiles` forgets the app's registrations, keeping other apps' history of shared files on Linux; Linux also respects GTK's history setting.

`desktop.SetDockMenu(cx, items)` sets actions on the app's icon: macOS's Dock menu, Windows' Jump List tasks and the quicklist on Linux's Ubuntu dock, Plasma and Dash to Dock where quicklists are supported. Linux needs an installed desktop entry matching the app's `id`, and keeps submenus, separators, checked and disabled items. The menu belongs to the scope that set it, and the last setter across the app's windows replaces it.

## Links

`url_schemes` gives the app links of its own, such as `myapp://settings`, which open it from a browser, another app, `open`, `start` or `xdg-open`. The macOS, iOS and Android apps register them, as do the Windows installer and MSIX package and the Linux desktop entry, which the Debian, Arch, AppImage and Flatpak packages carry. Uninstalling removes them.

```yaml
url_schemes: [myapp]
# app_links: [links.example.com]  # HTTPS hosts for the full app
```

The app hears them with `desktop.OnOpenLinks`, also while it runs: on Windows and Linux a second launch hands its link to the running app and exits. A link that launched the app waits for the first handler. Anyone can write such a link, and on Windows and Linux the handler hears any URL the app is started with, so check the scheme and ask the user before a link changes anything:

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

On iOS and Android, `mobile.OnOpenLinks` hears links at launch and while the app runs. Open one with `xcrun simctl openurl booted myapp://settings` on the iOS simulator or `adb shell am start -a android.intent.action.VIEW -d myapp://settings` on Android. `app.OpenLinks("myapp://settings")` delivers one in a test.

For iOS universal links, declare explicit HTTPS hosts with `app_links: [links.example.com]`. Each host is a DNS name without a scheme, path, port, IP address or wildcard; names are lowercased and duplicates are rejected. The tool writes `applinks:<host>` to the full app's Associated Domains entitlement, including Xcode-managed signing; an IPA profile must authorize it. App Clips retain the separate `appclips:` claims from `app_clip.domains`.

Each host must serve an `apple-app-site-association` file that names the full app at `https://<host>/.well-known/apple-app-site-association`; package metadata alone cannot authorize a universal link. [Apple's association requirements](https://developer.apple.com/documentation/xcode/supporting-associated-domains) describe the server setup. Ad hoc simulator signatures omit restricted developer entitlements, so a delegate test does not establish server association.

On iOS, `mobile.OpenedURL(cx)` tracks the most recent App Clip invocation, universal link or custom scheme URL, including the initial value before the root mounts. Custom scheme delivery still reaches `mobile.OnOpenLinks`; the invocation value is current when that handler runs. File opens stay in `OnOpenFiles` and do not replace the invocation URL. Android/Web invocation support is tracked separately in #1475.

## Incoming shares on Android

Choose the MIME types the app handles from the Android share sheet:

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

APK and App Bundle builds register those types for both single and multiple items. Register [`mobile.OnShared`](services.md#opening-links-and-files) to receive each share.

For a web app with path-based addresses, set `web.base` to its deployment directory (`/` or `/my-app/`) and configure your production server to serve `index.html` for application routes. The development server already provides that fallback for HTML navigation requests. Assets remain relative to that fixed HTML base when a deep link is reloaded. See [Application addresses](addresses.md).

## Incoming shares on iOS

Declare the Uniform Type Identifiers the share extension accepts, with URL and text types before broader file types:

```yaml
apple:
  share_types: [public.url, public.plain-text, public.image, com.adobe.pdf]
  # app_group: group.com.example.app  # defaults to group.<id>
sign:
  # share_profile: assets/Share.mobileprovision
```

`build` and `dev` package a Go share extension, `<id>.share`, with the app. Xcode signs it and registers its App Group for device runs. For an IPA, enable the same App Group for `<id>` and `<id>.share`; `sign.profile` and `sign.share_profile` must grant it. App Clips, tvOS and watchOS have no share extension.

The extension copies up to 128 items and 512 MiB of files into the App Group, then completes the share sheet. `mobile.OnShared` receives those batches while the app is active or when the user next opens it; they survive the extension and app exiting before delivery. iOS provides no supported API for a Share extension to foreground or launch its containing app. This handoff therefore does not automatically open the app.

## Mini programs

```sh
nsgo build -target miniprogram_wechat -run
nsgo dev -target miniprogram_alipay
```

Each target writes a project for its platform's developer tool, also in `dist/releases/miniprogram/<platform>`, which `-run` and `dev` open; `dev` builds again as you save, and the tool reloads:

| Target | Platform | Tool |
| --- | --- | --- |
| `miniprogram_wechat` | WeChat | [WeChat DevTools](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html), with its service port on (Settings, Security) |
| `miniprogram_qq` | QQ | [QQ's developer tool](https://q.qq.com/wiki/tools/devtool/), on macOS |
| `miniprogram_alipay` | Alipay | [Alipay's Mini Program Studio](https://opendocs.alipay.com/mini/ide/download), on macOS |
| `miniprogram_douyin` | Douyin and ByteDance's other apps | [Douyin's developer tool](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download), on macOS |

`miniprogram.appid` gives the mini program's AppID on each platform (`wechat`, `qq`, `alipay`, `douyin`); without one, WeChat DevTools previews it with its tourist AppID and the other tools ask for one. On WeChat, `-run` and each `dev` rebuild close and auto-open the project through the CLI, so DevTools compiles the fresh files. This was checked with DevTools 2.01.2510290. The platform's navigation bar shows the app's title; with `custom_navigation`, the app fills the page and its safe area keeps it clear of the menu button. Set `miniprogram.wechat_speech_plugin` to a plugin version (such as `"0.3.5"`) to declare WeChat's official `WechatSI` text-to-speech plugin (`wx069ba97219f66d99`) in WeChat's `app.json` after enabling it in the WeChat MP console. The app reads the phone's OS with `Env.Simulated` true, as on the web with `--nsgo-os`: a stack's pages show their own `Header`, as no system header replaces it. QQ, Alipay and Douyin have not been tried in their tools yet, and Alipay does not yet take back through the app, keyframes or the keyboard's height.

Mini programs are experimental. A mini program runs Go apps alone: `build` refuses apps that link `tsx` (and `tsx` entry points built with `nsgominiprogram` fail with `tsx.ErrUnsupported`), and the platforms hold the app's compressed module to 2 MB — an app of a few screens on `base` fits, as `examples/gallery-miniprogram` does, one of `ui`'s controls does not yet, and `build` says when one does not. Web views (`<web-view>` fills a page of its own), custom native components and shaders (`nsgo_noshader`) are unavailable and report `errors.ErrUnsupported` (`core.CodeUnavailable`); app font files (`loadFontFace`), tinted images, off-baseline inline attachments, selection areas and `<video>` playback are wired across all four platforms' templates though still unchecked in vendor DevTools.

## HarmonyOS

```sh
nsgo build -target harmonyos -run
```

`harmonyos` writes `<name>-<version>-harmonyos.hap` to `dist/releases`. The engine runs as WebAssembly in ArkWeb, and an ArkTS host draws what it mounts with ArkUI views, so text, fields, scrolling and dialogs are the system's own. With `hdc` on `PATH`, `-run` installs the HAP on the connected device or emulator and starts it, and `nsgo test -target harmonyos` runs an app's tests from an unsigned HAP on an emulator under a separate `.nsgotest` bundle identity, removing that test HAP after the run so the installed app remains available.

To compile the ArkTS host from the templates and use the app's `id` as the bundle name, put `hvigorw` on `PATH` and set `OHOS_BASE_SDK_HOME` to an OpenHarmony SDK with API 26 (`26.0.0`), the templates' compile and target SDK version. `build.harmonyos_bundles` picks `auto`, the default, `exact`, which requires the SDK, or `padded`, which pads the id to the committed host's fixed length and needs no SDK. Target tests use the same setting and print which host they packaged. `harmonyos.permissions` lists the `ohos.permission` names the app asks for, such as `MICROPHONE` and `CAMERA`, and `widget.title` is what its home-screen card shows.

HarmonyOS is a preview. Emulators can install unsigned HAPs. [The architecture notes](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md) list the ArkUI features the preview draws.

### Signed HAPs

Set `harmonyos.signing` to sign a HAP with your AppGallery Connect materials. A direct install on a Huawei phone needs a debug certificate and profile for that device. Use the release materials for distribution. [Huawei signing guide](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

Paths are relative to `nsgo.yaml`. Set `NSGO_HARMONY_KEYSTORE_PASSWORD` and `NSGO_HARMONY_KEY_PASSWORD` in the environment. Signing uses the SDK's `hap-sign-tool.jar` and Java from `JAVA_HOME` or `PATH`. The build keeps the app's exact `id` to match the profile, so signing needs the SDK and `hvigorw` even in `auto` mode. `nsgo doctor`, run in the app directory, checks the signing configuration and profile bundle name without passwords. `build -run` gives `hdc` the signed HAP.

## Sign

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # signs the macOS app, else ad hoc
  profile: assets/AppStore.mobileprovision  # signs the IPA
  keystore: assets/release.p12              # signs APKs, with key_alias
  certificate: assets/codesign.pfx          # signs the Windows app and its installer
  # widgets_profile: assets/Widgets.mobileprovision  # the iOS extension's profile; use a macOS profile for a Mac App Store build
  # share_profile: assets/Share.mobileprovision      # signs the IPA's share extension
  # clip_profile: assets/Clip.mobileprovision        # signs the IPA's App Clip
```

The keystore's passwords come from `NSGO_KEYSTORE_PASSWORD` and `NSGO_KEY_PASSWORD`. `-team` signs the iPhone's package. `-profile`, `-widgets-profile`, `-share-profile`, `-clip-profile` and `-macos-profile` replace the app's, the widget extension's, the share extension's, the App Clip's and the Mac App Store's provisioning profiles from the command line. `-installer` replaces the Mac Installer Distribution identity.

### Sign Windows packages

Authenticode identifies a Windows download's publisher; [SmartScreen checks its reputation separately](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation). `sign.certificate`, a `.pfx`, signs the executable, the command companion, the programs and libraries the app's resources bring, the installer with its uninstaller, and the MSIX built for the Store channel; its password comes from `NSGO_CERTIFICATE_PASSWORD`. The signatures are made in Go, as `signtool` makes them, so a Mac signs them without the Windows SDK:

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # the RFC 3161 server that timestamps them
```

`sign.timestamp_url` defaults to DigiCert's, and `none` leaves the signatures untimestamped, which they lose at the certificate's expiration. A certificate on a hardware token or in a cloud service signs through a command of its own instead, in which `%1` is the file to sign and which the build runs for every file above:

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

`build` stops if the command succeeds but leaves the app executable, command companion, a PE resource, or the setup executable without an embedded Authenticode signature.

The uninstaller inside the installer is signed before it is embedded, with the same `sign.certificate` or `sign.sign_command` as the app and the setup. `-certificate`, `-sign-command` and `-timestamp-url` set these from the command line.

### Notarize

Apps downloaded on a Mac open without Gatekeeper's warning once Apple has notarized them. Sign with a Developer ID and name the credentials notarytool keeps in your keychain:

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # the keychain profile above; or -notarize hello
```

`build` then signs with the hardened runtime, submits the disk image, waits for Apple's verdict, a few minutes, and staples the ticket to the image and the app; a rejection prints Apple's log. In CI, `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` and `NSGO_NOTARY_ISSUER`, an App Store Connect API key's `.p8` file, ID and issuer, take the profile's place. The hardened runtime denies the microphone, the camera, the calendars and location unless the app claims them, which `build` does when `apple.info` has `NSMicrophoneUsageDescription`, `NSCameraUsageDescription`, a calendars or reminders usage description, or a location usage description.

## Widgets and Live Activities

`build` and `dev` compile the extension that draws the app's widgets and Live Activities into the macOS and iOS packages with Xcode's Swift compiler, with no separate step. On iOS the app and its widgets share an app group: `-run` on an iPhone has Xcode sign both and register the group; for an IPA, enable it for both `<id>` and `<id>.widgets`, and sign the extension with `sign.widgets_profile`. Android standalone APKs and AABs include App Widget providers and resources from `widgets`; the prebuilt Java runtime draws their timelines with no SDK or Java compiler needed by app builds. Android 16 shows Live Activities as Live Updates, which need no extension. [Widgets](widgets.md) and [Live Activities](live-activities.md) cover their configuration and API.

## Android App Links

`app_links` names explicit HTTPS hosts whose links open the full Android app:

```yaml
app_links: [example.com, shop.example.com]
```

The APK and App Bundle declare a browsable `ACTION_VIEW` filter with `autoVerify` for each host. Use DNS hosts without a scheme, path, port, wildcard or IP address. Serve `https://<host>/.well-known/assetlinks.json` with the app's package ID and signing certificate fingerprint; Android verifies that association before routing links to the app. The tool does not publish association files; [Android's verification guide](https://developer.android.com/training/app-links/verify-applinks) describes their setup. An embedded Android host declares its own intent filters and forwards accepted links to its `NsgoView`.

`mobile.OpenedURL(cx)` reads the accepted invocation before the first root render and changes when another link opens the running app. `mobile.OnOpenLinks` still hears each accepted link. A whole-page web app reads its current page address through `mobile.OpenedURL`, including browser history and hash changes; an app mounted inside another page reads no containing-page address. Use [`cx.AppURL`](addresses.md) when the app also writes its address.

## App Clips

An App Clip is a small part of an iOS app that runs from a link, a code or an NFC tag without installing the app. `app_clip` builds one into the iOS package, from a main package of its own so it links only what it shows:

```yaml
app_clip:
  main: ./clip              # the clip's main package, the app's own by default
  domains: [example.com]    # whose links open it
  # name: Hello Clip        # "<name> Clip" by default
  # id: dev.example.hello.Clip  # <id>.Clip by default
  # minimum: "17.0"         # 16.0 by default; 17.0 allows 100 MB when only links open it
  # notifications: true     # ask for notifications for a day
  # location: true          # let the clip confirm where it was opened
```

The clip reads the link that opened it with `mobile.OpenedURL`, which changes as another link opens it again, and `mobile.IsAppClip` says whether the code runs as one. The system downloads a clip of up to 15 MB, or 100 MB from iOS 17 when only links open it, and `build` refuses a larger one; a clip linking `ui` takes about 11 MB. `dev -target ios_simulator -clip https://example.com/order?table=12` runs the clip alone in the simulator, opened from that link; `-target ios` runs it on an iPhone and prints the link's QR code: add the link in Settings > Developer > Local Experiences, then scan it with the camera. Use `-release` if development symbols push the clip over the QR size limit. Apple signs App Clips only for teams of the Apple Developer Program, not personal teams.

`dev -target ios` and `build -run -target ios` also keep the clip inside the full app: Xcode signs it as a separate target and embeds it before signing the app. Automatic signing needs an authenticated paid-team account in Xcode, with provisioning for both app and clip. For the App Store, enable App Clips for `<id>`, sign the clip with `sign.clip_profile`, and serve the apple-app-site-association file of the domains. The repository's `examples/appclip` is a café whose clip orders at a table.

To hand an order or other local data to the full app, set an explicit group alongside `app_clip`:

```yaml
apple:
  app_group: group.dev.example.hello
```

The clip and full app then share `cx.DataDir()` and `cx.Storage()` in the group’s container. Files under the data directory and key-value entries the clip writes are available when the full app starts; `cx.LogsDir()` follows that directory, while each bundle keeps its own cache. Xcode signing claims the group for both targets; when using profiles, enable the same group for both App IDs and grant it in `sign.profile` and `sign.clip_profile`. A missing grant refuses signing, and an inaccessible group stops startup. A host’s explicit data directory still takes precedence.

Without `apple.app_group`, each bundle keeps its own sandbox storage. Enabling the setting does not migrate data previously stored in those sandboxes, and widget-only apps keep their existing storage location. Keep shared secrets in the keychain rather than plain storage. The café example persists its clip’s order and displays it in the full app.

From an explicit user action inside the clip, call `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)` to ask StoreKit to recommend its corresponding full app. `AppClipOverlayBottomRaised` leaves room for a bottom bar. The returned function dismisses that request; scope cleanup also dismisses it, and an older request cannot dismiss a newer one. Calls outside a native iOS App Clip do nothing. The App Store decides whether it can load and display the recommendation, so the call reports neither a displayed offer nor an installation. Keep the clip's task usable without installing the full app. [Apple's overlay guidance](https://developer.apple.com/documentation/appclip/recommending-your-app-to-app-clip-users)

## Embed in another app

`-embed` packages the app for an iOS or Android app of your own: an XCFramework, of dynamic frameworks with `-dynamic`, or an AAR whose `NSGoView` or `NsgoView` shows it. Go desktop programs show apps with `nsgo.NewView`, and web pages with the site's `nsgo.js`, whose host is the page's or the one `web.host` in `nsgo.yaml` names. The repository's `examples/embed-*` show each; [Embedding](embedding.md) covers the host side.
